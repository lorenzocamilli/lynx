package api

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
)

func TestConcurrentInterceptQueriesAndForwarding(t *testing.T) {
	for _, response := range []bool{false, true} {
		name := "request"
		if response {
			name = "response"
		}
		t.Run(name, func(t *testing.T) {
			svc := intercept.NewService(intercept.Config{RequestsEnabled: !response, ResponsesEnabled: response})
			query := (&Resolver{InterceptService: svc}).Query()
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			id := ulid.ULID{1}
			req := httptest.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("request payload"))
			req = req.WithContext(proxy.WithRequestID(ctx, id))
			if response {
				// Response-only interception must preserve a replay before the
				// transport consumes/closes the live request body.
				if _, err := svc.InterceptRequest(req.Context(), req); err != nil {
					t.Fatal(err)
				}
				_, _ = io.Copy(io.Discard, req.Body)
				_ = req.Body.Close()
			}
			done := make(chan struct{})
			go func() {
				defer close(done)
				if response {
					res := &http.Response{Request: req, Proto: "HTTP/1.1", Status: "200 OK", StatusCode: 200, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("response payload"))}
					if err := svc.ResponseModifier(func(forwarded *http.Response) error {
						forwarded.Header.Set("X-Forwarded", "private")
						_, err := io.Copy(io.Discard, forwarded.Body)
						return err
					})(res); err != nil {
						t.Error(err)
					}
				} else {
					svc.RequestModifier(func(forwarded *http.Request) {
						forwarded.Header.Set("X-Forwarded", "private")
						_, _ = io.Copy(io.Discard, forwarded.Body)
					})(req)
				}
			}()
			deadline := time.Now().Add(2 * time.Second)
			for len(svc.Items()) == 0 {
				if time.Now().After(deadline) {
					t.Fatal("interception did not register")
				}
				time.Sleep(time.Millisecond)
			}
			ready := make(chan struct{}, 8)
			resume := make(chan struct{})
			var wg sync.WaitGroup
			for i := 0; i < 8; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					// Retain a view across forwarding/cleanup and parse it after
					// forwarding starts. No view may alias the live HTTP object.
					item, err := svc.ItemByID(id)
					if err != nil {
						t.Error(err)
					}
					ready <- struct{}{}
					<-resume
					for j := 0; j < 30; j++ {
						parsed, err := parseInterceptItem(item)
						if err != nil || parsed.Body == nil || *parsed.Body != "request payload" {
							t.Errorf("retained request body changed: %+v, %v", parsed, err)
							return
						}
						if response && (parsed.Response == nil || parsed.Response.Body == nil || *parsed.Response.Body != "response payload") {
							t.Error("retained response body changed")
							return
						}
						if _, err := query.InterceptedRequests(ctx); err != nil {
							t.Error(err)
						}
						if _, err := query.InterceptedRequest(ctx, id); err != nil {
							t.Error(err)
						}
					}
				}()
			}
			for i := 0; i < 8; i++ {
				<-ready
			}
			item, err := svc.ItemByID(id)
			if err != nil {
				t.Fatal(err)
			}
			close(resume)
			if response {
				err = svc.ModifyResponse(id, item.Response)
			} else {
				err = svc.ModifyRequest(id, item.Request, nil)
			}
			if err != nil {
				t.Fatal(err)
			}
			wg.Wait()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("forwarding did not finish")
			}
		})
	}
}
