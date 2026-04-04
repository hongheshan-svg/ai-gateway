package rewriter

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
	"github.com/zhengshan/openwrt-ai-gateway/internal/identity"
	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
)

const cchSalt = "59cf53e54c78"

var cchPositions = []int{4, 7, 20}

// AnthropicRewriter implements full identity normalization for Anthropic API requests.
// Port of cc-gateway's rewriter.ts logic.
type AnthropicRewriter struct{}

func (r *AnthropicRewriter) Name() string { return "anthropic" }

func (r *AnthropicRewriter) RewriteBody(body []byte, path string, cfg *config.Config) []byte {
	var parsed any
	if err := json.Unmarshal(body, &parsed); err != nil {
		return body // not JSON, pass through
	}

	obj, ok := parsed.(map[string]any)
	if !ok {
		return body
	}

	if strings.HasPrefix(path, "/v1/messages") {
		rewriteMessagesBody(obj, cfg)
	} else if strings.Contains(path, "/event_logging/batch") {
		rewriteEventBatch(obj, cfg)
	} else if strings.Contains(path, "/policy_limits") || strings.Contains(path, "/settings") {
		rewriteGenericIdentity(obj, cfg)
	}

	out, err := json.Marshal(parsed)
	if err != nil {
		logger.Error("Failed to marshal rewritten body: %v", err)
		return body
	}
	return out
}

func (r *AnthropicRewriter) RewriteHeaders(header http.Header, cfg *config.Config, provider *config.ProviderConfig) http.Header {
	out := make(http.Header)
	for key, values := range header {
		lower := strings.ToLower(key)

		// Skip hop-by-hop and auth headers (gateway injects real token)
		switch lower {
		case "host", "connection", "proxy-authorization", "proxy-connection",
			"transfer-encoding", "authorization":
			continue
		}

		if lower == "x-api-key" {
			// Will be replaced with real OAuth token below
			continue
		}

		if lower == "user-agent" {
			out.Set(key, fmt.Sprintf("claude-code/%s (external, cli)", cfg.Env.Version))
			continue
		}

		if lower == "x-anthropic-billing-header" {
			// Strip billing header entirely — consistent with
			// CLAUDE_CODE_ATTRIBUTION_HEADER=false
			logger.Debug("Stripped x-anthropic-billing-header")
			continue
		}

		for _, v := range values {
			out.Add(key, v)
		}
	}

	// Inject real API key / OAuth token
	if provider.OAuthAccessToken != "" {
		out.Set("X-Api-Key", provider.OAuthAccessToken)
	} else if provider.APIKey != "" {
		out.Set("X-Api-Key", provider.APIKey)
	}

	return out
}

// --- Messages body rewriting ---

func rewriteMessagesBody(body map[string]any, cfg *config.Config) {
	// 1. Rewrite metadata.user_id
	if metadata, ok := body["metadata"].(map[string]any); ok {
		if userIDStr, ok := metadata["user_id"].(string); ok {
			var userID map[string]any
			if err := json.Unmarshal([]byte(userIDStr), &userID); err == nil {
				userID["device_id"] = cfg.Identity.DeviceID
				if cfg.Identity.Email != "" {
					userID["email"] = cfg.Identity.Email
				}
				if rewritten, err := json.Marshal(userID); err == nil {
					metadata["user_id"] = string(rewritten)
					logger.Debug("Rewrote metadata.user_id device_id")
				}
			} else {
				logger.Warn("Failed to parse metadata.user_id")
			}
		}
	}

	// 2. Rewrite <system-reminder> blocks in messages
	if messages, ok := body["messages"].([]any); ok {
		for _, msg := range messages {
			msgMap, ok := msg.(map[string]any)
			if !ok {
				continue
			}
			if content, ok := msgMap["content"].(string); ok {
				msgMap["content"] = rewriteSystemReminders(content, cfg)
			} else if contentArr, ok := msgMap["content"].([]any); ok {
				for _, block := range contentArr {
					blockMap, ok := block.(map[string]any)
					if !ok {
						continue
					}
					if text, ok := blockMap["text"].(string); ok {
						blockMap["text"] = rewriteSystemReminders(text, cfg)
					}
				}
			}
		}
	}

	// 3. Extract first user message for CCH hash
	firstUserText := extractFirstUserMessage(body)

	// 4. Compute CCH hash
	version := cfg.Env.Version
	var hash string
	if firstUserText != "" {
		hash = computeCCH(firstUserText, version)
	} else {
		hash = fallbackHash()
	}
	logger.Debug("Computed CCH: %s (from %d char message)", hash, len(firstUserText))

	// 5. Strip billing header block from system prompt and rewrite remaining blocks
	rewriteSystemPrompt(body, cfg, hash)
}

var (
	reSystemReminder = regexp.MustCompile(`(?s)(<system-reminder>)(.*?)(</system-reminder>)`)
)

func rewriteSystemReminders(text string, cfg *config.Config) string {
	return reSystemReminder.ReplaceAllStringFunc(text, func(match string) string {
		parts := reSystemReminder.FindStringSubmatch(match)
		if len(parts) < 4 {
			return match
		}
		return parts[1] + rewritePromptText(parts[2], cfg, "") + parts[3]
	})
}

func rewriteSystemPrompt(body map[string]any, cfg *config.Config, hash string) {
	switch system := body["system"].(type) {
	case []any:
		// Filter out billing header blocks, rewrite remaining
		var filtered []any
		for _, item := range system {
			switch v := item.(type) {
			case string:
				if reBillingHeader.MatchString(v) {
					logger.Debug("Stripped billing header block from system prompt")
					continue
				}
				filtered = append(filtered, rewritePromptText(v, cfg, hash))
			case map[string]any:
				if text, ok := v["text"].(string); ok {
					if reBillingHeader.MatchString(text) {
						logger.Debug("Stripped billing header block from system prompt")
						continue
					}
					v["text"] = rewritePromptText(text, cfg, hash)
				}
				filtered = append(filtered, v)
			default:
				filtered = append(filtered, item)
			}
		}
		body["system"] = filtered

	case string:
		// Strip inline billing header
		cleaned := reBillingInline.ReplaceAllString(system, "")
		body["system"] = rewritePromptText(cleaned, cfg, hash)
	}
}

var (
	reBillingHeader = regexp.MustCompile(`(?m)^\s*x-anthropic-billing-header:`)
	reBillingInline = regexp.MustCompile(`x-anthropic-billing-header:[^\n]+\n?`)
	reCCVersion     = regexp.MustCompile(`cc_version=[\d.]+\.[a-f0-9]{3}`)
	rePlatform      = regexp.MustCompile(`Platform:\s*\S+`)
	reShell         = regexp.MustCompile(`Shell:\s*\S+`)
	reOSVersion     = regexp.MustCompile(`OS Version:\s*[^\n<]+`)
	reWorkingDir    = regexp.MustCompile(`((?:Primary )?[Ww]orking directory:\s*)/\S+`)
	reHomePath      = regexp.MustCompile(`/(?:Users|home)/[^/\s]+/`)
)

func rewritePromptText(text string, cfg *config.Config, hash string) string {
	pe := cfg.PromptEnv
	result := text

	// 1. Billing header fingerprint hash
	if hash != "" {
		result = reCCVersion.ReplaceAllString(result, fmt.Sprintf("cc_version=%s.%s", cfg.Env.Version, hash))
	}

	// 2. <env> block fields
	result = rePlatform.ReplaceAllString(result, "Platform: "+pe.Platform)
	result = reShell.ReplaceAllString(result, "Shell: "+pe.Shell)
	result = reOSVersion.ReplaceAllString(result, "OS Version: "+pe.OSVersion)

	// 3. Working directory
	result = reWorkingDir.ReplaceAllString(result, "${1}"+pe.WorkingDir)

	// 4. Home directory paths
	homePrefix := extractHomePrefix(pe.WorkingDir)
	result = reHomePath.ReplaceAllString(result, homePrefix)

	return result
}

func extractHomePrefix(workingDir string) string {
	// Extract /Users/xxx/ or /home/xxx/ from working dir
	parts := strings.SplitN(workingDir, "/", 4)
	if len(parts) >= 3 {
		return "/" + parts[1] + "/" + parts[2] + "/"
	}
	return "/Users/user/"
}

// --- Event batch rewriting ---

func rewriteEventBatch(body map[string]any, cfg *config.Config) {
	events, ok := body["events"].([]any)
	if !ok {
		return
	}
	for _, event := range events {
		eventMap, ok := event.(map[string]any)
		if !ok {
			continue
		}
		data, ok := eventMap["event_data"].(map[string]any)
		if !ok {
			continue
		}

		// Identity fields
		if _, ok := data["device_id"]; ok {
			data["device_id"] = cfg.Identity.DeviceID
		}
		if _, ok := data["email"]; ok {
			data["email"] = cfg.Identity.Email
		}

		// Environment fingerprint — replace entirely
		if _, ok := data["env"]; ok {
			data["env"] = identity.BuildCanonicalEnv(cfg)
		}

		// Process metrics
		if proc, ok := data["process"].(map[string]any); ok {
			data["process"] = identity.BuildCanonicalProcess(proc, cfg)
		} else if procStr, ok := data["process"].(string); ok {
			// Base64-encoded process data
			data["process"] = rewriteBase64Process(procStr, cfg)
		}

		// Strip leak fields
		delete(data, "baseUrl")
		delete(data, "base_url")
		delete(data, "gateway")

		// Rewrite additional_metadata if present
		if meta, ok := data["additional_metadata"].(string); ok {
			data["additional_metadata"] = rewriteAdditionalMetadata(meta)
		}

		eventName := ""
		if name, ok := data["event_name"].(string); ok {
			eventName = name
		}
		logger.Debug("Rewrote event: %s", eventName)
	}
}

func rewriteGenericIdentity(body map[string]any, cfg *config.Config) {
	if _, ok := body["device_id"]; ok {
		body["device_id"] = cfg.Identity.DeviceID
	}
	if _, ok := body["email"]; ok {
		body["email"] = cfg.Identity.Email
	}
}

// --- Helpers ---

func extractFirstUserMessage(body map[string]any) string {
	messages, ok := body["messages"].([]any)
	if !ok {
		return ""
	}
	for _, msg := range messages {
		msgMap, ok := msg.(map[string]any)
		if !ok {
			continue
		}
		if role, _ := msgMap["role"].(string); role != "user" {
			continue
		}
		if content, ok := msgMap["content"].(string); ok {
			return content
		}
		if contentArr, ok := msgMap["content"].([]any); ok {
			for _, block := range contentArr {
				blockMap, ok := block.(map[string]any)
				if !ok {
					continue
				}
				if blockMap["type"] == "text" {
					if text, ok := blockMap["text"].(string); ok {
						return text
					}
				}
			}
		}
		return ""
	}
	return ""
}

func computeCCH(firstUserMessage, version string) string {
	chars := make([]byte, len(cchPositions))
	for i, pos := range cchPositions {
		if pos < len(firstUserMessage) {
			chars[i] = firstUserMessage[pos]
		} else {
			chars[i] = '0'
		}
	}
	input := cchSalt + string(chars) + version
	sum := sha256.Sum256([]byte(input))
	return hex.EncodeToString(sum[:])[:3]
}

func fallbackHash() string {
	b := make([]byte, 2)
	rand.Read(b)
	return hex.EncodeToString(b)[:3]
}

func rewriteBase64Process(encoded string, cfg *config.Config) string {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded
	}
	var proc map[string]any
	if err := json.Unmarshal(decoded, &proc); err != nil {
		return encoded
	}
	rewritten := identity.BuildCanonicalProcess(proc, cfg)
	out, err := json.Marshal(rewritten)
	if err != nil {
		return encoded
	}
	return base64.StdEncoding.EncodeToString(out)
}

func rewriteAdditionalMetadata(encoded string) string {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return encoded
	}
	var meta map[string]any
	if err := json.Unmarshal(decoded, &meta); err != nil {
		return encoded
	}
	delete(meta, "baseUrl")
	delete(meta, "base_url")
	delete(meta, "gateway")
	out, err := json.Marshal(meta)
	if err != nil {
		return encoded
	}
	return base64.StdEncoding.EncodeToString(out)
}
