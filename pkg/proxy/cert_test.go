package proxy_test

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lorenzocamilli/lynx/pkg/proxy"
)

const testHostname = "example.com"

func TestNewCA(t *testing.T) {
	cert, key, err := proxy.NewCA("Test", "Test Org", 24*time.Hour)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if key == nil {
		t.Fatal("expected non-nil private key")
	}

	if !cert.IsCA {
		t.Error("expected IsCA to be true")
	}

	if !cert.BasicConstraintsValid {
		t.Error("expected BasicConstraintsValid to be true")
	}

	if cert.KeyUsage&x509.KeyUsageCertSign == 0 {
		t.Error("expected KeyUsageCertSign to be set")
	}

	if cert.Subject.CommonName != "Test" {
		t.Errorf("expected CommonName %q, got %q", "Test", cert.Subject.CommonName)
	}

	if len(cert.Subject.Organization) != 1 || cert.Subject.Organization[0] != "Test Org" {
		t.Errorf("expected Organization [%q], got %v", "Test Org", cert.Subject.Organization)
	}

	if len(cert.SubjectKeyId) != 20 {
		t.Errorf("expected 20-byte SubjectKeyId (SHA-1), got %d bytes", len(cert.SubjectKeyId))
	}

	if got, want := cert.NotAfter.Sub(cert.NotBefore), 24*time.Hour; got != want {
		t.Errorf("expected validity window %v, got %v", want, got)
	}

	// Self-signed: the CA cert must verify against its own public key.
	if err := cert.CheckSignatureFrom(cert); err != nil {
		t.Errorf("expected CA cert to be self-signed and verify: %v", err)
	}
}

func TestLoadOrCreateCA(t *testing.T) {
	t.Run("creates a new CA when files don't exist", func(t *testing.T) {
		dir := t.TempDir()
		keyFile := filepath.Join(dir, "sub", "key.pem")
		certFile := filepath.Join(dir, "sub", "cert.pem")

		cert, key, err := proxy.LoadOrCreateCA(keyFile, certFile)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !cert.IsCA {
			t.Error("expected IsCA to be true")
		}

		if key == nil {
			t.Fatal("expected non-nil private key")
		}

		info, err := os.Stat(filepath.Dir(keyFile))
		if err != nil {
			t.Fatalf("failed to stat key directory: %v", err)
		}

		if perm := info.Mode().Perm(); perm != 0o700 {
			t.Errorf("expected key directory mode 0700, got %o", perm)
		}

		keyInfo, err := os.Stat(keyFile)
		if err != nil {
			t.Fatalf("failed to stat key file: %v", err)
		}

		if perm := keyInfo.Mode().Perm(); perm != 0o600 {
			t.Errorf("expected key file mode 0600, got %o", perm)
		}
	})

	t.Run("loads the existing CA on a second call instead of regenerating", func(t *testing.T) {
		dir := t.TempDir()
		keyFile := filepath.Join(dir, "key.pem")
		certFile := filepath.Join(dir, "cert.pem")

		cert1, key1, err := proxy.LoadOrCreateCA(keyFile, certFile)
		if err != nil {
			t.Fatalf("unexpected error on first call: %v", err)
		}

		cert2, key2, err := proxy.LoadOrCreateCA(keyFile, certFile)
		if err != nil {
			t.Fatalf("unexpected error on second call: %v", err)
		}

		if cert1.SerialNumber.Cmp(cert2.SerialNumber) != 0 {
			t.Error("expected the same CA (same serial number) to be loaded, got a different one")
		}

		if !key1.Equal(key2) {
			t.Error("expected the same private key to be loaded on the second call")
		}
	})
}

func TestCertConfigLeafCertificates(t *testing.T) {
	caCert, caKey, err := proxy.NewCA("Test CA", "Test", 365*24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create CA: %v", err)
	}

	certConfig, err := proxy.NewCertConfig(caCert, caKey)
	if err != nil {
		t.Fatalf("failed to create cert config: %v", err)
	}

	tlsConfig := certConfig.TLSConfig()

	t.Run("rejects a handshake without SNI", func(t *testing.T) {
		_, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: ""})
		if err == nil {
			t.Fatal("expected an error for a missing SNI server name")
		}
	})

	t.Run("issues a DNS-name leaf certificate signed by the CA", func(t *testing.T) {
		tlsCert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: testHostname})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse leaf certificate: %v", err)
		}

		if leaf.IsCA {
			t.Error("expected leaf certificate to not be a CA")
		}

		if got, want := leaf.Subject.CommonName, testHostname; got != want {
			t.Errorf("expected CommonName %q, got %q", want, got)
		}

		if got, want := leaf.DNSNames, testHostname; len(got) != 1 || got[0] != want {
			t.Errorf("expected DNSNames [%q], got %v", want, got)
		}

		if len(leaf.IPAddresses) != 0 {
			t.Errorf("expected no IPAddresses for a DNS hostname, got %v", leaf.IPAddresses)
		}

		if len(leaf.ExtKeyUsage) != 1 || leaf.ExtKeyUsage[0] != x509.ExtKeyUsageServerAuth {
			t.Errorf("expected ExtKeyUsage [ServerAuth], got %v", leaf.ExtKeyUsage)
		}

		if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 {
			t.Error("expected KeyUsageDigitalSignature to be set")
		}

		if err := leaf.CheckSignatureFrom(caCert); err != nil {
			t.Errorf("expected leaf certificate to be signed by the CA: %v", err)
		}

		validity := leaf.NotAfter.Sub(leaf.NotBefore)
		if validity < 30*24*time.Hour || validity > 31*time.Hour+30*24*time.Hour {
			t.Errorf("expected ~30 day validity window (with 1h NotBefore skew tolerance), got %v", validity)
		}
	})

	t.Run("issues an IP leaf certificate for an IP-literal hostname", func(t *testing.T) {
		tlsCert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "127.0.0.1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse leaf certificate: %v", err)
		}

		if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "127.0.0.1" {
			t.Errorf("expected IPAddresses [127.0.0.1], got %v", leaf.IPAddresses)
		}

		if len(leaf.DNSNames) != 0 {
			t.Errorf("expected no DNSNames for an IP hostname, got %v", leaf.DNSNames)
		}
	})

	t.Run("strips the port from a host:port SNI value", func(t *testing.T) {
		tlsCert, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "example.com:8443"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		leaf, err := x509.ParseCertificate(tlsCert.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse leaf certificate: %v", err)
		}

		if got, want := leaf.Subject.CommonName, testHostname; got != want {
			t.Errorf("expected port stripped from CommonName, got %q", got)
		}
	})

	// Documents actual (not assumed) behavior: cert.go has no cache, so a
	// repeat handshake to an already-issued host mints a brand-new
	// certificate rather than reusing one. See the MaxSerialNumber comment
	// in cert.go — a prior comment here incorrectly assumed hostname-keyed
	// caching existed. If caching is added later, this test should change
	// along with that comment, not be deleted silently.
	t.Run("is not cached: repeat handshakes for the same host mint distinct certificates", func(t *testing.T) {
		first, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "repeat.example.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		second, err := tlsConfig.GetCertificate(&tls.ClientHelloInfo{ServerName: "repeat.example.com"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		firstLeaf, err := x509.ParseCertificate(first.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse first leaf certificate: %v", err)
		}

		secondLeaf, err := x509.ParseCertificate(second.Certificate[0])
		if err != nil {
			t.Fatalf("failed to parse second leaf certificate: %v", err)
		}

		if firstLeaf.SerialNumber.Cmp(secondLeaf.SerialNumber) == 0 {
			t.Error("expected distinct serial numbers across repeat handshakes (no caching exists)")
		}
	})
}
