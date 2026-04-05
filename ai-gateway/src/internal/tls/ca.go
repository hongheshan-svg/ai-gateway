package tlsca

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
)

const (
	certCacheTTL     = 24 * time.Hour
	certCacheMaxSize = 200 // conservative for embedded devices
)

type cachedCert struct {
	cert      *tls.Certificate
	createdAt time.Time
}

// Manager handles CA certificate generation, domain certificate signing, and caching.
type Manager struct {
	caDir        string
	cacheDir     string
	caCert       *x509.Certificate
	caKey        *ecdsa.PrivateKey
	caTLSCert    tls.Certificate
	certCache    map[string]*cachedCert
	mu           sync.RWMutex
}

// NewManager creates a TLS CA manager. It loads or generates a root CA.
func NewManager(caDir, cacheDir string) (*Manager, error) {
	m := &Manager{
		caDir:     caDir,
		cacheDir:  cacheDir,
		certCache: make(map[string]*cachedCert),
	}
	if err := os.MkdirAll(caDir, 0700); err != nil {
		return nil, fmt.Errorf("create CA dir: %w", err)
	}
	if err := os.MkdirAll(cacheDir, 0700); err != nil {
		return nil, fmt.Errorf("create cert cache dir: %w", err)
	}

	certPath := filepath.Join(caDir, "ca.crt")
	keyPath := filepath.Join(caDir, "ca.key")

	if fileExists(certPath) && fileExists(keyPath) {
		if err := m.loadCA(certPath, keyPath); err != nil {
			logger.Warn("Failed to load existing CA, regenerating: %v", err)
			return m, m.generateCA(certPath, keyPath)
		}
		logger.Info("Loaded existing CA certificate from %s", certPath)
		return m, nil
	}

	logger.Info("Generating new CA certificate...")
	return m, m.generateCA(certPath, keyPath)
}

func (m *Manager) generateCA(certPath, keyPath string) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("generate CA key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   "AI Gateway CA",
			Organization: []string{"AI Gateway"},
		},
		NotBefore:             time.Now().Add(-1 * time.Hour),
		NotAfter:              time.Now().Add(10 * 365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
		MaxPathLen:            0,
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return fmt.Errorf("create CA cert: %w", err)
	}

	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return fmt.Errorf("parse CA cert: %w", err)
	}

	// Write cert PEM
	certFile, err := os.OpenFile(certPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("write CA cert: %w", err)
	}
	defer certFile.Close()
	if err := pem.Encode(certFile, &pem.Block{Type: "CERTIFICATE", Bytes: certDER}); err != nil {
		return err
	}

	// Write key PEM (restricted permissions)
	keyFile, err := os.OpenFile(keyPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return fmt.Errorf("write CA key: %w", err)
	}
	defer keyFile.Close()
	keyBytes, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal CA key: %w", err)
	}
	if err := pem.Encode(keyFile, &pem.Block{Type: "EC PRIVATE KEY", Bytes: keyBytes}); err != nil {
		return err
	}

	m.caCert = cert
	m.caKey = key
	m.caTLSCert = tls.Certificate{
		Certificate: [][]byte{certDER},
		PrivateKey:  key,
		Leaf:        cert,
	}

	logger.Info("Generated CA certificate: %s", certPath)
	return nil
}

func (m *Manager) loadCA(certPath, keyPath string) error {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return err
	}

	block, _ := pem.Decode(certPEM)
	if block == nil {
		return fmt.Errorf("no PEM block in CA cert")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		return err
	}

	keyBlock, _ := pem.Decode(keyPEM)
	if keyBlock == nil {
		return fmt.Errorf("no PEM block in CA key")
	}
	key, err := x509.ParseECPrivateKey(keyBlock.Bytes)
	if err != nil {
		return err
	}

	m.caCert = cert
	m.caKey = key
	m.caTLSCert = tls.Certificate{
		Certificate: [][]byte{cert.Raw},
		PrivateKey:  key,
		Leaf:        cert,
	}
	return nil
}

// GetCertificate returns a TLS certificate for the given domain, generating on demand.
func (m *Manager) GetCertificate(hello *tls.ClientHelloInfo) (*tls.Certificate, error) {
	domain := hello.ServerName
	if domain == "" {
		return nil, fmt.Errorf("no SNI provided")
	}

	m.mu.RLock()
	if cached, ok := m.certCache[domain]; ok && time.Since(cached.createdAt) < certCacheTTL {
		m.mu.RUnlock()
		return cached.cert, nil
	}
	m.mu.RUnlock()

	m.mu.Lock()
	defer m.mu.Unlock()

	// Double-check after acquiring write lock
	if cached, ok := m.certCache[domain]; ok && time.Since(cached.createdAt) < certCacheTTL {
		return cached.cert, nil
	}

	// Evict expired entries and enforce max size
	if len(m.certCache) >= certCacheMaxSize {
		m.evictExpired()
	}
	// If still at max after eviction, remove oldest
	if len(m.certCache) >= certCacheMaxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range m.certCache {
			if oldestKey == "" || v.createdAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.createdAt
			}
		}
		if oldestKey != "" {
			delete(m.certCache, oldestKey)
		}
	}

	cert, err := m.signDomainCert(domain)
	if err != nil {
		return nil, err
	}
	m.certCache[domain] = &cachedCert{cert: cert, createdAt: time.Now()}
	logger.Debug("Generated certificate for domain: %s", domain)
	return cert, nil
}

// evictExpired removes expired entries from the cert cache (must be called with write lock held).
func (m *Manager) evictExpired() {
	now := time.Now()
	for k, v := range m.certCache {
		if now.Sub(v.createdAt) >= certCacheTTL {
			delete(m.certCache, k)
		}
	}
}

func (m *Manager) signDomainCert(domain string) (*tls.Certificate, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate domain key: %w", err)
	}

	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName: domain,
		},
		NotBefore: time.Now().Add(-1 * time.Hour),
		NotAfter:  time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:  x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{
			x509.ExtKeyUsageServerAuth,
		},
		DNSNames: []string{domain},
	}

	// Also add IP SANs if domain looks like an IP
	if ip := net.ParseIP(domain); ip != nil {
		template.IPAddresses = []net.IP{ip}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, m.caCert, &key.PublicKey, m.caKey)
	if err != nil {
		return nil, fmt.Errorf("sign domain cert: %w", err)
	}

	tlsCert := &tls.Certificate{
		Certificate: [][]byte{certDER, m.caCert.Raw},
		PrivateKey:  key,
	}
	tlsCert.Leaf, _ = x509.ParseCertificate(certDER)
	return tlsCert, nil
}

// CACertPEM returns the CA certificate in PEM format for client download.
func (m *Manager) CACertPEM() []byte {
	return pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: m.caCert.Raw,
	})
}

// CACertDER returns the CA certificate in DER format.
func (m *Manager) CACertDER() []byte {
	return m.caCert.Raw
}

// CACertInfo returns key fields about the CA certificate for status display.
func (m *Manager) CACertInfo() map[string]any {
	return map[string]any{
		"subject":    m.caCert.Subject.CommonName,
		"issuer":     m.caCert.Issuer.CommonName,
		"not_before": m.caCert.NotBefore.Format(time.RFC3339),
		"not_after":  m.caCert.NotAfter.Format(time.RFC3339),
	}
}

// CertCacheSize returns the number of cached domain certificates.
func (m *Manager) CertCacheSize() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.certCache)
}

// CACertFingerprint returns SHA-256 fingerprint of the CA cert.
func (m *Manager) CACertFingerprint() string {
	sum := sha256.Sum256(m.caCert.Raw)
	parts := make([]string, len(sum))
	for i, b := range sum {
		parts[i] = fmt.Sprintf("%02X", b)
	}
	return joinFingerprint(parts)
}

func joinFingerprint(parts []string) string {
	result := ""
	for i, p := range parts {
		if i > 0 {
			result += ":"
		}
		result += p
	}
	return result
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
