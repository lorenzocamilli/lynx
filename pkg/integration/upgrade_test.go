package integration_test

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
)

// A 101 body is a connection, not a finite HTTP document. Exercise the real
// ReverseProxy upgrade path with both service middlewares, not a recorder
// (which cannot hijack). The test protocol sends data in both directions.
func TestHTTPUpgrade(t *testing.T) {
	yes, no := true, false
	for _, tc := range []struct {
		name      string
		logging   bool
		responses bool
		requests  bool
		override  *bool
		gzip      bool
	}{
		{name: "no-project"},
		{name: "logging", logging: true},
		{name: "response-interception", logging: true, responses: true},
		{name: "response-interception-no-project", responses: true},
		{name: "force-response", logging: true, override: &yes},
		{name: "skip-response", logging: true, responses: true, override: &no},
		{name: "approved-request-force-response", logging: true, requests: true, override: &yes},
		{name: "approved-request-skip-response", logging: true, requests: true, responses: true, override: &no},
		{name: "gzip-header", logging: true, responses: true, gzip: true},
	} {
		for _, ending := range []string{"client-close", "proxy-cancel", "upstream-close"} {
			t.Run(tc.name+"/"+ending, func(t *testing.T) {
				originDone := make(chan struct{})
				origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					defer close(originDone)
					conn, rw, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					defer conn.Close()
					_ = conn.SetDeadline(time.Now().Add(5 * time.Second))
					_, _ = fmt.Fprint(rw, "HTTP/1.1 101 Switching Protocols\r\nConnection: Upgrade\r\nUpgrade: lynx-test\r\n")
					if tc.gzip {
						// Encoding headers must not trigger ordinary body decoding.
						_, _ = fmt.Fprint(rw, "Content-Encoding: gzip\r\n")
					}
					_, _ = fmt.Fprint(rw, "\r\nready\n")
					_ = rw.Flush()
					for {
						line, err := rw.ReadString('\n')
						if err != nil || line == "close\n" {
							return
						}
						if _, err := fmt.Fprint(rw, "echo: "+line); err != nil {
							return
						}
						if err := rw.Flush(); err != nil {
							return
						}
					}
				}))
				defer origin.Close()

				db, err := bolt.OpenDatabase(filepath.Join(t.TempDir(), "lynx.db"), nil)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				ctx := context.Background()
				projectID := ulid.ULID{1}
				if err := db.UpsertProject(ctx, proj.Project{ID: projectID}); err != nil {
					t.Fatal(err)
				}
				activeID := projectID
				if !tc.logging {
					activeID = ulid.ULID{}
				}
				logs := reqlog.NewService(reqlog.Config{Repository: db, ActiveProjectID: activeID, MaxBodyBytes: 4})
				interceptor := intercept.NewService(intercept.Config{RequestsEnabled: tc.requests, ResponsesEnabled: tc.responses})
				cert, key, err := proxy.NewCA("Upgrade Test CA", "Test", time.Hour)
				if err != nil {
					t.Fatal(err)
				}
				p, err := proxy.NewProxy(proxy.Config{CACert: cert, CAKey: key})
				if err != nil {
					t.Fatal(err)
				}
				p.UseRequestModifier(logs.RequestModifier)
				p.UseResponseModifier(logs.ResponseModifier)
				if tc.override != nil && !tc.requests {
					p.UseRequestModifier(func(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
						return func(r *http.Request) {
							*r = *r.WithContext(intercept.WithInterceptResponse(r.Context(), *tc.override))
							next(r)
						}
					})
				}
				p.UseRequestModifier(interceptor.RequestModifier)
				p.UseResponseModifier(interceptor.ResponseModifier)
				proxyDone := make(chan struct{})
				proxyCancel := make(chan context.CancelFunc, 1)
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					ctx, cancel := context.WithCancel(r.Context())
					defer cancel()
					proxyCancel <- cancel
					defer close(proxyDone)
					p.ServeHTTP(w, r.WithContext(ctx))
				}))
				defer server.Close()
				proxyURL, err := url.Parse(server.URL)
				if err != nil {
					t.Fatal(err)
				}
				transport := &http.Transport{Proxy: http.ProxyURL(proxyURL), ResponseHeaderTimeout: 3 * time.Second}
				defer transport.CloseIdleConnections()
				// Client.Timeout wraps Body in a read-only timer wrapper. Bound
				// the handshake here and stream I/O at the origin instead.
				client := &http.Client{Transport: transport}
				req, err := http.NewRequest(http.MethodGet, origin.URL+"/upgrade", nil)
				if err != nil {
					t.Fatal(err)
				}
				req.Header.Set("Connection", "Upgrade")
				req.Header.Set("Upgrade", "lynx-test")
				type result struct {
					res *http.Response
					err error
				}
				completed := make(chan result, 1)
				go func() {
					res, err := client.Do(req)
					completed <- result{res, err}
				}()
				cancelProxy := <-proxyCancel
				defer cancelProxy()
				if tc.requests {
					waitFor(t, 2*time.Second, func() bool { return len(interceptor.Items()) > 0 })
					item := interceptor.Items()[0]
					if item.Request == nil {
						t.Fatal("upgrade handshake request was not intercepted")
					}
					id, ok := proxy.RequestIDFromContext(item.Request.Context())
					if !ok {
						t.Fatal("pending request has no ID")
					}
					if err := interceptor.ModifyRequest(id, item.Request, tc.override); err != nil {
						t.Fatal(err)
					}
				}
				roundTrip := <-completed
				if roundTrip.err != nil {
					t.Fatal(roundTrip.err)
				}
				res := roundTrip.res
				defer res.Body.Close()
				if res.StatusCode != http.StatusSwitchingProtocols {
					t.Fatalf("status = %d, want 101", res.StatusCode)
				}
				stream, ok := res.Body.(io.ReadWriteCloser)
				if !ok {
					t.Fatal("upgrade body is not bidirectional")
				}
				reader := bufio.NewReader(stream)
				if line, err := reader.ReadString('\n'); err != nil || line != "ready\n" {
					t.Fatalf("server greeting = %q, %v", line, err)
				}
				for _, message := range []string{"hello\n", "second frame\n"} {
					if _, err := io.WriteString(stream, message); err != nil {
						t.Fatal(err)
					}
					if line, err := reader.ReadString('\n'); err != nil || line != "echo: "+message {
						t.Fatalf("echo = %q, %v", line, err)
					}
				}
				if len(interceptor.Items()) != 0 {
					t.Fatal("upgrade response entered the pending queue")
				}
				if tc.logging {
					// Metadata must be persisted while the connection is still open.
					waitFor(t, time.Second, func() bool {
						stored, err := db.FindRequestLogs(ctx, reqlog.FindRequestsFilter{ProjectID: projectID}, nil)
						if err != nil {
							t.Fatal(err)
						}
						if len(stored) != 1 || stored[0].Response == nil {
							return false
						}
						response := stored[0].Response
						if response.StatusCode != 101 || len(response.Body) != 0 || response.Size != 0 || response.Header.Get("Upgrade") != "lynx-test" {
							t.Fatalf("invalid upgrade metadata: %+v", response)
						}
						return true
					})
				}
				switch ending {
				case "proxy-cancel":
					cancelProxy()
				case "upstream-close":
					if _, err := io.WriteString(stream, "close\n"); err != nil {
						t.Fatal(err)
					}
				}
				if ending != "client-close" {
					if _, err := reader.ReadByte(); err != io.EOF {
						t.Fatalf("expected EOF after %s, got %v", ending, err)
					}
				}
				_ = stream.Close()
				for _, done := range []<-chan struct{}{originDone, proxyDone} {
					select {
					case <-done:
					case <-time.After(time.Second):
						t.Fatalf("connection cleanup stalled after %s", ending)
					}
				}
			})
		}
	}
}
