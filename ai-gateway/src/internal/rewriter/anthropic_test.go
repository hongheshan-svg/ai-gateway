package rewriter

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

func testConfig() *config.Config {
	return &config.Config{
		Identity: config.IdentityConfig{
			DeviceID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			Email:    "test@example.com",
		},
		Env: config.EnvConfig{
			Platform:              "darwin",
			PlatformRaw:           "darwin",
			Arch:                  "arm64",
			NodeVersion:           "v24.3.0",
			Terminal:              "iTerm2.app",
			PackageManagers:       "npm,pnpm",
			Runtimes:              "node",
			Version:               "2.1.81",
			VersionBase:           "2.1.81",
			BuildTime:             "2026-03-20T21:26:18Z",
			DeploymentEnvironment: "unknown-darwin",
			VCS:                   "git",
		},
		PromptEnv: config.PromptEnvConfig{
			Platform:   "darwin",
			Shell:      "zsh",
			OSVersion:  "Darwin 24.4.0",
			WorkingDir: "/Users/jack/projects",
		},
		Process: config.ProcessConfig{
			ConstrainedMemory: 34359738368,
			RSSRange:          [2]int64{300000000, 500000000},
			HeapTotalRange:    [2]int64{40000000, 80000000},
			HeapUsedRange:     [2]int64{100000000, 200000000},
		},
		Upstream: map[string]config.ProviderConfig{
			"anthropic": {
				Enabled:          true,
				Upstream:         "https://api.anthropic.com",
				Domains:          []string{"api.anthropic.com"},
				OAuthAccessToken: "test-oauth-token",
			},
		},
	}
}

// --- CCH Hash Tests ---

func TestComputeCCH(t *testing.T) {
	// The CCH function: salt "59cf53e54c78" + chars at positions [4,7,20] + version → SHA256 → first 3 hex chars
	msg := "Hello, this is a test message for CCH!"
	version := "2.1.81"
	hash := computeCCH(msg, version)

	if len(hash) != 3 {
		t.Errorf("expected CCH hash length 3, got %d: %s", len(hash), hash)
	}
	// Must be hex
	for _, c := range hash {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex char in CCH: %c", c)
		}
	}
	// Deterministic
	hash2 := computeCCH(msg, version)
	if hash != hash2 {
		t.Errorf("CCH not deterministic: %s vs %s", hash, hash2)
	}
	// Different message → different hash (usually)
	hash3 := computeCCH("Xifferent message for testing CCH!!", version)
	// They could collide in 3 hex chars but it's unlikely with different inputs
	_ = hash3
}

func TestComputeCCHShortMessage(t *testing.T) {
	// Message shorter than position 20 — should use '0' as fallback
	hash := computeCCH("short", "2.1.81")
	if len(hash) != 3 {
		t.Errorf("expected CCH hash length 3, got %d", len(hash))
	}
}

func TestFallbackHash(t *testing.T) {
	hash := fallbackHash()
	if len(hash) != 3 {
		t.Errorf("expected fallback hash length 3, got %d", len(hash))
	}
}

// --- Anthropic Body Rewriting ---

func TestAnthropicRewriteMessagesBody(t *testing.T) {
	cfg := testConfig()
	rw := &AnthropicRewriter{}

	// Build a messages body with metadata.user_id containing device_id
	userID := map[string]any{
		"device_id": "original-device-id",
		"email":     "original@email.com",
	}
	userIDJSON, _ := json.Marshal(userID)

	body := map[string]any{
		"model":      "claude-sonnet-4-20250514",
		"max_tokens": 1024,
		"metadata": map[string]any{
			"user_id": string(userIDJSON),
		},
		"messages": []any{
			map[string]any{
				"role":    "user",
				"content": "Hello world, this is a test message!",
			},
		},
		"system": []any{
			"You are a helpful assistant.",
		},
	}

	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/v1/messages", cfg)

	var rewritten map[string]any
	if err := json.Unmarshal(result, &rewritten); err != nil {
		t.Fatalf("failed to unmarshal rewritten body: %v", err)
	}

	// Check metadata.user_id was rewritten
	metadata := rewritten["metadata"].(map[string]any)
	userIDStr := metadata["user_id"].(string)
	var parsedUID map[string]any
	if err := json.Unmarshal([]byte(userIDStr), &parsedUID); err != nil {
		t.Fatalf("failed to parse rewritten user_id: %v", err)
	}
	if parsedUID["device_id"] != cfg.Identity.DeviceID {
		t.Errorf("device_id not rewritten: got %v", parsedUID["device_id"])
	}
	if parsedUID["email"] != cfg.Identity.Email {
		t.Errorf("email not rewritten: got %v", parsedUID["email"])
	}
}

func TestAnthropicRewriteEventBatch(t *testing.T) {
	cfg := testConfig()
	rw := &AnthropicRewriter{}

	body := map[string]any{
		"events": []any{
			map[string]any{
				"event_data": map[string]any{
					"device_id": "leaked-device-id",
					"email":     "leaked@email.com",
					"baseUrl":   "https://internal.example.com",
					"gateway":   "internal-gw",
				},
			},
		},
	}

	bodyBytes, _ := json.Marshal(body)
	result := rw.RewriteBody(bodyBytes, "/api/event_logging/batch", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	events := rewritten["events"].([]any)
	eventData := events[0].(map[string]any)["event_data"].(map[string]any)

	if eventData["device_id"] != cfg.Identity.DeviceID {
		t.Errorf("device_id not rewritten in event: got %v", eventData["device_id"])
	}
	if eventData["email"] != cfg.Identity.Email {
		t.Errorf("email not rewritten in event: got %v", eventData["email"])
	}
	if _, ok := eventData["baseUrl"]; ok {
		t.Error("baseUrl should be stripped from event data")
	}
	if _, ok := eventData["gateway"]; ok {
		t.Error("gateway should be stripped from event data")
	}
}

func TestAnthropicRewriteHeaders(t *testing.T) {
	cfg := testConfig()
	p := cfg.Upstream["anthropic"]
	provider := &p
	rw := &AnthropicRewriter{}

	headers := http.Header{
		"User-Agent":                 {"claude-code/1.0.0 (linux, cli)"},
		"X-Api-Key":                  {"original-key"},
		"X-Anthropic-Billing-Header": {"billing-data"},
		"Content-Type":               {"application/json"},
		"Connection":                 {"keep-alive"},
		"X-Forwarded-For":            {"192.168.1.100"},
		"X-Real-Ip":                  {"10.0.0.1"},
	}

	result := rw.RewriteHeaders(headers, cfg, provider)

	// User-Agent should be normalized
	ua := result.Get("User-Agent")
	if !strings.Contains(ua, "claude-code/") {
		t.Errorf("User-Agent not normalized: %s", ua)
	}
	if !strings.Contains(ua, cfg.Env.Version) {
		t.Errorf("User-Agent doesn't contain version %s: %s", cfg.Env.Version, ua)
	}

	// Billing header should be stripped
	if result.Get("X-Anthropic-Billing-Header") != "" {
		t.Error("billing header not stripped")
	}

	// Connection should be stripped
	if result.Get("Connection") != "" {
		t.Error("hop-by-hop Connection header not stripped")
	}

	// Content-Type should pass through
	if result.Get("Content-Type") != "application/json" {
		t.Error("Content-Type should pass through")
	}

	// API key should be injected from OAuth token
	if result.Get("X-Api-Key") != "test-oauth-token" {
		t.Errorf("OAuth token not injected: got %s", result.Get("X-Api-Key"))
	}

	// Forwarding headers should be stripped
	if result.Get("X-Forwarded-For") != "" {
		t.Error("x-forwarded-for header should be stripped")
	}
	if result.Get("X-Real-Ip") != "" {
		t.Error("x-real-ip header should be stripped")
	}
}

func TestAnthropicRewritePromptText(t *testing.T) {
	cfg := testConfig()

	text := `<env>
Platform: linux
Shell: bash
OS Version: Linux 5.15.0
Working directory: /home/alice/src
</env>

The user's home is at /home/alice/documents`

	result := rewritePromptText(text, cfg, "abc")

	if !strings.Contains(result, "Platform: darwin") {
		t.Error("Platform not rewritten")
	}
	if !strings.Contains(result, "Shell: zsh") {
		t.Error("Shell not rewritten")
	}
	if !strings.Contains(result, "OS Version: Darwin 24.4.0") {
		t.Error("OS Version not rewritten")
	}
	if strings.Contains(result, "/home/alice/") {
		t.Error("home path not normalized")
	}
}

func TestAnthropicRewriteGenericIdentity(t *testing.T) {
	cfg := testConfig()
	rw := &AnthropicRewriter{}

	body := map[string]any{
		"device_id": "leaked-id",
		"email":     "leaked@example.com",
		"other":     "data",
	}
	bodyBytes, _ := json.Marshal(body)

	result := rw.RewriteBody(bodyBytes, "/api/policy_limits", cfg)

	var rewritten map[string]any
	json.Unmarshal(result, &rewritten)

	if rewritten["device_id"] != cfg.Identity.DeviceID {
		t.Errorf("device_id not rewritten: %v", rewritten["device_id"])
	}
	if rewritten["email"] != cfg.Identity.Email {
		t.Errorf("email not rewritten: %v", rewritten["email"])
	}
	if rewritten["other"] != "data" {
		t.Error("unrelated fields should not be modified")
	}
}

func TestAnthropicNonJSON(t *testing.T) {
	cfg := testConfig()
	rw := &AnthropicRewriter{}
	input := []byte("not json at all")
	result := rw.RewriteBody(input, "/v1/messages", cfg)
	if string(result) != string(input) {
		t.Error("non-JSON body should be returned unchanged")
	}
}

func TestExtractHomePrefix(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"/Users/jack/projects", "/Users/jack/"},
		{"/home/user/work", "/home/user/"},
		{"/short", "/Users/user/"},
	}

	for _, tt := range tests {
		result := extractHomePrefix(tt.input)
		if result != tt.expected {
			t.Errorf("extractHomePrefix(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}
