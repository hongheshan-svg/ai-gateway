package proxy

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
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
	ProviderErrors  sync.Map // provider name -> *atomic.Int64
}

var stats Stats

// GetStats returns current proxy statistics as a map.
func GetStats() map[string]any {
	result := map[string]any{
		"total_requests":  stats.TotalRequests.Load(),
		"active_requests": stats.ActiveRequests.Load(),
		"providers":       map[string]int64{},
		"errors":          map[string]int64{},
	}
	providers := result["providers"].(map[string]int64)
	stats.ProviderCounts.Range(func(key, value any) bool {
		providers[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	errs := result["errors"].(map[string]int64)
	stats.ProviderErrors.Range(func(key, value any) bool {
		errs[key.(string)] = value.(*atomic.Int64).Load()
		return true
	})
	return result
}

func incrProvider(name string) {
	v, _ := stats.ProviderCounts.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

func incrProviderError(name string) {
	v, _ := stats.ProviderErrors.LoadOrStore(name, &atomic.Int64{})
	v.(*atomic.Int64).Add(1)
}

// --- Circuit Breaker ---

const (
	cbThreshold = 5                // consecutive failures to trip
	cbCooldown  = 30 * time.Second // time before half-open
)

type circuitBreaker struct {
	mu          sync.Mutex
	failures    int
	lastFailure time.Time
	tripped     bool
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	if !cb.tripped {
		return true
	}
	if time.Since(cb.lastFailure) > cbCooldown {
		cb.tripped = false
		cb.failures = 0
		logger.Info("Circuit breaker half-open, allowing probe request")
		return true
	}
	return false
}

func (cb *circuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures = 0
	cb.tripped = false
}

func (cb *circuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.failures++
	cb.lastFailure = time.Now()
	if cb.failures >= cbThreshold {
		cb.tripped = true
		logger.Warn("Circuit breaker tripped after %d failures", cb.failures)
	}
}

// Server is the main HTTPS reverse proxy server.
type Server struct {
	cfg       *config.Config
	tlsMgr    *tlsca.Manager
	transport *http.Transport
	httpSrv   *http.Server
	listener  net.Listener
	breakers  sync.Map // provider name -> *circuitBreaker
}

func (s *Server) getBreaker(provider string) *circuitBreaker {
	v, _ := s.breakers.LoadOrStore(provider, &circuitBreaker{})
	return v.(*circuitBreaker)
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
			MaxIdleConns:        20,
			MaxIdleConnsPerHost: 5,
			IdleConnTimeout:     90 * time.Second,
			DisableCompression:  true, // we handle compression ourselves
			DialContext: (&net.Dialer{
				Timeout:   10 * time.Second,
				KeepAlive: 30 * time.Second,
			}).DialContext,
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

	s.httpSrv = &http.Server{
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 300 * time.Second, // long for SSE streams
		IdleTimeout:  120 * time.Second,
	}

	return s.httpSrv.Serve(ln)
}

// Shutdown gracefully stops the proxy server, draining active connections.
func (s *Server) Shutdown(ctx context.Context) error {
	s.transport.CloseIdleConnections()
	if s.httpSrv != nil {
		return s.httpSrv.Shutdown(ctx)
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if rv := recover(); rv != nil {
			logger.Error("Request panic recovered: %v", rv)
			http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		}
	}()

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

	// Check circuit breaker
	cb := s.getBreaker(providerName)
	if !cb.allow() {
		logger.Warn("Circuit breaker open for %s, rejecting request", providerName)
		incrProviderError(providerName)
		http.Error(w, `{"error":"service unavailable","detail":"upstream circuit breaker open"}`, http.StatusServiceUnavailable)
		if s.cfg.Logging.Audit {
			logger.Audit(clientIP, r.Method, r.URL.Path, 503)
		}
		return
	}

	// Get rewriter for this provider
	rw, hasRewriter := rewriter.Get(providerName)

	// Read request body with size limit
	var body []byte
	if r.Body != nil {
		limited := io.LimitReader(r.Body, s.cfg.Server.MaxBodySize+1)
		var err error
		body, err = io.ReadAll(limited)
		r.Body.Close()
		if err != nil {
			logger.Error("Failed to read request body: %v", err)
			http.Error(w, `{"error":"failed to read body"}`, http.StatusBadRequest)
			return
		}
		if int64(len(body)) > s.cfg.Server.MaxBodySize {
			logger.Warn("Request body too large: %d bytes from %s", len(body), clientIP)
			http.Error(w, `{"error":"request body too large"}`, http.StatusRequestEntityTooLarge)
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

	// For Gemini, always replace API key in query parameter
	if providerName == "gemini" && provider.APIKey != "" {
		q := targetURL.Query()
		q.Set("key", provider.APIKey)
		targetURL.RawQuery = q.Encode()
	} else if providerName == "gemini" {
		q := targetURL.Query()
		q.Del("key")
		targetURL.RawQuery = q.Encode()
	}

	// Build and rewrite headers for upstream request
	var upstreamHeaders http.Header
	if hasRewriter {
		upstreamHeaders = rw.RewriteHeaders(r.Header, s.cfg, provider)
	} else {
		upstreamHeaders = make(http.Header)
		for key, values := range r.Header {
			lower := strings.ToLower(key)
			if lower == "host" || lower == "connection" || lower == "transfer-encoding" {
				continue
			}
			for _, v := range values {
				upstreamHeaders.Add(key, v)
			}
		}
	}
	upstreamHeaders.Set("Host", upstreamURL.Host)
	upstreamHeaders.Set("Content-Length", fmt.Sprintf("%d", len(body)))

	// Forward request with retry
	resp, err := s.doWithRetry(r.Context(), providerName, targetURL.String(), r.Method, body, upstreamHeaders)
	if err != nil {
		cb.recordFailure()
		incrProviderError(providerName)
		logger.Error("Upstream error [%s]: %v", providerName, err)
		// Sanitize: don't expose internal error details to client
		http.Error(w, `{"error":"bad gateway"}`, http.StatusBadGateway)
		if s.cfg.Logging.Audit {
			logger.Audit(clientIP, r.Method, r.URL.Path, 502)
		}
		return
	}
	defer resp.Body.Close()
	cb.recordSuccess()

	// Handle gzipped upstream response — decompress for client
	respBody := resp.Body
	if strings.Contains(resp.Header.Get("Content-Encoding"), "gzip") {
		if gr, err := gzip.NewReader(resp.Body); err == nil {
			respBody = gr
			defer gr.Close()
			resp.Header.Del("Content-Encoding")
			resp.Header.Del("Content-Length")
		}
	}

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
			n, readErr := respBody.Read(buf)
			if n > 0 {
				if _, writeErr := w.Write(buf[:n]); writeErr != nil {
					logger.Debug("Client disconnected during SSE stream [%s]", providerName)
					break
				}
				flusher.Flush()
			}
			if readErr != nil {
				break
			}
			// Check if client context is cancelled
			if r.Context().Err() != nil {
				logger.Debug("Client context cancelled during SSE [%s]", providerName)
				break
			}
		}
	} else {
		io.Copy(w, respBody)
	}

	if s.cfg.Logging.Audit {
		logger.Audit(clientIP, r.Method, r.URL.Path, resp.StatusCode)
	}
}

// doWithRetry sends an upstream request with retry on transient errors.
func (s *Server) doWithRetry(ctx context.Context, provider, urlStr, method string, body []byte, headers http.Header) (*http.Response, error) {
	maxRetries := s.cfg.Server.RetryCount
	var lastErr error

	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			// Exponential backoff: 500ms, 1s
			delay := time.Duration(attempt) * 500 * time.Millisecond
			timer := time.NewTimer(delay)
			select {
			case <-timer.C:
			case <-ctx.Done():
				timer.Stop()
				return nil, ctx.Err()
			}
			logger.Warn("Upstream retry %d/%d [%s]", attempt, maxRetries, provider)
		}

		req, err := http.NewRequestWithContext(ctx, method, urlStr, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		req.Header = headers

		resp, err := s.transport.RoundTrip(req)
		if err == nil {
			return resp, nil
		}
		// RoundTrip can return both resp and err; drain resp body to prevent leak
		if resp != nil {
			resp.Body.Close()
		}
		lastErr = err

		if !isRetryable(err) {
			return nil, err
		}
	}
	return nil, lastErr
}

// isRetryable returns true for transient network errors worth retrying.
func isRetryable(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	errStr := err.Error()
	return strings.Contains(errStr, "connection reset") ||
		strings.Contains(errStr, "connection refused") ||
		strings.Contains(errStr, "EOF")
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
			"ca_info":        tlsMgr.CACertInfo(),
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

// maxDecompressedSize caps gzip decompression to prevent zip-bomb OOM.
// Keep conservative for memory-constrained routers.
const maxDecompressedSize = 10 * 1024 * 1024 // 10 MB

func decompressGzip(data []byte) ([]byte, error) {
	reader, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	limited := io.LimitReader(reader, maxDecompressedSize+1)
	result, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if int64(len(result)) > maxDecompressedSize {
		return nil, fmt.Errorf("decompressed body exceeds %d bytes", maxDecompressedSize)
	}
	return result, nil
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
