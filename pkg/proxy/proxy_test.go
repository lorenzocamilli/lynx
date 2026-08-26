package proxy_test

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
)

func newTestProxy(t *testing.T) *proxy.Proxy {
	t.Helper()

	caCert, caKey, err := proxy.NewCA("Test CA", "Test", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	p, err := proxy.NewProxy(proxy.Config{CACert: caCert, CAKey: caKey})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	return p
}

func TestServeHTTP(t *testing.T) {
	var gotHeader http.Header

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHeader = r.Header.Clone()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from origin"))
	}))
	defer origin.Close()

	p := newTestProxy(t)

	req := httptest.NewRequest(http.MethodGet, origin.URL+"/path", nil)
	req.Header.Set("X-Forwarded-For", "should-be-stripped")
	req.Header.Set("Accept-Encoding", "gzip, br, identity")

	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}

	if got, want := rec.Body.String(), "hello from origin"; got != want {
		t.Errorf("expected body %q, got %q", want, got)
	}

	if _, ok := gotHeader["X-Forwarded-For"]; ok {
		t.Errorf("expected X-Forwarded-For to be stripped, origin received: %v", gotHeader["X-Forwarded-For"])
	}

	// Only gzip (and wildcard) directives should survive; br/identity are stripped.
	if got, want := gotHeader.Get("Accept-Encoding"), "gzip"; got != want {
		t.Errorf("expected Accept-Encoding %q forwarded to origin, got %q", want, got)
	}
}

func TestServeHTTP_DecompressesGzipResponse(t *testing.T) {
	var buf bytes.Buffer

	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write([]byte("decompressed body")); err != nil {
		t.Fatalf("failed to write gzip body: %v", err)
	}

	if err := gz.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	gzipBody := buf.Bytes()

	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Encoding", "gzip")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(gzipBody)
	}))
	defer origin.Close()

	p := newTestProxy(t)

	req := httptest.NewRequest(http.MethodGet, origin.URL+"/", nil)
	rec := httptest.NewRecorder()
	p.ServeHTTP(rec, req)

	if got, want := rec.Body.String(), "decompressed body"; got != want {
		t.Errorf("expected decompressed body %q, got %q", want, got)
	}

	if enc := rec.Header().Get("Content-Encoding"); enc != "" {
		t.Errorf("expected Content-Encoding header to be removed after decompression, got %q", enc)
	}
}

func TestServeHTTP_RequestIDAvailableToModifiers(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()

	p := newTestProxy(t)

	var sawID bool

	p.UseRequestModifier(func(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
		return func(req *http.Request) {
			_, sawID = proxy.RequestIDFromContext(req.Context())
			next(req)
		}
	})

	req := httptest.NewRequest(http.MethodGet, origin.URL+"/", nil)
	p.ServeHTTP(httptest.NewRecorder(), req)

	if !sawID {
		t.Error("expected a request ID to be present in the modifier's request context")
	}
}

// TestModifierChainOrder empirically pins down the request/response modifier
// execution order, since it's easy to get backwards from reading the
// registration loop in proxy.go alone (fn := reqModifiers[i](fn) iterated in
// reverse builds an onion, and reasoning about which side of a middleware's
// own next(req) call its side effect sits on is exactly the kind of thing
// that should be verified, not assumed — see the 2026-08-26 CLAUDE.md
// correction to a similar assumption about cert caching).
func TestModifierChainOrder(t *testing.T) {
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer origin.Close()

	p := newTestProxy(t)

	var reqOrder []string

	registerReqModifier := func(name string) {
		p.UseRequestModifier(func(next proxy.RequestModifyFunc) proxy.RequestModifyFunc {
			return func(req *http.Request) {
				reqOrder = append(reqOrder, name)
				next(req)
			}
		})
	}

	registerReqModifier("first-registered")
	registerReqModifier("second-registered")

	var resOrder []string

	registerResModifier := func(name string) {
		p.UseResponseModifier(func(next proxy.ResponseModifyFunc) proxy.ResponseModifyFunc {
			return func(res *http.Response) error {
				resOrder = append(resOrder, name)
				return next(res)
			}
		})
	}

	registerResModifier("first-registered")
	registerResModifier("second-registered")

	req := httptest.NewRequest(http.MethodGet, origin.URL+"/", nil)
	p.ServeHTTP(httptest.NewRecorder(), req)

	wantReqOrder := []string{"first-registered", "second-registered"}
	if !equalStrings(reqOrder, wantReqOrder) {
		t.Errorf("request modifier execution order: got %v, want %v (registration order runs outer-to-inner)", reqOrder, wantReqOrder)
	}

	wantResOrder := []string{"first-registered", "second-registered"}
	if !equalStrings(resOrder, wantResOrder) {
		t.Errorf("response modifier execution order: got %v, want %v (registration order runs outer-to-inner)", resOrder, wantResOrder)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}

	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}

	return true
}
