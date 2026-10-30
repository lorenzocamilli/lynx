package intercept_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
)

func pendingRequest(t *testing.T, svc *intercept.Service, body string) (ulid.ULID, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	id := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
	req := httptest.NewRequest(http.MethodPost, "https://example.com/original", strings.NewReader(body))
	req.Header.Set("X-Test", "original")
	req = req.WithContext(proxy.WithRequestID(ctx, id))
	done := make(chan error, 1)
	go func() {
		_, err := svc.InterceptRequest(req.Context(), req)
		done <- err
	}()
	waitForPendingItem(t, svc)
	return id, done
}

func waitInterceptResult(t *testing.T, done <-chan error, expected error) {
	t.Helper()
	select {
	case err := <-done:
		if !errors.Is(err, expected) {
			t.Fatalf("intercept result = %v, want %v", err, expected)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interception did not complete")
	}
}

func TestPendingItemsOwnRequestBodiesAndMetadata(t *testing.T) {
	svc := intercept.NewService(intercept.Config{RequestsEnabled: true})
	id, done := pendingRequest(t, svc, "request payload")
	first, err := svc.ItemByID(id)
	if err != nil {
		t.Fatal(err)
	}
	first.Request.Header.Set("X-Test", "changed")
	first.Request.URL.Path = "/changed"
	_, _ = io.ReadAll(first.Request.Body)
	_ = first.Request.Body.Close()
	second := svc.Items()[0].Request
	body, err := io.ReadAll(second.Body)
	if err != nil || string(body) != "request payload" || second.URL.Path != "/original" || second.Header.Get("X-Test") != "original" {
		t.Fatalf("reader mutation leaked into pending request: body=%q error=%v request=%+v", body, err, second)
	}
	if err := svc.CancelRequest(id); err != nil {
		t.Fatal(err)
	}
	waitInterceptResult(t, done, intercept.ErrRequestAborted)
}

func TestConcurrentRequestReadersAndForwarding(t *testing.T) {
	svc := intercept.NewService(intercept.Config{RequestsEnabled: true})
	id, done := pendingRequest(t, svc, "shared payload")
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				item, err := svc.ItemByID(id)
				if err != nil {
					t.Error(err)
					return
				}
				match, err := intercept.MatchRequestFilter(item.Request, filter.StringLiteral{Value: "shared payload"})
				if err != nil || !match {
					t.Errorf("reader lost request body: match=%t, error=%v", match, err)
					return
				}
				body, err := io.ReadAll(item.Request.Body)
				if err != nil || string(body) != "shared payload" {
					t.Errorf("reader body = %q, %v", body, err)
					return
				}
				item.Request.Header.Set("X-Reader", "private")
			}
		}()
	}
	wg.Wait()
	item, err := svc.ItemByID(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ModifyRequest(id, item.Request, nil); err != nil {
		t.Fatal(err)
	}
	waitInterceptResult(t, done, nil)
	// A retained UI snapshot must remain usable even after queue cleanup.
	body, err := io.ReadAll(item.Request.Body)
	if err != nil || string(body) != "shared payload" {
		t.Fatalf("forwarding consumed the caller's body: %q, %v", body, err)
	}
}

func TestPendingResponseAndRelatedRequestAreIndependentSnapshots(t *testing.T) {
	svc := intercept.NewService(intercept.Config{ResponsesEnabled: true})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	id := ulid.ULID{1}
	req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
	req = req.WithContext(proxy.WithRequestID(ctx, id))
	req.Header.Set("X-Test", "original")
	req.GetBody = func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader("sent request")), nil }
	originalBody := &trackedBody{Reader: strings.NewReader("response payload")}
	res := &http.Response{Request: req, Proto: "HTTP/1.1", Status: "200 OK", Header: http.Header{"X-Test": {"original"}}, Body: originalBody}
	done := make(chan error, 1)
	go func() { _, err := svc.InterceptResponse(req.Context(), res); done <- err }()
	waitForPendingItem(t, svc)
	if originalBody.closes != 1 {
		t.Fatalf("original response body closed %d times", originalBody.closes)
	}
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 30; j++ {
				item, err := svc.ItemByID(id)
				if err != nil {
					t.Error(err)
					return
				}
				body, err := io.ReadAll(item.Response.Body)
				if err != nil || string(body) != "response payload" {
					t.Errorf("response body = %q, %v", body, err)
				}
				sent, err := io.ReadAll(item.Response.Request.Body)
				if err != nil || string(sent) != "sent request" {
					t.Errorf("related request body = %q, %v", sent, err)
				}
				if item.Response.Request.Header.Get("X-Test") != "original" {
					t.Error("related request metadata was shared")
				}
				item.Response.Request.Header.Set("X-Test", "changed")
				item.Response.Header.Set("X-Test", "changed")
			}
		}()
	}
	wg.Wait()
	if err := svc.CancelResponse(id); err != nil {
		t.Fatal(err)
	}
	waitInterceptResult(t, done, intercept.ErrRequestAborted)
}

type blockedBody struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
	io.Reader
}

func (b *blockedBody) Read(p []byte) (int, error) {
	b.once.Do(func() { close(b.started) })
	<-b.release
	return b.Reader.Read(p)
}
func (b *blockedBody) Close() error { return nil }

func TestDisableCannotMissAnAdmissionPreparingItsBody(t *testing.T) {
	for _, response := range []bool{false, true} {
		name := "request"
		if response {
			name = "response"
		}
		t.Run(name, func(t *testing.T) {
			expr, err := filter.ParseQuery(`body = "payload"`)
			if err != nil {
				t.Fatal(err)
			}
			svc := intercept.NewService(intercept.Config{RequestsEnabled: !response, ResponsesEnabled: response, RequestFilter: expr, ResponseFilter: expr})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
			req = req.WithContext(proxy.WithRequestID(ctx, ulid.ULID{1}))
			body := &blockedBody{started: make(chan struct{}), release: make(chan struct{}), Reader: strings.NewReader("payload")}
			done := make(chan error, 1)
			go func() {
				if response {
					_, err := svc.InterceptResponse(req.Context(), &http.Response{Request: req, Header: http.Header{}, Body: body})
					done <- err
				} else {
					req.Body = body
					_, err := svc.InterceptRequest(req.Context(), req)
					done <- err
				}
			}()
			select {
			case <-body.started:
			case <-time.After(2 * time.Second):
				t.Fatal("body preparation did not start")
			}
			settingsDone := make(chan struct{})
			go func() { svc.UpdateSettings(intercept.Settings{}); close(settingsDone) }()
			select {
			case <-settingsDone:
			case <-time.After(2 * time.Second):
				close(body.release)
				t.Fatal("settings waited for body I/O")
			}
			close(body.release)
			waitInterceptResult(t, done, nil)
			if len(svc.Items()) != 0 {
				t.Fatal("disabled interception admitted a stale pending item")
			}
		})
	}
}

func TestConcurrentSettingsAndBypassedTraffic(t *testing.T) {
	svc := intercept.NewService(intercept.Config{})
	expr := filter.StringLiteral{Value: "never matches"}
	var wg sync.WaitGroup
	for worker := 0; worker < 4; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				svc.UpdateSettings(intercept.Settings{RequestsEnabled: i%2 == 0, ResponsesEnabled: i%2 == 0, RequestFilter: expr, ResponseFilter: expr})
				req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
				req = req.WithContext(proxy.WithRequestID(req.Context(), ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)))
				if _, err := svc.InterceptRequest(req.Context(), req); err != nil {
					t.Error(err)
				}
				if _, err := svc.InterceptResponse(req.Context(), &http.Response{Request: req, Status: "200 OK", Header: http.Header{}, Body: http.NoBody}); err != nil {
					t.Error(err)
				}
			}
		}()
	}
	wg.Wait()
}

func TestConcurrentClearCancelAndForward(t *testing.T) {
	for _, response := range []bool{false, true} {
		name := "request"
		if response {
			name = "response"
		}
		t.Run(name, func(t *testing.T) {
			for attempt := 0; attempt < 20; attempt++ {
				svc := intercept.NewService(intercept.Config{RequestsEnabled: !response, ResponsesEnabled: response})
				ctx, cancel := context.WithCancel(context.Background())
				req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
				id := ulid.ULID{1}
				req = req.WithContext(proxy.WithRequestID(ctx, id))
				done := make(chan error, 1)
				go func() {
					if response {
						_, err := svc.InterceptResponse(req.Context(), &http.Response{Request: req, Status: "200 OK", Header: http.Header{}, Body: io.NopCloser(strings.NewReader("body"))})
						done <- err
					} else {
						_, err := svc.InterceptRequest(req.Context(), req)
						done <- err
					}
				}()
				waitForPendingItem(t, svc)
				var wg sync.WaitGroup
				for worker := 0; worker < 6; worker++ {
					wg.Add(1)
					go func(worker int) {
						defer wg.Done()
						switch worker {
						case 0:
							svc.ClearRequests()
							svc.ClearResponses()
						case 1:
							svc.UpdateSettings(intercept.Settings{})
						case 2:
							cancel()
						case 3, 4:
							if response {
								_ = svc.ModifyResponse(id, &http.Response{Status: "200 OK", Header: http.Header{}, Body: http.NoBody})
							} else {
								// A mutation owns its input during the call, so each
								// competing caller must provide a distinct object.
								_ = svc.ModifyRequest(id, httptest.NewRequest(http.MethodGet, "https://example.com/edited", nil), nil)
							}
						case 5:
							_ = svc.Items()
						}
					}(worker)
				}
				wg.Wait()
				select {
				case err := <-done:
					if err != nil && !errors.Is(err, intercept.ErrRequestAborted) && !errors.Is(err, context.Canceled) {
						t.Error(err)
					}
				case <-time.After(2 * time.Second):
					t.Fatal("resolution deadlocked")
				}
				cancel()
				if len(svc.Items()) != 0 {
					t.Fatal("completed item remained in queue")
				}
			}
		})
	}
}

type trackedBody struct {
	io.Reader
	closes int
}

func (b *trackedBody) Close() error { b.closes++; return nil }

type failingReader struct{ err error }

func (r failingReader) Read([]byte) (int, error) { return 0, r.err }

func TestBodyCaptureClosesOriginalAndErrorsFailClosed(t *testing.T) {
	failure := errors.New("read failed")
	for _, fail := range []bool{false, true} {
		name := "success"
		if fail {
			name = "error"
		}
		t.Run(name, func(t *testing.T) {
			svc := intercept.NewService(intercept.Config{ResponsesEnabled: true})
			body := &trackedBody{Reader: strings.NewReader("payload")}
			if fail {
				body.Reader = failingReader{failure}
			}
			req := httptest.NewRequest(http.MethodPost, "https://example.com", nil)
			req.Body = body
			id := ulid.ULID{1}
			req = req.WithContext(proxy.WithRequestID(req.Context(), id))
			called := false
			svc.RequestModifier(func(*http.Request) { called = true })(req)
			if body.closes != 1 {
				t.Fatalf("original body closed %d times", body.closes)
			}
			if fail {
				if called || req.Context().Err() == nil {
					t.Fatal("capture failure did not cancel forwarding")
				}
				if got, ok := proxy.RequestIDFromContext(req.Context()); !ok || got != id {
					t.Fatal("cancellation discarded request identity")
				}
			} else if !called || req.Context().Err() != nil {
				t.Fatal("successful capture prevented forwarding")
			}
		})
	}
}

func TestResponseOverrideAndClosedLiveRequestBody(t *testing.T) {
	for _, override := range []bool{false, true} {
		name := "explicit bypass"
		if override {
			name = "explicit intercept"
		}
		t.Run(name, func(t *testing.T) {
			svc := intercept.NewService(intercept.Config{ResponsesEnabled: !override})
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ctx = intercept.WithInterceptResponse(proxy.WithRequestID(ctx, ulid.ULID{1}), override)
			req := httptest.NewRequest(http.MethodGet, "https://example.com", nil).WithContext(ctx)
			// There is no replay reader for a request sent before interception
			// was enabled. Inspect metadata; never reread a closed live body.
			req.Body = &trackedBody{Reader: failingReader{errors.New("closed live stream")}}
			res := &http.Response{Request: req, Status: "200 OK", Header: http.Header{}, Body: http.NoBody}
			done := make(chan error, 1)
			go func() { _, err := svc.InterceptResponse(ctx, res); done <- err }()
			if override {
				waitForPendingItem(t, svc)
				item, err := svc.ItemByID(ulid.ULID{1})
				if err != nil {
					t.Fatal(err)
				}
				if item.Response.Request.Body != nil {
					t.Fatal("unavailable sent body was exposed as a live reader")
				}
				svc.ClearResponses()
				waitInterceptResult(t, done, intercept.ErrRequestAborted)
			} else {
				waitInterceptResult(t, done, nil)
			}
		})
	}
}
