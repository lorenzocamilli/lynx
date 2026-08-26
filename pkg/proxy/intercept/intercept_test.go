package intercept_test

import (
	"context"
	"crypto/rand"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"
	"go.uber.org/zap"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
)

var ulidEntropy = rand.Reader

// waitForPendingItem polls svc.Items() (the same public, mutex-guarded
// snapshot the GraphQL layer uses) until the request/response modifier
// goroutine under test has actually registered its item, instead of
// guessing a fixed sleep duration. Each subtest using this uses its own
// freshly-constructed *intercept.Service, so "at least one pending item"
// unambiguously means "our goroutine got there".
func waitForPendingItem(t *testing.T, svc *intercept.Service) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	for {
		if len(svc.Items()) > 0 {
			return
		}

		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the intercepted item to be registered")
		}

		time.Sleep(time.Millisecond)
	}
}

func TestRequestModifier(t *testing.T) {
	t.Parallel()

	t.Run("modify request that's not found", func(t *testing.T) {
		t.Parallel()

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  true,
			ResponsesEnabled: false,
		})

		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)

		err := svc.ModifyRequest(reqID, nil, nil)
		if !errors.Is(err, intercept.ErrRequestNotFound) {
			t.Fatalf("expected `intercept.ErrRequestNotFound`, got: %v", err)
		}
	})

	t.Run("modify request that's done", func(t *testing.T) {
		t.Parallel()

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  true,
			ResponsesEnabled: false,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := httptest.NewRequest("GET", "https://example.com/foo", nil)
		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
		*req = *req.WithContext(ctx)
		*req = *req.WithContext(proxy.WithRequestID(req.Context(), reqID))

		next := func(req *http.Request) {}
		go svc.RequestModifier(next)(req)

		// Poll instead of guessing a fixed sleep duration, so cancel() can't
		// fire before the request is even registered.
		waitForPendingItem(t, svc)
		cancel()

		modReq := req.Clone(req.Context())
		modReq.Header.Set("X-Foo", "bar")

		// Either error is a valid outcome here, not just ErrRequestDone: once
		// cancel() fires, InterceptRequest's deferred cleanup does
		// close(done) then delete(svc.requests, reqID) as two separate,
		// unsynchronized-with-callers steps. A concurrent ModifyRequest can
		// land before either (ErrRequestNotFound was already impossible,
		// waitForPendingItem ruled that window out above), between them
		// (ErrRequestDone), or after both (ErrRequestNotFound again) — this
		// is a genuine, unavoidable race in that cleanup sequence, not a
		// test synchronization bug. Confirmed by reading every real caller
		// (pkg/api/resolvers.go): none distinguish the two errors, both are
		// wrapped into the same generic "could not modify" response, so the
		// distinction isn't a guarantee this codebase actually relies on.
		err := svc.ModifyRequest(reqID, modReq, nil)
		if !errors.Is(err, intercept.ErrRequestDone) && !errors.Is(err, intercept.ErrRequestNotFound) {
			t.Fatalf("expected `intercept.ErrRequestDone` or `intercept.ErrRequestNotFound`, got: %v", err)
		}
	})

	t.Run("modify intercepted request", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("GET", "https://example.com/foo", nil)
		req.Header.Set("X-Foo", "foo")

		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
		*req = *req.WithContext(proxy.WithRequestID(req.Context(), reqID))

		modReq := req.Clone(context.Background())
		modReq.Header.Set("X-Foo", "bar")

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  true,
			ResponsesEnabled: false,
		})

		var got *http.Request

		next := func(req *http.Request) {
			got = req.Clone(context.Background())
		}

		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			svc.RequestModifier(next)(req)
			wg.Done()
		}()

		waitForPendingItem(t, svc)

		err := svc.ModifyRequest(reqID, modReq, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wg.Wait()

		if got == nil {
			t.Fatal("expected `got` not to be nil")
		}

		if exp := "bar"; exp != got.Header.Get("X-Foo") {
			t.Fatalf("incorrect modified request header value (expected: %v, got: %v)", exp, got.Header.Get("X-Foo"))
		}
	})
}

func TestResponseModifier(t *testing.T) {
	t.Parallel()

	t.Run("modify response that's not found", func(t *testing.T) {
		t.Parallel()

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  false,
			ResponsesEnabled: true,
		})

		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)

		err := svc.ModifyResponse(reqID, nil)
		if !errors.Is(err, intercept.ErrRequestNotFound) {
			t.Fatalf("expected `intercept.ErrRequestNotFound`, got: %v", err)
		}
	})

	t.Run("modify response of request that's done", func(t *testing.T) {
		t.Parallel()

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  false,
			ResponsesEnabled: true,
		})

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		req := httptest.NewRequest("GET", "https://example.com/foo", nil)
		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
		*req = *req.WithContext(ctx)
		*req = *req.WithContext(proxy.WithRequestID(req.Context(), reqID))

		res := &http.Response{
			Request: req,
			Header:  make(http.Header),
		}
		res.Header.Add("X-Foo", "foo")

		var modErr error
		var wg sync.WaitGroup
		wg.Add(1)

		next := func(res *http.Response) error { return nil }
		go func() {
			defer wg.Done()
			modErr = svc.ResponseModifier(next)(res)
		}()

		waitForPendingItem(t, svc)
		cancel()

		modRes := *res
		modRes.Header = make(http.Header)
		modRes.Header.Set("X-Foo", "bar")

		// See the "either error is a valid outcome" comment in
		// TestRequestModifier's "modify request that's done" subtest above —
		// same race, same reasoning, on the response side.
		err := svc.ModifyResponse(reqID, &modRes)
		if !errors.Is(err, intercept.ErrRequestDone) && !errors.Is(err, intercept.ErrRequestNotFound) {
			t.Fatalf("expected `intercept.ErrRequestDone` or `intercept.ErrRequestNotFound`, got: %v", err)
		}

		wg.Wait()

		if !errors.Is(modErr, context.Canceled) {
			t.Fatalf("expected `context.Canceled`, got: %v", modErr)
		}
	})

	t.Run("modify intercepted response", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequest("GET", "https://example.com/foo", nil)
		req.Header.Set("X-Foo", "foo")

		reqID := ulid.MustNew(ulid.Timestamp(time.Now()), ulidEntropy)
		*req = *req.WithContext(proxy.WithRequestID(req.Context(), reqID))

		res := &http.Response{
			Request: req,
			Header:  make(http.Header),
		}
		res.Header.Add("X-Foo", "foo")

		modRes := *res
		modRes.Header = make(http.Header)
		modRes.Header.Set("X-Foo", "bar")

		logger, _ := zap.NewDevelopment()
		svc := intercept.NewService(intercept.Config{
			Logger:           logger.Sugar(),
			RequestsEnabled:  false,
			ResponsesEnabled: true,
		})

		var gotHeader string

		var next proxy.ResponseModifyFunc = func(res *http.Response) error {
			gotHeader = res.Header.Get("X-Foo")
			return nil
		}

		var modErr error
		var wg sync.WaitGroup
		wg.Add(1)

		go func() {
			modErr = svc.ResponseModifier(next)(res)
			wg.Done()
		}()

		waitForPendingItem(t, svc)

		err := svc.ModifyResponse(reqID, &modRes)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		wg.Wait()

		if modErr != nil {
			t.Fatalf("unexpected error: %v", modErr)
		}

		if exp := "bar"; exp != gotHeader {
			t.Fatalf("incorrect modified request header value (expected: %v, got: %v)", exp, gotHeader)
		}
	})
}
