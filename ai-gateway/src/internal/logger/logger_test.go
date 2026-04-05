package logger

import (
	"testing"
)

func TestMaskSensitive_AnthropicKey(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string // just check it doesn't contain the full key
	}{
		{
			"anthropic api key",
			"Using key sk-ant-api03-AAAAAABBBBBBCCCCCCDDDDDD-EEEEEE",
			"sk-ant-api03-A***", // prefix preserved, rest masked
		},
		{
			"openai key",
			"Bearer sk-proj1234567890abcdef",
			"Bearer ***",
		},
		{
			"gemini key in URL",
			"key=AIzaSyABCDEFGHIJKLMNOP",
			"key=***",
		},
		{
			"bearer token",
			"Authorization: Bearer eyJhbGciOiJSUzI1NiJ9.long-token-here",
			"Bearer ***",
		},
		{
			"no sensitive data",
			"normal log message without secrets",
			"normal log message without secrets",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := maskSensitive(tt.input)
			if tt.name == "no sensitive data" {
				if result != tt.want {
					t.Errorf("got %q, want %q", result, tt.want)
				}
				return
			}
			// For sensitive data, ensure the original full key is NOT in the output
			if result == tt.input {
				t.Errorf("input was not masked: %q", result)
			}
			// Ensure "***" appears
			if !containsStr(result, "***") {
				t.Errorf("masked output should contain ***: %q", result)
			}
		})
	}
}

func TestMaskSensitive_MultipleKeys(t *testing.T) {
	input := "key1=sk-ant-api03-AAAAAA key2=Bearer secret-token key3=AIzaSyXXXXXXXXXXXXXXX"
	result := maskSensitive(input)
	// All three should be masked
	if containsStr(result, "AAAAAA") {
		t.Error("anthropic key not fully masked")
	}
	if containsStr(result, "secret-token") {
		t.Error("bearer token not masked")
	}
}

func TestSetLevel(t *testing.T) {
	tests := []struct {
		input string
		want  Level
	}{
		{"debug", LevelDebug},
		{"info", LevelInfo},
		{"warn", LevelWarn},
		{"error", LevelError},
		{"unknown", LevelInfo},
		{"", LevelInfo},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			SetLevel(tt.input)
			mu.RLock()
			got := level
			mu.RUnlock()
			if got != tt.want {
				t.Errorf("SetLevel(%q): got %d, want %d", tt.input, got, tt.want)
			}
		})
	}
	// Reset to default
	SetLevel("info")
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
