package proxy_test

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
)

// testLogger routes the proxy's internal error/debug logs to t.Log, so a
// failing test shows *why* (e.g. a TLS handshake error inside handleConnect)
// instead of just the opaque client-side symptom.
type testLogger struct{ t *testing.T }

func (l testLogger) Debugw(msg string, v ...interface{}) { l.t.Logf("DEBUG: %s %v", msg, v) }
func (l testLogger) Infow(msg string, v ...interface{})  { l.t.Logf("INFO: %s %v", msg, v) }
func (l testLogger) Errorw(msg string, v ...interface{}) { l.t.Logf("ERROR: %s %v", msg, v) }

// TestHandleConnect exercises the full client-facing MITM path end-to-end:
// hijacking a CONNECT tunnel, generating an on-the-fly leaf certificate for
// the SNI hostname, and completing a real TLS handshake presenting it — using
// Go's own http.Transport CONNECT-proxy support (the same mechanism a real
// browser uses via its Proxy field), not a hand-rolled protocol client.
//
// The upstream (proxy -> origin) leg isn't exercised here: the proxy's
// internal outbound *http.Transport has no hook to inject a custom RootCAs
// pool, so it can't be pointed at a test origin without a real, trusted TLS
// cert. Instead the tunnel targets an address nothing listens on, so the
// proxy's own upstream dial fails fast and errorHandler writes a 502 back
// through the tunnel — which also confirms the response path survives a
// dial failure without hanging or crashing. The plain-HTTP round trip
// (modifyRequest/modifyResponse/gzip) is already covered by TestServeHTTP
// against a real origin.
func TestHandleConnect(t *testing.T) {
	caCert, caKey, err := proxy.NewCA("Test CA", "Test", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	p, err := proxy.NewProxy(proxy.Config{CACert: caCert, CAKey: caKey, Logger: testLogger{t}})
	if err != nil {
		t.Fatalf("failed to create proxy: %v", err)
	}

	proxyServer := httptest.NewServer(p)
	defer proxyServer.Close()

	proxyURL, err := url.Parse(proxyServer.URL)
	if err != nil {
		t.Fatalf("failed to parse proxy URL: %v", err)
	}

	caPool := x509.NewCertPool()
	caPool.AddCert(caCert)

	client := &http.Client{
		Transport: &http.Transport{
			Proxy:           http.ProxyURL(proxyURL),
			TLSClientConfig: &tls.Config{RootCAs: caPool}, //nolint:gosec // test CA pool, not InsecureSkipVerify
		},
		Timeout: 5 * time.Second,
	}

	// "localhost", not an IP literal: crypto/tls (correctly, per RFC 6066)
	// only sends SNI for DNS-style names, and cert.go's GetCertificate
	// requires SNI — an IP-literal target here would fail the handshake for
	// reasons unrelated to what this test is verifying. Port 1: nothing
	// listens there, so the proxy's own upstream dial fails immediately and
	// deterministically instead of timing out.
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, "https://localhost:1/", nil)
	if err != nil {
		t.Fatalf("failed to build request: %v", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("unexpected error completing the CONNECT + TLS round trip: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusBadGateway {
		t.Errorf("expected 502 from the proxy's upstream-dial error handler, got %d", resp.StatusCode)
	}

	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		t.Fatal("expected TLS connection state with the on-the-fly leaf certificate")
	}

	leaf := resp.TLS.PeerCertificates[0]

	if got, want := leaf.Subject.CommonName, "localhost"; got != want {
		t.Errorf("expected the on-the-fly leaf cert CommonName %q, got %q", want, got)
	}

	if got, want := leaf.Issuer.CommonName, "Test CA"; got != want {
		t.Errorf("expected the leaf cert to be issued by %q, got %q", want, got)
	}

	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "localhost" {
		t.Errorf("expected leaf cert DNSNames [localhost], got %v", leaf.DNSNames)
	}
}
