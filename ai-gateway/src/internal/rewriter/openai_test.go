package rewriter

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

func TestOpenAIRewriteUserField(t *testing.T) {
	cfg := testConfig()
	rw := &OpenAIRewriter{}

	body := map[string]any{
		"model":  "gpt-4",
		"user":   "original-user-fingerprint",
		"stream": true,
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/chat/completions", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	expected := cfg.Identity.DeviceID[:16]
	if rewritten["user"] != expected {
		t.Errorf("user field not rewritten: got %v, want %s", rewritten["user"], expected)
	}
}

func TestOpenAIRemoveDeviceInfo(t *testing.T) {
	cfg := testConfig()
	rw := &OpenAIRewriter{}

	body := map[string]any{
		"model":       "gpt-4",
		"fingerprint": "leaked-fp",
		"device_id":   "leaked-did",
		"client_info": map[string]any{"os": "linux"},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/chat/completions", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	for _, field := range []string{"fingerprint", "device_id", "client_info"} {
		if _, ok := rewritten[field]; ok {
			t.Errorf("field %s should be removed", field)
		}
	}
}

func TestOpenAIRewriteSystemMessage(t *testing.T) {
	cfg := testConfig()
	rw := &OpenAIRewriter{}

	body := map[string]any{
		"model": "gpt-4",
		"messages": []any{
			map[string]any{
				"role":    "system",
				"content": "Platform: linux\nShell: bash\nOS Version: Linux 5.15.0",
			},
			map[string]any{
				"role":    "user",
				"content": "Hello",
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/chat/completions", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	messages := rewritten["messages"].([]any)
	sysMsg := messages[0].(map[string]any)
	content := sysMsg["content"].(string)

	if !strings.Contains(content, "Platform: darwin") {
		t.Error("system message Platform not rewritten")
	}
	if !strings.Contains(content, "Shell: zsh") {
		t.Error("system message Shell not rewritten")
	}
}

func TestOpenAIRewriteHeaders(t *testing.T) {
	cfg := testConfig()
	provider := &config.ProviderConfig{
		APIKey: "sk-test-key",
	}
	rw := &OpenAIRewriter{}

	headers := http.Header{
		"User-Agent":       {"python-openai/1.5.0"},
		"Authorization":    {"Bearer sk-original"},
		"X-Stainless-Lang": {"python"},
		"X-Stainless-Os":   {"Linux"},
		"X-Request-Id":     {"req-123"},
		"Content-Type":     {"application/json"},
	}

	result := rw.RewriteHeaders(headers, cfg, provider)

	if result.Get("User-Agent") != "OpenAI-Client/1.0" {
		t.Errorf("User-Agent not normalized: %s", result.Get("User-Agent"))
	}
	if result.Get("Authorization") != "Bearer sk-test-key" {
		t.Errorf("API key not injected: %s", result.Get("Authorization"))
	}
	if result.Get("X-Stainless-Lang") != "" {
		t.Error("x-stainless-* headers should be stripped")
	}
	if result.Get("X-Request-Id") != "" {
		t.Error("x-request-id should be stripped")
	}
	if result.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should pass through")
	}
}

func TestOpenAINoModification(t *testing.T) {
	cfg := testConfig()
	rw := &OpenAIRewriter{}

	body := map[string]any{
		"model":  "gpt-4",
		"prompt": "Hello",
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/completions", cfg)

	// No user/fingerprint/device_id fields → body should be returned as-is
	if string(result) != string(bodyBytes) {
		t.Error("body with no identity fields should be returned unchanged")
	}
}
