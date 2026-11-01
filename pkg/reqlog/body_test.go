package reqlog

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
)

type trackedBody struct {
	io.Reader
	closed bool
}

func (b *trackedBody) Close() error {
	b.closed = true
	return nil
}

type responseWrite struct {
	projectID ulid.ULID
	log       ResponseLog
}

type captureRepository struct {
	Repository
	request  RequestLog
	response chan responseWrite
}

func (r *captureRepository) StoreRequestLog(_ context.Context, log RequestLog) error {
	r.request = log
	return nil
}

func (r *captureRepository) StoreResponseLog(_ context.Context, projectID, _ ulid.ULID, log ResponseLog) error {
	r.response <- responseWrite{projectID: projectID, log: log}
	return nil
}

func TestCaptureBody(t *testing.T) {
	for _, input := range []string{"", "abc", "abcd", "abcdef"} {
		t.Run(input, func(t *testing.T) {
			original := &trackedBody{Reader: strings.NewReader(input)}
			prefix, restored, err := captureBody(original, 4)
			if err != nil {
				t.Fatal(err)
			}
			want := input
			if len(want) > 4 {
				want = want[:4]
			}
			if string(prefix) != want {
				t.Fatalf("prefix = %q, want %q", prefix, want)
			}
			forwarded, err := io.ReadAll(restored)
			if err != nil || string(forwarded) != input {
				t.Fatalf("forwarded = %q, %v; want %q", forwarded, err, input)
			}
			if err := restored.Close(); err != nil {
				t.Fatal(err)
			}
			if !original.closed {
				t.Fatal("original body was not closed")
			}
		})
	}
}

func TestModifiersPreserveTrafficAndResponseProject(t *testing.T) {
	projectID := ulid.ULID{1}
	repo := &captureRepository{response: make(chan responseWrite, 1)}
	svc := NewService(Config{
		ActiveProjectID: projectID,
		Repository:      repo,
		MaxBodyBytes:    4,
		RedactHeaders:   []string{"Authorization"},
	})
	req := httptest.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("request body"))
	req.Header.Set("Authorization", "secret")
	req = req.WithContext(proxy.WithRequestID(req.Context(), ulid.ULID{2}))
	svc.RequestModifier(func(*http.Request) {})(req)
	if string(repo.request.Body) != "requ" {
		t.Fatalf("logged request = %q, want truncated prefix", repo.request.Body)
	}
	body, err := io.ReadAll(req.Body)
	if err != nil || string(body) != "request body" {
		t.Fatalf("forwarded request = %q, %v", body, err)
	}
	if req.Header.Get("Authorization") != "secret" || repo.request.Header.Get("Authorization") != redactMask {
		t.Fatal("redaction must affect only the stored request")
	}

	svc.SetActiveProjectID(ulid.ULID{3})
	original := &trackedBody{Reader: strings.NewReader("response body")}
	res := &http.Response{Request: req, Body: original, Header: http.Header{}}
	if err := svc.ResponseModifier(func(*http.Response) error { return nil })(res); err != nil {
		t.Fatal(err)
	}
	body, err = io.ReadAll(res.Body)
	if err != nil || string(body) != "response body" {
		t.Fatalf("forwarded response = %q, %v", body, err)
	}
	if err := res.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !original.closed {
		t.Fatal("upstream response body was not closed")
	}
	select {
	case stored := <-repo.response:
		if stored.projectID != projectID || string(stored.log.Body) != "resp" {
			t.Fatalf("response stored in wrong project or with wrong body: %+v", stored)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("response was not stored")
	}
}

func TestRequestModifierWithoutProjectDoesNotConsumeBody(t *testing.T) {
	svc := NewService(Config{MaxBodyBytes: 4})
	original := &trackedBody{Reader: strings.NewReader("unlogged request")}
	req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
	req.Body = original
	svc.RequestModifier(func(*http.Request) {})(req)
	if req.Body != original {
		t.Fatal("logging without an active project must not replace the body")
	}
}

func TestRequestKeepsProjectAcrossNextModifier(t *testing.T) {
	a, b := ulid.ULID{1}, ulid.ULID{2}
	repo := &captureRepository{}
	svc := NewService(Config{Repository: repo, ActiveProjectID: a})
	req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
	req = req.WithContext(proxy.WithRequestID(req.Context(), ulid.ULID{3}))
	svc.RequestModifier(func(*http.Request) {
		// The inner modifier can pause in interception while the user opens
		// another project. Capturing state after it returns is too late.
		svc.SetActiveProjectID(b)
	})(req)
	if repo.request.ProjectID != a {
		t.Fatalf("request stored in %v, want its initial project %v", repo.request.ProjectID, a)
	}
}

// Reading or closing this stream before ReverseProxy takes over is a bug.
type upgradeBody struct {
	t      *testing.T
	closed bool
}

func (b *upgradeBody) Read([]byte) (int, error) {
	b.t.Error("attempted to read upgraded stream as a response document")
	return 0, io.EOF
}
func (b *upgradeBody) Write(p []byte) (int, error) { return len(p), nil }
func (b *upgradeBody) Close() error {
	b.closed = true
	return nil
}

func TestUpgradeLogsMetadataWithoutTouchingStream(t *testing.T) {
	projectID := ulid.ULID{1}
	repo := &captureRepository{response: make(chan responseWrite, 1)}
	svc := NewService(Config{Repository: repo, ActiveProjectID: projectID, RedactHeaders: []string{"Secret"}})
	req := httptest.NewRequest(http.MethodGet, "http://example.com/upgrade", nil)
	req = req.WithContext(proxy.WithRequestID(req.Context(), ulid.ULID{2}))
	svc.RequestModifier(func(*http.Request) {})(req)
	svc.SetActiveProjectID(ulid.ULID{3})
	stream := &upgradeBody{t: t}
	res := &http.Response{
		Request: req, Body: stream, StatusCode: http.StatusSwitchingProtocols,
		Status: "101 Switching Protocols", Proto: "HTTP/1.1",
		Header: http.Header{"Connection": {"Upgrade"}, "Upgrade": {"websocket"}, "Secret": {"value"}, "Content-Encoding": {"gzip"}},
	}
	called := false
	if err := svc.ResponseModifier(func(*http.Response) error { called = true; return nil })(res); err != nil {
		t.Fatal(err)
	}
	if !called || res.Body != stream || stream.closed || res.Header.Get("Secret") != "value" {
		t.Fatal("logger changed upgraded stream or live headers, or skipped the next middleware")
	}
	select {
	case stored := <-repo.response:
		if stored.projectID != projectID || stored.log.StatusCode != 101 || len(stored.log.Body) != 0 || stored.log.Size != 0 || stored.log.Header.Get("Upgrade") != "websocket" || stored.log.Header.Get("Secret") != redactMask || stored.log.Header.Get("Content-Encoding") != "gzip" {
			t.Fatalf("wrong upgrade metadata: %+v", stored)
		}
	case <-time.After(time.Second):
		t.Fatal("metadata logging waited on stream closure")
	}
	if _, ok := res.Body.(io.ReadWriteCloser); !ok {
		t.Fatal("upgrade lost write capability")
	}
	if err := res.Body.Close(); err != nil || !stream.closed {
		t.Fatal("connection owner could not close original stream")
	}
}

func TestParseUpgradeResponseDoesNotReadStream(t *testing.T) {
	stream := &upgradeBody{t: t}
	res := &http.Response{StatusCode: http.StatusSwitchingProtocols, Body: stream, Header: http.Header{"Content-Encoding": {"gzip"}}}
	log, err := ParseHTTPResponse(res)
	if err != nil || len(log.Body) != 0 || log.Size != 0 || stream.closed || log.Header.Get("Content-Encoding") != "gzip" {
		t.Fatalf("parsing upgrade metadata: %+v, %v", log, err)
	}
}
