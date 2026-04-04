package proxy

import (
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
	"github.com/zhengshan/openwrt-ai-gateway/internal/rewriter"
	tlsca "github.com/zhengshan/openwrt-ai-gateway/internal/tls"
)

// Stats tracks proxy statistics.
type Stats struct {
	TotalRequests   atomic.Int64
	ActiveRequests  atomic.Int64
	ProviderCounts  sync.Map // provider name -> *atomic.Int64
}

var stats Stats

// GetStats returns current proxy statistics as a map.
func GetStats() map[string]any {
	result := map[string]any{
		"total_requests":  stats.TotalRequests.Load(),
		"active_requests": stats.ActiveRequests.Load(),
		"providers":       map[string]int64{},
	}
	providers := result["providers"].(map[string]int64)
	stats.ProviderCounts.Range(func(key, value any) bool {
		providers[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	return result
}

func incrProvider(name string) {
	v, _ := stats.ProviderCounts.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// Server is the main HTTPS reverse proxy server.
type Server struct {
	cfg       *config.Config
	tlsMgr    *tlsca.Manager
	transport *http.Transport
	listener  net.Listener
}

// NewServer creates a new proxy server.
func NewServer(cfg *config.Config, tlsMgr *tlsca.Manager) *Server {
	return &Server{
		cfg:    cfg,
		tlsMgr: tlsMgr,
		transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
			},
			MaxIdleConns:        100,
			MaxIdleConnsPerHost: 10,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true, // we handle compression ourselves
		},
	}
}

// Start begins listening and serving HTTPS connections.
func (s *Server) Start() error {
	addr := fmt.Sprintf(":%d", s.cfg.Server.ListenPort)

	tlsConfig := &tls.Config{
		GetCertificate: s.tlsMgr.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}

	ln, err := tls.Listen("tcp", addr, tlsConfig)
	if err != nil {
		return fmt.Errorf("listen %s: %w", addr, err)
	}
	s.listener = ln

	// Log all intercepted domains
	domains := s.cfg.AllDomains()
	logger.Info("AI Gateway listening on https://0.0.0.0%s", addr)
	logger.Info("Intercepting domains: %s", strings.Join(domains, ", "))
	for name, p := range s.cfg.Upstream {
		if p.Enabled {
			logger.Info("Provider %s → %s", name, p.Upstream)
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleRequest)

	server := &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // long for SSE streams
		IdleTimeout:  120 * time.Second,
	}

	return server.Serve(ln)
}

// Close stops the proxy server.
func (s *Server) Close() error {
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	stats.TotalRequests.Add(1)
	stats.ActiveRequests.Add(1)
	defer stats.ActiveRequests.Add(-1)

	clientIP := r.RemoteAddr
	if idx := strings.LastIndex(clientIP, ":"); idx > 0 {
		clientIP = clientIP[:idx]
	}

	// Determine which provider this request is for based on TLS SNI (Host header)
	host := r.Host
	if h := r.TLS; h != nil && h.ServerName != "" {
		host = h.ServerName
	}
	// Strip port if present
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}

	providerName, provider, found := s.cfg.ProviderForDomain(host)
	if !found {
		logger.Warn("Unknown domain: %s from %s", host, clientIP)
		http.Error(w, `{"error":"unknown domain"}`, http.StatusBadGateway)
		return
	}

	logger.Info("← %s %s%s from %s [%s]", r.Method, host, r.URL.Path, clientIP, providerName)
	incrProvider(providerName)

	// Get rewriter for this provider
	rw, hasRewriter := rewriter.Get(providerName)

	// Read request body
	var body []byte
	if r.Body != nil {
		var err error
		body, err = io.ReadAll(r.Body)
		r.Body.Close()
		if err != nil {
			logger.Error("Failed to read request body: %v", err)
			http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
			return
		}
	}

	// Decompress body if gzipped
	wasGzipped := false
	if strings.Contains(r.Header.Get("Content-Encoding"), "gzip") && len(body) > 0 {
		if decompressed, err := decompressGzip(body); err == nil {
			body = decompressed
			wasGzipped = true
		}
	}

	// Rewrite body
	if hasRewriter && len(body) > 0 {
		body = rw.RewriteBody(body, r.URL.Path, s.cfg)
	}

	// Re-compress if was gzipped
	if wasGzipped {
		if compressed, err := compressGzip(body); err == nil {
			body = compressed
		}
	}

	// Build upstream URL
	upstreamURL, err := url.Parse(provider.Upstream)
	if err != nil {
		logger.Error("Invalid upstream URL: %s", provider.Upstream)
		http.Error(w, `{"error":"invalid upstream"}`, http.StatusBadGateway)
		return
	}

	targetURL := *r.URL
	targetURL.Scheme = upstreamURL.Scheme
	targetURL.Host = upstreamURL.Host

	// For Gemini, inject API key as query parameter
	if providerName == "gemini" && provider.APIKey != "" {
		q := targetURL.Query()
		if q.Get("key") == "" {
			q.Set("key", provider.APIKey)
			targetURL.RawQuery = q.Encode()
		}
	}

	// Build upstream request
	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), bytes.NewReader(body))
	if err != nil {
		logger.Error("Failed to create upstream request: %v", err)
		http.Error(w, `{"error":"upstream request failed"}`, http.StatusBadGateway)
		return
	}

	// Rewrite headers
	if hasRewriter {
		upstreamReq.Header = rw.RewriteHeaders(r.Header, s.cfg, provider)
	} else {
		// Copy headers without modification
		for key, values := range r.Header {
			lower := strings.ToLower(key)
			if lower == "host" || lower == "connection" || lower == "transfer-encoding" {
				continue
			}
			for _, v := range values {
				upstreamReq.Header.Add(key, v)
			}
		}
	}

	upstreamReq.Header.Set("Host", upstreamURL.Host)
	upstreamReq.Header.Set("Content-Length", fmt.Sprintf("%d", len(body)))

	// Forward request
	resp, err := s.transport.RoundTrip(upstreamReq)
	if err != nil {
		logger.Error("Upstream error [%s]: %v", providerName, err)
		http.Error(w, fmt.Sprintf(`{"error":"bad gateway","detail":"%s"}`, err.Error()), http.StatusBadGateway)
		if s.cfg.Logging.Audit {
			logger.Audit(clientIP, r.Method, r.URL.Path, 502)
		}
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for key, values := range resp.Header {
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}

	// Stream response (SSE passthrough)
	w.WriteHeader(resp.StatusCode)

	// Use flusher for SSE streaming support
	flusher, canFlush := w.(http.Flusher)

	isSSE := strings.Contains(resp.Header.Get("Content-Type"), "text/event-stream")

	if isSSE && canFlush {
		// SSE: stream chunks to client as they arrive
		buf := make([]byte, 4096)
		for {
			n, err := resp.Body.Read(buf)
			if n > 0 {
				w.Write(buf[:n])
				flusher.Flush()
			}
			if err != nil {
				break
			}
		}
	} else {
		io.Copy(w, resp.Body)
	}

	if s.cfg.Logging.Audit {
		logger.Audit(clientIP, r.Method, r.URL.Path, resp.StatusCode)
	}
}

// StartCADownloadServer starts a plain HTTP server for CA certificate download.
func StartCADownloadServer(cfg *config.Config, tlsMgr *tlsca.Manager) error {
	addr := fmt.Sprintf(":%d", cfg.Server.CADownloadPort)

	mux := http.NewServeMux()

	mux.HandleFunc("/ca.crt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", "attachment; filename=\"ai-gateway-ca.crt\"")
		w.Write(tlsMgr.CACertPEM())
	})

	mux.HandleFunc("/ca.der", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-x509-ca-cert")
		w.Header().Set("Content-Disposition", "attachment; filename=\"ai-gateway-ca.der\"")
		w.Write(tlsMgr.CACertDER())
	})

	mux.HandleFunc("/status", func(w http.ResponseWriter, r *http.Request) {
		status := map[string]any{
			"status":         "running",
			"ca_fingerprint": tlsMgr.CACertFingerprint(),
			"domains":        cfg.AllDomains(),
			"stats":          GetStats(),
			"providers":      map[string]any{},
		}
		for name, p := range cfg.Upstream {
			status["providers"].(map[string]any)[name] = map[string]any{
				"enabled":  p.Enabled,
				"upstream": p.Upstream,
				"domains":  p.Domains,
			}
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(status)
	})

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		fmt.Fprintf(w, `<!DOCTYPE html>
<html><head><title>AI Gateway - CA Certificate</title>
<style>body{font-family:system-ui;max-width:600px;margin:50px auto;padding:0 20px}
a{display:inline-block;margin:10px 0;padding:10px 20px;background:#0066cc;color:white;
text-decoration:none;border-radius:4px}a:hover{background:#0052a3}
code{background:#f0f0f0;padding:2px 6px;border-radius:3px}
.section{margin:20px 0;padding:15px;border:1px solid #ddd;border-radius:8px}</style>
</head><body>
<h1>🛡️ AI Gateway</h1>
<p>Download and install the CA certificate to enable transparent AI API proxying.</p>
<div class="section">
<h3>Download Certificate</h3>
<a href="/ca.crt">Download CA Certificate (PEM)</a>
<a href="/ca.der">Download CA Certificate (DER)</a>
</div>
<div class="section">
<h3>Installation Guide</h3>
<h4>macOS</h4>
<ol>
<li>Download <code>ca.crt</code></li>
<li>Double-click to open in Keychain Access</li>
<li>Find "AI Gateway CA" → Get Info → Trust → Always Trust</li>
</ol>
<h4>Windows</h4>
<ol>
<li>Download <code>ca.der</code></li>
<li>Double-click → Install Certificate → Local Machine → Trusted Root CAs</li>
</ol>
<h4>Linux</h4>
<pre>curl -o /usr/local/share/ca-certificates/ai-gateway.crt http://%s/ca.crt
update-ca-certificates</pre>
<h4>iOS</h4>
<ol>
<li>Open <code>http://%s/ca.crt</code> in Safari</li>
<li>Install Profile → Settings → General → About → Certificate Trust Settings → Enable</li>
</ol>
</div>
<div class="section">
<h3>Status</h3>
<p><a href="/status">View Gateway Status (JSON)</a></p>
</div>
</body></html>`, r.Host, r.Host)
	})

	logger.Info("CA download server listening on http://0.0.0.0%s", addr)
	return http.ListenAndServe(addr, mux)
}

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func compressGzip(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer := gzip.NewWriter(&buf)
	if _, err := writer.Write(data); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
