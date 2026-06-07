package main

import (
	"context"
	"crypto/rand"
	"crypto/tls"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/mitchellh/go-homedir"
	"go.etcd.io/bbolt"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

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

const (
	// shutdownTimeout bounds how long graceful shutdown waits for in-flight
	// connections to drain before the process exits.
	shutdownTimeout = 10 * time.Second

	// readHeaderTimeout bounds how long the server waits for request headers.
	readHeaderTimeout = 20 * time.Second
)

//go:embed admin
//go:embed admin/_next/static
//go:embed admin/_next/static/chunks/pages/*.js
//go:embed admin/_next/static/*/*.js
var adminContent embed.FS

func loadOrCreateToken(path string) (string, error) {
	expanded, err := homedir.Expand(path)
	if err != nil {
		return "", fmt.Errorf("failed to expand token path: %w", err)
	}

	data, err := os.ReadFile(expanded)
	if err == nil {
		return strings.TrimSpace(string(data)), nil
	}
	if !os.IsNotExist(err) {
		return "", fmt.Errorf("failed to read token file: %w", err)
	}

	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := hex.EncodeToString(buf)

	if err := os.MkdirAll(filepath.Dir(expanded), 0o700); err != nil {
		return "", fmt.Errorf("failed to create token directory: %w", err)
	}
	if err := os.WriteFile(expanded, []byte(token), 0o600); err != nil {
		return "", fmt.Errorf("failed to write token file: %w", err)
	}

	return token, nil
}

func run(ctx context.Context, cfg config.Config, logger *zap.Logger) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt)
	defer stop()

	mainLogger := logger.Named("main")

	host := cfg.Host
	if host == "" {
		host = "127.0.0.1"
	}
	addr := fmt.Sprintf("%s:%d", host, cfg.Port)
	url := fmt.Sprintf("http://%s:%d", host, cfg.Port)

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

	adminToken, err := loadOrCreateToken("~/.lynx/token")
	if err != nil {
		mainLogger.Fatal("Failed to load or create admin token.", zap.Error(err))
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
		Scope:         scope,
		Repository:    boltDB,
		Logger:        logger.Named("reqlog").Sugar(),
		Broadcaster:   broadcaster,
		MaxBodyBytes:  cfg.MaxBodyBytes,
		RedactHeaders: cfg.RedactHeaders,
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

	// Auth middleware: require Bearer token on all /api/* routes except /api/token.
	adminRouter.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/api/token" {
				next.ServeHTTP(w, r)
				return
			}
			if strings.HasPrefix(r.URL.Path, "/api/") {
				auth := r.Header.Get("Authorization")
				if !strings.HasPrefix(auth, "Bearer ") || strings.TrimPrefix(auth, "Bearer ") != adminToken {
					http.Error(w, "unauthorized", http.StatusUnauthorized)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	// CSRF: for state-changing requests, validate Origin matches the listen address if present.
	adminRouter.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodPost || r.Method == http.MethodPut || r.Method == http.MethodDelete {
				origin := r.Header.Get("Origin")
				if origin != "" {
					allowed := []string{
						fmt.Sprintf("http://%s", addr),
						fmt.Sprintf("http://localhost:%d", cfg.Port),
						fmt.Sprintf("http://127.0.0.1:%d", cfg.Port),
					}
					ok := false
					for _, a := range allowed {
						if origin == a {
							ok = true
							break
						}
					}
					if !ok {
						http.Error(w, "forbidden", http.StatusForbidden)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	})

	// Bootstrap: return the admin token (no auth required; protected by localhost-only bind).
	adminRouter.Path("/api/token").Methods(http.MethodGet).HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		fmt.Fprint(w, adminToken)
	})

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
		if input.Host == "" {
			input.Host = "127.0.0.1"
		}
		if input.LogLevel == "" {
			input.LogLevel = config.DefaultLogLevel
		}
		if _, err := zapcore.ParseLevel(input.LogLevel); err != nil {
			http.Error(w, "logLevel must be one of: debug, info, warn, error", http.StatusBadRequest)
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
		Addr:    addr,
		Handler: router,
		// ReadHeaderTimeout bounds the header-read phase to mitigate slow-header
		// (Slowloris) attacks. It only applies before a CONNECT tunnel is hijacked,
		// so it doesn't cap long-lived proxy connections or streamed bodies.
		ReadHeaderTimeout: readHeaderTimeout,
		TLSNextProto:      map[string]func(*http.Server, *tls.Conn, http.Handler){},
		ErrorLog:          zap.NewStdLog(logger.Named("http")),
	}

	go func() {
		mainLogger.Info(fmt.Sprintf("Lynx (v%v) is running on %v ...", version, addr))
		mainLogger.Info(fmt.Sprintf("\x1b[%dm%s\x1b[0m", uint8(32), "Get started at "+url))
		mainLogger.Info(fmt.Sprintf("Admin token: %s", adminToken))

		err := httpServer.ListenAndServe()
		if err != http.ErrServerClosed {
			mainLogger.Fatal("HTTP server closed unexpected.", zap.Error(err))
		}
	}()

	<-ctx.Done()
	stop()

	mainLogger.Info("Shutting down HTTP server. Press Ctrl+C to force quit.")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	//nolint:contextcheck
	err = httpServer.Shutdown(shutdownCtx)
	if err != nil {
		return fmt.Errorf("failed to shutdown HTTP server: %w", err)
	}

	return nil
}
