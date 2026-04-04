package rewriter

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
)

// OpenAIRewriter normalizes identity/telemetry in OpenAI API requests.
type OpenAIRewriter struct{}

func (r *OpenAIRewriter) Name() string { return "openai" }

func (r *OpenAIRewriter) RewriteBody(body []byte, path string, cfg *config.Config) []byte {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body
	}

	obj, ok := parsed.(map[string]any)
	if !ok {
		return body
	}

	modified := false

	// Normalize "user" field to a canonical value
	if _, ok := obj["user"]; ok {
		obj["user"] = cfg.Identity.DeviceID[:16]
		modified = true
		logger.Debug("OpenAI: Rewrote user field")
	}

	// Remove any metadata or telemetry fields that may leak device info
	for _, field := range []string{"fingerprint", "device_id", "client_info"} {
		if _, ok := obj[field]; ok {
			delete(obj, field)
			modified = true
			logger.Debug("OpenAI: Removed field %s", field)
		}
	}

	// For chat completions, sanitize system messages containing env info
	if strings.Contains(path, "/chat/completions") {
		if messages, ok := obj["messages"].([]any); ok {
			for _, msg := range messages {
				msgMap, ok := msg.(map[string]any)
				if !ok {
					continue
				}
				if role, _ := msgMap["role"].(string); role == "system" {
					if content, ok := msgMap["content"].(string); ok {
						rewritten := rewriteOpenAISystemContent(content, cfg)
						if rewritten != content {
							msgMap["content"] = rewritten
							modified = true
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

func (r *OpenAIRewriter) RewriteHeaders(header http.Header, cfg *config.Config, provider *config.ProviderConfig) http.Header {
	out := make(http.Header)
	for key, values := range header {
		lower := strings.ToLower(key)

		switch lower {
		case "host", "connection", "proxy-authorization", "proxy-connection",
			"transfer-encoding":
			continue
		case "authorization":
			// Will be replaced below
			continue
		}

		if lower == "user-agent" {
			// Normalize User-Agent to a generic value
			out.Set(key, "OpenAI-Client/1.0")
			continue
		}

		// Strip OpenAI-specific telemetry headers
		if strings.HasPrefix(lower, "x-stainless-") {
			continue
		}
		if lower == "x-request-id" {
			continue
		}

		for _, v := range values {
			out.Add(key, v)
		}
	}

	// Inject API key
	if provider.APIKey != "" {
		out.Set("Authorization", "Bearer "+provider.APIKey)
	}

	return out
}

// rewriteOpenAISystemContent normalizes environment info in system prompts.
func rewriteOpenAISystemContent(content string, cfg *config.Config) string {
	pe := cfg.PromptEnv
	result := content

	// Rewrite common env patterns
	result = rePlatform.ReplaceAllString(result, "Platform: "+pe.Platform)
	result = reShell.ReplaceAllString(result, "Shell: "+pe.Shell)
	result = reOSVersion.ReplaceAllString(result, "OS Version: "+pe.OSVersion)
	result = reWorkingDir.ReplaceAllString(result, "${1}"+pe.WorkingDir)

	homePrefix := extractHomePrefix(pe.WorkingDir)
	result = reHomePath.ReplaceAllString(result, homePrefix)

	return result
}
