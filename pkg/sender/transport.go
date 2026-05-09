package sender

import (
	"crypto/tls"
	"net"
	"net/http"
	"time"
)

type HTTPTransport struct{}

type protoCtxKey struct{}

const (
	HTTPProto10 = "HTTP/1.0"
	HTTPProto11 = "HTTP/1.1"
	HTTPProto20 = "HTTP/2.0"
)

var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

// h1OnlyTransport sends HTTP/1.x requests directly, bypassing any system proxy.
var h1OnlyTransport = &http.Transport{
	Proxy:       nil,
	DialContext: dialer.DialContext,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	},
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,

	// Disable HTTP/2.
	TLSNextProto: map[string]func(string, *tls.Conn) http.RoundTripper{},
}

// h2Transport sends HTTP/2 requests directly, bypassing any system proxy.
var h2Transport = &http.Transport{
	Proxy:       nil,
	DialContext: dialer.DialContext,
	TLSClientConfig: &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
	},
	ForceAttemptHTTP2:     true,
	MaxIdleConns:          100,
	IdleConnTimeout:       90 * time.Second,
	TLSHandshakeTimeout:   10 * time.Second,
	ExpectContinueTimeout: 1 * time.Second,
}

// RoundTrip implements http.RoundTripper. Based on a context value on the
// HTTP request, it switches between an HTTP/1.1-only transport and an
// HTTP/2-capable transport. Both bypass any system proxy settings.
func (t *HTTPTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	proto, ok := req.Context().Value(protoCtxKey{}).(string)

	if ok && proto == HTTPProto10 || proto == HTTPProto11 {
		return h1OnlyTransport.RoundTrip(req)
	}

	return h2Transport.RoundTrip(req)
}

func isValidProto(proto string) bool {
	return proto == HTTPProto10 || proto == HTTPProto11 || proto == HTTPProto20
}
