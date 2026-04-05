package rewriter

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
)

// GeminiRewriter normalizes identity/telemetry in Google Gemini API requests.
type GeminiRewriter struct{}

func (r *GeminiRewriter) Name() string { return "gemini" }

func (r *GeminiRewriter) RewriteBody(body []byte, path string, cfg *config.Config) []byte {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}

	obj, ok := parsed.(map[string]any)
	if !ok {
		return body
	}

	modified := false

	// Remove client metadata / telemetry fields
	for _, field := range []string{"client_info", "device_id", "fingerprint", "metadata"} {
		if _, ok := obj[field]; ok {
			delete(obj, field)
			modified = true
			logger.Debug("Gemini: Removed field %s", field)
		}
	}

	// Rewrite system instruction env blocks
	// Gemini API paths use colon: /models/{model}:generateContent
	if strings.Contains(path, "generateContent") || strings.Contains(path, "streamGenerateContent") ||
		strings.Contains(path, "countTokens") || strings.Contains(path, "batchGenerateContent") {
		if si, ok := obj["system_instruction"].(map[string]any); ok {
			if parts, ok := si["parts"].([]any); ok {
				for _, part := range parts {
					partMap, ok := part.(map[string]any)
					if !ok {
						continue
					}
					if text, ok := partMap["text"].(string); ok {
						rewritten := rewriteGeminiSystemText(text, cfg)
						if rewritten != text {
							partMap["text"] = rewritten
							modified = true
						}
					}
				}
			}
		}

		// Also check contents array for system-like messages
		if contents, ok := obj["contents"].([]any); ok {
			for _, content := range contents {
				contentMap, ok := content.(map[string]any)
				if !ok {
					continue
				}
				if role, _ := contentMap["role"].(string); role == "model" {
					continue // don't modify model responses
				}
				if parts, ok := contentMap["parts"].([]any); ok {
					for _, part := range parts {
						partMap, ok := part.(map[string]any)
						if !ok {
							continue
						}
						if text, ok := partMap["text"].(string); ok {
							// Only rewrite if it contains env-like patterns
							if strings.Contains(text, "<env>") || strings.Contains(text, "Platform:") {
								rewritten := rewriteGeminiSystemText(text, cfg)
								if rewritten != text {
									partMap["text"] = rewritten
									modified = true
								}
							}
						}
					}
				}
			}
		}
	}

	if !modified {
		return body
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		return body
	}
	return out
}

func (r *GeminiRewriter) RewriteHeaders(header http.Header, cfg *config.Config, provider *config.ProviderConfig) http.Header {
	out := make(http.Header)
	for key, values := range header {
		lower := strings.ToLower(key)

		switch lower {
		case "host", "connection", "proxy-authorization", "proxy-connection",
			"transfer-encoding":
			continue
		}

		if lower == "user-agent" {
			out.Set(key, "google-api-client/1.0")
			continue
		}

		// Strip Google-specific telemetry headers
		if strings.HasPrefix(lower, "x-goog-") {
			if lower == "x-goog-api-client" {
				out.Set(key, "genai-go/0.1.0")
			}
			// Drop all other x-goog-* headers (x-goog-api-key handled via query param)
			continue
		}

		// Strip forwarding headers that leak client IP
		if lower == "x-forwarded-for" || lower == "x-real-ip" || lower == "x-forwarded-host" {
			continue
		}

		for _, v := range values {
			out.Add(key, v)
		}
	}

	// API key is passed as query parameter for Gemini, handled in proxy

	return out
}

func rewriteGeminiSystemText(content string, cfg *config.Config) string {
	pe := cfg.PromptEnv
	result := content

	result = rePlatform.ReplaceAllString(result, "Platform: "+pe.Platform)
	result = reShell.ReplaceAllString(result, "Shell: "+pe.Shell)
	result = reOSVersion.ReplaceAllString(result, "OS Version: "+pe.OSVersion)
	result = reWorkingDir.ReplaceAllString(result, "${1}"+pe.WorkingDir)

	homePrefix := extractHomePrefix(pe.WorkingDir)
	result = reHomePath.ReplaceAllString(result, homePrefix)

	return result
}
