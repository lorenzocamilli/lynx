package intercept_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/filter"
	"github.com/lorenzocamilli/lynx/pkg/proxy"
	"github.com/lorenzocamilli/lynx/pkg/proxy/intercept"
)

type upgradeStream struct {
	t      *testing.T
	closed bool
}

func (s *upgradeStream) Read([]byte) (int, error) {
	s.t.Error("interception consumed upgraded stream")
	return 0, io.EOF
}
func (s *upgradeStream) Write(p []byte) (int, error) { return len(p), nil }
func (s *upgradeStream) Close() error {
	s.closed = true
	return nil
}

func TestUpgradeBypassesResponseInterception(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		for _, override := range []string{"unset", "yes", "no"} {
			t.Run(fmt.Sprintf("enabled=%t/override=%s", enabled, override), func(t *testing.T) {
				svc := intercept.NewService(intercept.Config{
					ResponsesEnabled: enabled,
					ResponseFilter:   filter.StringLiteral{Value: "never matches"},
				})
				req := httptest.NewRequest(http.MethodGet, "http://example.com/upgrade", nil)
				ctx := proxy.WithRequestID(req.Context(), ulid.ULID{1})
				if override != "unset" {
					ctx = intercept.WithInterceptResponse(ctx, override == "yes")
				}
				req = req.WithContext(ctx)
				req.GetBody = func() (io.ReadCloser, error) {
					t.Error("upgrade interception tried to reconstruct sent request body")
					return http.NoBody, nil
				}
				stream := &upgradeStream{t: t}
				res := &http.Response{Request: req, StatusCode: http.StatusSwitchingProtocols, Status: "101 Switching Protocols", Body: stream, Header: http.Header{"Upgrade": {"websocket"}}}
				if got, err := svc.InterceptResponse(ctx, res); err != nil || got != res {
					t.Fatalf("InterceptResponse = %p, %v", got, err)
				}
				called := false
				if err := svc.ResponseModifier(func(got *http.Response) error {
					called = true
					if got != res {
						t.Error("next middleware received another response")
					}
					return nil
				})(res); err != nil {
					t.Fatal(err)
				}
				if !called || res.Body != stream || stream.closed || len(svc.Items()) != 0 {
					t.Fatal("upgrade response was captured, queued, closed or stopped middleware traversal")
				}
				if _, ok := res.Body.(io.ReadWriteCloser); !ok {
					t.Fatal("upgrade stream lost write capability")
				}
				_ = res.Body.Close()
				if !stream.closed {
					t.Fatal("connection owner could not close stream")
				}
			})
		}
	}
}
