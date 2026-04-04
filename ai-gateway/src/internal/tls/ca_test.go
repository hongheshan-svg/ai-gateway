package tlsca

import (
	"crypto/tls"
	"crypto/x509"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewManagerGeneratesCA(t *testing.T) {
	tmpDir := t.TempDir()
	caDir := filepath.Join(tmpDir, "ca")
	cacheDir := filepath.Join(tmpDir, "certs")

	m, err := NewManager(caDir, cacheDir)
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	// CA files should exist
	if _, err := os.Stat(filepath.Join(caDir, "ca.crt")); err != nil {
		t.Error("ca.crt not created")
	}
	if _, err := os.Stat(filepath.Join(caDir, "ca.key")); err != nil {
		t.Error("ca.key not created")
	}

	// Key file should have restricted permissions
	info, _ := os.Stat(filepath.Join(caDir, "ca.key"))
	if info.Mode().Perm() != 0600 {
		t.Errorf("ca.key permissions: got %o, want 0600", info.Mode().Perm())
	}

	// CA cert should be valid
	pem := m.CACertPEM()
	if len(pem) == 0 {
		t.Error("CACertPEM returned empty")
	}
	der := m.CACertDER()
	if len(der) == 0 {
		t.Error("CACertDER returned empty")
	}

	// Parse the cert
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("invalid DER cert: %v", err)
	}
	if cert.Subject.CommonName != "AI Gateway CA" {
		t.Errorf("unexpected CN: %s", cert.Subject.CommonName)
	}
	if !cert.IsCA {
		t.Error("certificate should be CA")
	}

	// Fingerprint format
	fp := m.CACertFingerprint()
	if len(fp) == 0 {
		t.Error("empty fingerprint")
	}
	parts := strings.Split(fp, ":")
	if len(parts) != 32 {
		t.Errorf("fingerprint should have 32 hex pairs, got %d", len(parts))
	}
}

func TestNewManagerLoadsExistingCA(t *testing.T) {
	tmpDir := t.TempDir()
	caDir := filepath.Join(tmpDir, "ca")
	cacheDir := filepath.Join(tmpDir, "certs")

	// First creation
	m1, err := NewManager(caDir, cacheDir)
	if err != nil {
		t.Fatalf("first NewManager failed: %v", err)
	}
	fp1 := m1.CACertFingerprint()

	// Second load — should reuse same CA
	m2, err := NewManager(caDir, cacheDir)
	if err != nil {
		t.Fatalf("second NewManager failed: %v", err)
	}
	fp2 := m2.CACertFingerprint()

	if fp1 != fp2 {
		t.Errorf("CA fingerprint changed after reload: %s vs %s", fp1, fp2)
	}
}

func TestGetCertificateForDomain(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewManager(filepath.Join(tmpDir, "ca"), filepath.Join(tmpDir, "certs"))
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	hello := &tls.ClientHelloInfo{
		ServerName: "api.anthropic.com",
	}
	cert, err := m.GetCertificate(hello)
	if err != nil {
		t.Fatalf("GetCertificate failed: %v", err)
	}
	if cert == nil {
		t.Fatal("got nil certificate")
	}

	// Parse and verify the domain cert
	parsed, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("invalid domain cert: %v", err)
	}
	if parsed.Subject.CommonName != "api.anthropic.com" {
		t.Errorf("domain cert CN: %s", parsed.Subject.CommonName)
	}
	if len(parsed.DNSNames) == 0 || parsed.DNSNames[0] != "api.anthropic.com" {
		t.Errorf("domain cert SAN: %v", parsed.DNSNames)
	}

	// Should include CA cert in chain
	if len(cert.Certificate) != 2 {
		t.Errorf("expected 2 certs in chain (domain + CA), got %d", len(cert.Certificate))
	}

	// Verify cert is signed by our CA
	roots := x509.NewCertPool()
	roots.AddCert(m.caCert)
	_, err = parsed.Verify(x509.VerifyOptions{
		Roots: roots,
	})
	if err != nil {
		t.Errorf("domain cert not verified by CA: %v", err)
	}
}

func TestGetCertificateCaching(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewManager(filepath.Join(tmpDir, "ca"), filepath.Join(tmpDir, "certs"))
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	hello := &tls.ClientHelloInfo{ServerName: "api.openai.com"}
	cert1, _ := m.GetCertificate(hello)
	cert2, _ := m.GetCertificate(hello)

	// Should return the same cached certificate
	if cert1 != cert2 {
		t.Error("second call should return cached certificate")
	}
}

func TestGetCertificateNoSNI(t *testing.T) {
	tmpDir := t.TempDir()
	m, err := NewManager(filepath.Join(tmpDir, "ca"), filepath.Join(tmpDir, "certs"))
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	hello := &tls.ClientHelloInfo{ServerName: ""}
	_, err = m.GetCertificate(hello)
	if err == nil {
		t.Error("expected error for empty SNI")
	}
}
