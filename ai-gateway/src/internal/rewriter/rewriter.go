package rewriter

import (
	"net/http"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

// Rewriter modifies HTTP requests to normalize identity/telemetry before upstream forwarding.
type Rewriter interface {
	// Name returns the provider name (e.g., "anthropic", "openai", "gemini").
	Name() string

	// RewriteBody modifies the request body JSON to normalize identity fields.
	// Returns the rewritten body bytes.
	RewriteBody(body []byte, path string, cfg *config.Config) []byte

	// RewriteHeaders modifies request headers to normalize identity.
	// It receives the original headers and provider config, returns cleaned headers.
	RewriteHeaders(header http.Header, cfg *config.Config, provider *config.ProviderConfig) http.Header
}

// Registry maps provider names to their rewriter implementations.
var Registry = map[string]Rewriter{
	"anthropic": &AnthropicRewriter{},
	"openai":    &OpenAIRewriter{},
	"gemini":    &GeminiRewriter{},
}

// Get returns the rewriter for the given provider name.
func Get(provider string) (Rewriter, bool) {
	r, ok := Registry[provider]
	return r, ok
}
