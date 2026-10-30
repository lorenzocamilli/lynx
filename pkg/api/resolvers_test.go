package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/oklog/ulid"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
)

func TestParseHTTPRequestProtocol(t *testing.T) {
	for _, protocol := range []struct {
		wire string
		api  HTTPProtocol
	}{
		{"HTTP/1.0", HTTPProtocolHTTP10},
		{"HTTP/1.1", HTTPProtocolHTTP11},
		{"HTTP/2.0", HTTPProtocolHTTP20},
	} {
		t.Run(protocol.wire, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "https://example.com", nil)
			req.Proto = protocol.wire
			req = req.WithContext(proxy.WithRequestID(req.Context(), ulid.ULID{1}))
			parsed, err := parseHTTPRequest(req)
			if err != nil {
				t.Fatal(err)
			}
			if parsed.Proto != protocol.api || !parsed.Proto.IsValid() {
				t.Fatalf("protocol = %q, want GraphQL enum %q", parsed.Proto, protocol.api)
			}
		})
	}
}
