package rewriter

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

func TestGeminiRemoveDeviceInfo(t *testing.T) {
	cfg := testConfig()
	rw := &GeminiRewriter{}

	body := map[string]any{
		"model":       "gemini-pro",
		"client_info": map[string]any{"os": "linux"},
		"device_id":   "leaked-did",
		"fingerprint": "leaked-fp",
		"metadata":    map[string]any{"key": "val"},
		"contents":    []any{},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/models/gemini-pro:generateContent", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	for _, field := range []string{"client_info", "device_id", "fingerprint", "metadata"} {
		if _, ok := rewritten[field]; ok {
			t.Errorf("field %s should be removed", field)
		}
	}
	// contents should still be present
	if _, ok := rewritten["contents"]; !ok {
		t.Error("contents should not be removed")
	}
}

func TestGeminiRewriteSystemInstruction(t *testing.T) {
	cfg := testConfig()
	rw := &GeminiRewriter{}

	body := map[string]any{
		"system_instruction": map[string]any{
			"parts": []any{
				map[string]any{
					"text": "Platform: linux\nShell: bash\nOS Version: Linux 5.15.0\nWorking directory: /home/alice/src",
				},
			},
		},
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{"text": "Hello"},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/models/gemini-pro:generateContent", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	si := rewritten["system_instruction"].(map[string]any)
	parts := si["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)

	if !strings.Contains(text, "Platform: darwin") {
		t.Error("system_instruction Platform not rewritten")
	}
	if !strings.Contains(text, "Shell: zsh") {
		t.Error("system_instruction Shell not rewritten")
	}
	if !strings.Contains(text, "OS Version: Darwin 24.4.0") {
		t.Error("system_instruction OS Version not rewritten")
	}
}

func TestGeminiRewriteContentsEnv(t *testing.T) {
	cfg := testConfig()
	rw := &GeminiRewriter{}

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "user",
				"parts": []any{
					map[string]any{
						"text": "<env>\nPlatform: linux\nShell: bash\n</env>",
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/models/gemini-pro:streamGenerateContent", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	contents := rewritten["contents"].([]any)
	parts := contents[0].(map[string]any)["parts"].([]any)
	text := parts[0].(map[string]any)["text"].(string)

	if !strings.Contains(text, "Platform: darwin") {
		t.Error("contents env Platform not rewritten")
	}
}

func TestGeminiRewriteHeaders(t *testing.T) {
	cfg := testConfig()
	provider := &config.ProviderConfig{
		APIKey: "gemini-key-123",
	}
	rw := &GeminiRewriter{}

	headers := http.Header{
		"User-Agent":         {"python-genai/0.5.0"},
		"X-Goog-Api-Client":  {"gl-python/3.11 genai/0.5.0"},
		"X-Goog-Api-Key":     {"leaked-key"},
		"Content-Type":       {"application/json"},
		"X-Forwarded-For":    {"192.168.1.100"},
	}

	result := rw.RewriteHeaders(headers, cfg, provider)

	if result.Get("User-Agent") != "google-api-client/1.0" {
		t.Errorf("User-Agent not normalized: %s", result.Get("User-Agent"))
	}
	if result.Get("X-Goog-Api-Client") != "genai-go/0.1.0" {
		t.Errorf("x-goog-api-client not normalized: %s", result.Get("X-Goog-Api-Client"))
	}
	if result.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should pass through")
	}
	if result.Get("X-Goog-Api-Key") != "" {
		t.Error("x-goog-api-key header should be stripped")
	}
	if result.Get("X-Forwarded-For") != "" {
		t.Error("x-forwarded-for header should be stripped")
	}
}

func TestGeminiModelResponseNotModified(t *testing.T) {
	cfg := testConfig()
	rw := &GeminiRewriter{}

	body := map[string]any{
		"contents": []any{
			map[string]any{
				"role": "model",
				"parts": []any{
					map[string]any{
						"text": "<env>\nPlatform: linux\nShell: bash\n</env>",
					},
				},
			},
		},
	}
	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/models/gemini-pro:generateContent", cfg)

	// Model responses should not be modified, so body stays the same
	if string(result) != string(bodyBytes) {
		t.Error("model response content should not be modified")
	}
}

// --- Registry Tests ---

func TestRegistry(t *testing.T) {
	for _, name := range []string{"anthropic", "openai", "gemini"} {
		rw, ok := Get(name)
		if !ok {
			t.Errorf("registry missing provider: %s", name)
			continue
		}
		if rw.Name() != name {
			t.Errorf("provider %s Name() = %s", name, rw.Name())
		}
	}

	_, ok := Get("nonexistent")
	if ok {
		t.Error("nonexistent provider should not be found")
	}
}
