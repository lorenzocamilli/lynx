// Package integration_test drives a real HTTP request through the same
// service wiring cmd/lynx/lynx.go builds at startup (proxy -> reqlog ->
// intercept -> upstream, then a Sender replay), using the public API of each
// pkg/* package rather than mocks. It cannot import cmd/lynx directly: that
// package's run() hardcodes ~/.lynx/* paths and calls logger.Fatal (os.Exit)
// on setup errors, so this test reconstructs the same wiring against a
// temp-dir database and an httptest origin instead.
package integration_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/lorenzocamilli/lynx/pkg/db/bolt"
	"github.com/lorenzocamilli/lynx/pkg/proj"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
	"github.com/lorenzocamilli/lynx/pkg/reqlog"
	"github.com/lorenzocamilli/lynx/pkg/scope"
	"github.com/lorenzocamilli/lynx/pkg/sender"
)

// waitFor polls fn until it returns true or the deadline passes, failing the
// test on timeout. Used instead of a fixed sleep to wait on the async bits
// of the chain under test: the intercept goroutine registering a pending
// request, and reqlog's response-storage goroutine.
func waitFor(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for {
		if fn() {
			return
		}

		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for condition")
		}

		time.Sleep(time.Millisecond)
	}
}

const originBody = "hello from origin"

// TestEndToEndRequestFlow proxies a real HTTP request through the full
// modifier chain (reqlog -> intercept -> upstream, mirroring the
// registration order in cmd/lynx/lynx.go), approves it out of the intercept
// queue the same way a GraphQL mutation would, confirms both the request and
// (asynchronously stored) response were logged, then clones the logged
// request into Sender and replays it, confirming Sender gets an independent
// response from the same origin.
func TestEndToEndRequestFlow(t *testing.T) {
	ctx := context.Background()

	var gotPath string

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path

		w.Header().Set("Content-Type", "text/plain")
		w.WriteHeader(http.StatusTeapot)
		_, _ = io.WriteString(w, originBody)
	}))
	defer origin.Close()

	dbPath := filepath.Join(t.TempDir(), "lynx.db")

	db, err := bolt.OpenDatabase(dbPath, nil)
	if err != nil {
		t.Fatalf("failed to open database: %v", err)
	}
	defer db.Close()

	scopeSvc := &scope.Scope{}
	reqLogSvc := reqlog.NewService(reqlog.Config{Scope: scopeSvc, Repository: db})
	interceptSvc := intercept.NewService(intercept.Config{})
	senderSvc := sender.NewService(sender.Config{Scope: scopeSvc, Repository: db, ReqLogService: reqLogSvc})

	projSvc, err := proj.NewService(proj.Config{
		Repository:       db,
		InterceptService: interceptSvc,
		ReqLogService:    reqLogSvc,
		SenderService:    senderSvc,
		Scope:            scopeSvc,
	})
	if err != nil {
		t.Fatalf("failed to create proj service: %v", err)
	}

	project, err := projSvc.CreateProject(ctx, "integration test project")
	if err != nil {
		t.Fatalf("CreateProject failed: %v", err)
	}

	if _, err := projSvc.OpenProject(ctx, project.ID); err != nil {
		t.Fatalf("OpenProject failed: %v", err)
	}

	// Enable request interception through the same path the admin UI uses,
	// so this exercises proj -> intercept wiring too, not just proxy -> reqlog.
	if err := projSvc.UpdateInterceptSettings(ctx, intercept.Settings{RequestsEnabled: true}); err != nil {
		t.Fatalf("UpdateInterceptSettings failed: %v", err)
	}

	caCert, caKey, err := proxy.NewCA("Integration Test CA", "Lynx", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	p, err := proxy.NewProxy(proxy.Config{CACert: caCert, CAKey: caKey})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	// Registration order mirrors cmd/lynx/lynx.go exactly: reqlog outermost,
	// intercept innermost, so a paused request has already been logged.
	p.UseRequestModifier(reqLogSvc.RequestModifier)
	p.UseResponseModifier(reqLogSvc.ResponseModifier)
	p.UseRequestModifier(interceptSvc.RequestModifier)
	p.UseResponseModifier(interceptSvc.ResponseModifier)

	req := httptest.NewRequest(http.MethodGet, origin.URL+"/foo?bar=baz", nil)
	req.Header.Set("X-Test-Header", "integration")

	rec := httptest.NewRecorder()

	served := make(chan struct{})
	go func() {
		p.ServeHTTP(rec, req)
		close(served)
	}()

	// The request is now blocked in intercept.RequestModifier. Approve it
	// unmodified, exactly like the "Forward" mutation in the admin UI does.
	waitFor(t, 2*time.Second, func() bool { return len(interceptSvc.Items()) > 0 })

	items := interceptSvc.Items()
	if len(items) != 1 || items[0].Request == nil {
		t.Fatalf("expected exactly one pending intercepted request, got %+v", items)
	}

	pendingReq := items[0].Request

	reqID, ok := proxy.RequestIDFromContext(pendingReq.Context())
	if !ok {
		t.Fatal("pending request has no request ID in its context")
	}

	if err := interceptSvc.ModifyRequest(reqID, pendingReq, nil); err != nil {
		t.Fatalf("ModifyRequest failed: %v", err)
	}

	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for the proxied request to complete")
	}

	if rec.Code != http.StatusTeapot {
		t.Fatalf("proxied response status = %d, want %d", rec.Code, http.StatusTeapot)
	}

	if got := rec.Body.String(); got != originBody {
		t.Fatalf("proxied response body = %q, want %q", got, originBody)
	}

	if gotPath != "/foo" {
		t.Fatalf("origin received path %q, want %q", gotPath, "/foo")
	}

	// The request log is written synchronously (before intercept unblocks
	// next()), but the response log is stored by a background goroutine.
	var reqLogs []reqlog.RequestLog

	waitFor(t, 2*time.Second, func() bool {
		reqLogs, err = reqLogSvc.FindRequests(ctx, 10, 0)
		if err != nil {
			t.Fatalf("FindRequests failed: %v", err)
		}

		return len(reqLogs) == 1 && reqLogs[0].Response != nil
	})

	reqLog := reqLogs[0]

	if reqLog.Method != http.MethodGet {
		t.Errorf("logged Method = %q, want %q", reqLog.Method, http.MethodGet)
	}

	if reqLog.URL.Path != "/foo" || reqLog.URL.RawQuery != "bar=baz" {
		t.Errorf("logged URL = %q, want path /foo with query bar=baz", reqLog.URL.String())
	}

	if got := reqLog.Header.Get("X-Test-Header"); got != "integration" {
		t.Errorf("logged request header X-Test-Header = %q, want %q", got, "integration")
	}

	if reqLog.Response.StatusCode != http.StatusTeapot {
		t.Errorf("logged response StatusCode = %d, want %d", reqLog.Response.StatusCode, http.StatusTeapot)
	}

	if string(reqLog.Response.Body) != originBody {
		t.Errorf("logged response Body = %q, want %q", reqLog.Response.Body, originBody)
	}

	// Clone the logged request into Sender and replay it against the same
	// origin, confirming the reqlog -> sender hand-off works end-to-end.
	senderReq, err := senderSvc.CloneFromRequestLog(ctx, reqLog.ID)
	if err != nil {
		t.Fatalf("CloneFromRequestLog failed: %v", err)
	}

	if senderReq.SourceRequestLogID.Compare(reqLog.ID) != 0 {
		t.Errorf("sender request SourceRequestLogID = %v, want %v", senderReq.SourceRequestLogID, reqLog.ID)
	}

	sent, err := senderSvc.SendRequest(ctx, senderReq.ID)
	if err != nil {
		t.Fatalf("SendRequest failed: %v", err)
	}

	if sent.Response == nil {
		t.Fatal("expected Sender's replayed request to have a response")
	}

	if sent.Response.StatusCode != http.StatusTeapot {
		t.Errorf("Sender response StatusCode = %d, want %d", sent.Response.StatusCode, http.StatusTeapot)
	}

	if string(sent.Response.Body) != originBody {
		t.Errorf("Sender response Body = %q, want %q", sent.Response.Body, originBody)
	}
}
