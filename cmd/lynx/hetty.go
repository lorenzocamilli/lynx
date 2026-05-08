package main

import (
	"context"
	"crypto/tls"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"

	"github.com/gorilla/mux"
	"github.com/mitchellh/go-homedir"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"

	"github.com/lorenzocamilli/lynx/pkg/api"
	"github.com/lorenzocamilli/lynx/pkg/config"
	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
	"github.com/lorenzocamilli/lynx/pkg/sse"
)

var version = "0.0.0"

//go:embed admin
//go:embed admin/_next/static
//go:embed admin/_next/static/chunks/pages/*.js
//go:embed admin/_next/static/*/*.js
var adminContent embed.FS

func run(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	mainLogger := logger.Named("main")

	addr := fmt.Sprintf(":%d", cfg.Port)
	url := fmt.Sprintf("http://localhost:%d", cfg.Port)

	caCertFile, err := homedir.Expand("~/.lynx/lynx_cert.pem")
	if err != nil {
		mainLogger.Fatal("Failed to expand CA certificate filepath.", zap.Error(err))
	}

	caKeyFile, err := homedir.Expand("~/.lynx/lynx_key.pem")
	if err != nil {
		mainLogger.Fatal("Failed to expand CA private key filepath.", zap.Error(err))
	}

	dbFilePath, err := homedir.Expand("~/.lynx/lynx.db")
	if err != nil {
		mainLogger.Fatal("Failed to expand database path.", zap.Error(err))
	}

	caCert, caKey, err := proxy.LoadOrCreateCA(caKeyFile, caCertFile)
	if err != nil {
		mainLogger.Fatal("Failed to load or create CA key pair.", zap.Error(err))
	}

	dbLogger := logger.Named("boltdb").Sugar()
	boltOpts := *bbolt.DefaultOptions
	boltOpts.Logger = &bolt.Logger{SugaredLogger: dbLogger}

	boltDB, err := bolt.OpenDatabase(dbFilePath, &boltOpts)
	if err != nil {
		mainLogger.Fatal("Failed to open database.", zap.Error(err))
	}
	defer boltDB.Close()

	scope := &scope.Scope{}

	broadcaster := sse.NewBroadcaster()

	reqLogService := reqlog.NewService(reqlog.Config{
		Scope:       scope,
		Repository:  boltDB,
		Logger:      logger.Named("reqlog").Sugar(),
		Broadcaster: broadcaster,
	})

	interceptService := intercept.NewService(intercept.Config{
		Logger:      logger.Named("intercept").Sugar(),
		Broadcaster: broadcaster,
	})

	senderService := sender.NewService(sender.Config{
		Repository:    boltDB,
		ReqLogService: reqLogService,
	})

	projService, err := proj.NewService(proj.Config{
		Repository:       boltDB,
		InterceptService: interceptService,
		ReqLogService:    reqLogService,
		SenderService:    senderService,
		Scope:            scope,
	})
	if err != nil {
		mainLogger.Fatal("Failed to create new projects service.", zap.Error(err))
	}

	p, err := proxy.NewProxy(proxy.Config{
		CACert: caCert,
		CAKey:  caKey,
		Logger: logger.Named("proxy").Sugar(),
	})
	if err != nil {
		mainLogger.Fatal("Failed to create new proxy.", zap.Error(err))
	}

	p.UseRequestModifier(reqLogService.RequestModifier)
	p.UseResponseModifier(reqLogService.ResponseModifier)
	p.UseRequestModifier(interceptService.RequestModifier)
	p.UseResponseModifier(interceptService.ResponseModifier)

	fsSub, err := fs.Sub(adminContent, "admin")
	if err != nil {
		mainLogger.Fatal("Failed to construct file system subtree from admin dir.", zap.Error(err))
	}

	adminHandler := http.FileServer(http.FS(fsSub))
	router := mux.NewRouter().SkipClean(true)
	adminRouter := router.MatcherFunc(func(req *http.Request, match *mux.RouteMatch) bool {
		hostname, _ := os.Hostname()
		host, _, _ := net.SplitHostPort(req.Host)

		return strings.EqualFold(host, hostname) ||
			req.Host == "lynx.proxy" ||
			req.Host == fmt.Sprintf("localhost:%d", cfg.Port) ||
			req.Host == fmt.Sprintf("127.0.0.1:%d", cfg.Port) ||
			req.Method != http.MethodConnect && !strings.HasPrefix(req.RequestURI, "http://")
	}).Subrouter().StrictSlash(true)

	// GraphQL server.
	gqlEndpoint := "/api/graphql/"
	adminRouter.Path(gqlEndpoint).Handler(api.HTTPHandler(&api.Resolver{
		ProjectService:    projService,
		RequestLogService: reqLogService,
		InterceptService:  interceptService,
		SenderService:     senderService,
	}, gqlEndpoint))

	// SSE event stream.
	adminRouter.Path("/api/events").Handler(sse.Handler(broadcaster))

	// CA certificate download endpoint (DER-encoded, importable by browsers).
	adminRouter.Path("/api/ca.crt").HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", `attachment; filename="lynx_ca.crt"`)
		w.Write(caCert.Raw)
	})

	// Settings REST endpoints.
	adminRouter.Path("/api/settings").Methods(http.MethodGet).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(cfg)
	})

	adminRouter.Path("/api/settings").Methods(http.MethodPost).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input config.Config
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		if input.Port < 1 || input.Port > 65535 {
			http.Error(w, "port must be between 1 and 65535", http.StatusBadRequest)
			return
		}

		if err := config.Save(config.DefaultPath, input); err != nil {
			mainLogger.Error("Failed to save config.", zap.Error(err))
			http.Error(w, "failed to save config", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(input)
	})

	adminRouter.PathPrefix("").Handler(adminHandler)

	// Fallback (default) is the Proxy handler.
	router.PathPrefix("").Handler(p)

	httpServer := &http.Server{
		Addr:         addr,
		Handler:      router,
		TLSNextProto: map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ErrorLog:     zap.NewStdLog(logger.Named("http")),
	}

	go func() {
		mainLogger.Info(fmt.Sprintf("Lynx (v%v) is running on %v ...", version, addr))
		mainLogger.Info(fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(32), "Get started at "+url))

		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			mainLogger.Fatal("HTTP server closed unexpected.", zap.Error(err))
		}
	}()

	<-ctx.Done()
	stop()

	mainLogger.Info("Shutting down HTTP server. Press Ctrl+C to force quit.")

	//nolint:contextcheck
	err = httpServer.Shutdown(context.Background())
	if err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	return nil
}
