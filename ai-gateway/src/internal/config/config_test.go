package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidate_ValidConfig(t *testing.T) {
	cfg := validConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("valid config should pass: %v", err)
	}
}

func TestValidate_InvalidPort(t *testing.T) {
	tests := []struct {
		name       string
		listenPort int
		caPort     int
		wantErr    string
	}{
		{"listen_port zero", 0, 8080, "invalid listen_port"},
		{"listen_port too high", 70000, 8080, "invalid listen_port"},
		{"ca_port zero", 443, 0, "invalid ca_download_port"},
		{"same ports", 443, 443, "must differ"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := validConfig()
			cfg.Server.ListenPort = tt.listenPort
			cfg.Server.CADownloadPort = tt.caPort
			err := cfg.Validate()
			if err == nil {
				t.Fatal("expected error")
			}
			if !containsStr(err.Error(), tt.wantErr) {
				t.Errorf("error %q should contain %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestValidate_ProviderMissingUpstream(t *testing.T) {
	cfg := validConfig()
	cfg.Upstream["test"] = ProviderConfig{Enabled: true, Domains: []string{"example.com"}, APIKey: "sk-123"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing upstream")
	}
}

func TestValidate_ProviderMissingDomains(t *testing.T) {
	cfg := validConfig()
	cfg.Upstream["test"] = ProviderConfig{Enabled: true, Upstream: "https://api.example.com", APIKey: "sk-123"}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing domains")
	}
}

func TestValidate_ProviderMissingKey(t *testing.T) {
	cfg := validConfig()
	cfg.Upstream["test"] = ProviderConfig{
		Enabled:  true,
		Upstream: "https://api.example.com",
		Domains:  []string{"api.example.com"},
	}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for missing key")
	}
}

func TestValidate_DisabledProviderSkipped(t *testing.T) {
	cfg := validConfig()
	cfg.Upstream["test"] = ProviderConfig{Enabled: false} // no upstream/key, but disabled
	if err := cfg.Validate(); err != nil {
		t.Fatalf("disabled provider should not be validated: %v", err)
	}
}

func TestValidate_RSSRangeInverted(t *testing.T) {
	cfg := validConfig()
	cfg.Process.RSSRange = [2]int64{500, 100}
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for inverted RSS range")
	}
}

func TestValidate_DeviceIDTooShort(t *testing.T) {
	cfg := validConfig()
	cfg.Identity.DeviceID = "abc123" // 6 chars, less than 16
	err := cfg.Validate()
	if err == nil {
		t.Fatal("expected error for short device_id")
	}
	if !containsStr(err.Error(), "at least 16") {
		t.Errorf("error %q should mention length requirement", err.Error())
	}
}

func TestValidate_DeviceIDAutoAllowed(t *testing.T) {
	cfg := validConfig()
	cfg.Identity.DeviceID = "auto"
	if err := cfg.Validate(); err != nil {
		t.Fatalf("device_id 'auto' should pass: %v", err)
	}
}

func TestValidate_DeviceIDEmptyAllowed(t *testing.T) {
	cfg := validConfig()
	cfg.Identity.DeviceID = ""
	if err := cfg.Validate(); err != nil {
		t.Fatalf("empty device_id should pass: %v", err)
	}
}

func TestLoadFromYAML(t *testing.T) {
	tmpDir := t.TempDir()
	yamlContent := `
server:
  listen_port: 8443
  ca_download_port: 9090
  ca_dir: /tmp/ca
  cert_cache_dir: /tmp/certs
providers:
  anthropic:
    enabled: true
    upstream: "https://api.anthropic.com"
    domains: ["api.anthropic.com"]
    api_key: "sk-ant-test"
identity:
  device_id: "auto"
  email: "test@example.com"
logging:
  level: debug
  audit: true
`
	path := filepath.Join(tmpDir, "config.yaml")
	os.WriteFile(path, []byte(yamlContent), 0644)

	cfg, err := LoadFromYAML(path)
	if err != nil {
		t.Fatalf("LoadFromYAML failed: %v", err)
	}
	if cfg.Server.ListenPort != 8443 {
		t.Errorf("listen_port: got %d, want 8443", cfg.Server.ListenPort)
	}
	if cfg.Server.MaxBodySize != 10*1024*1024 {
		t.Errorf("max_body_size default: got %d, want %d", cfg.Server.MaxBodySize, 10*1024*1024)
	}
	if cfg.Server.RetryCount != 2 {
		t.Errorf("retry_count default: got %d, want 2", cfg.Server.RetryCount)
	}
	if !cfg.Upstream["anthropic"].Enabled {
		t.Error("anthropic should be enabled")
	}
}

func TestLoadFromYAML_FileNotFound(t *testing.T) {
	_, err := LoadFromYAML("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestApplyDefaults(t *testing.T) {
	cfg := &Config{}
	applyDefaults(cfg)
	if cfg.Server.ListenPort != 443 {
		t.Errorf("default listen_port: got %d, want 443", cfg.Server.ListenPort)
	}
	if cfg.Server.CADownloadPort != 8080 {
		t.Errorf("default ca_download_port: got %d, want 8080", cfg.Server.CADownloadPort)
	}
	if cfg.Server.MaxBodySize != 10*1024*1024 {
		t.Errorf("default max_body_size: got %d", cfg.Server.MaxBodySize)
	}
	if cfg.Server.RetryCount != 2 {
		t.Errorf("default retry_count: got %d", cfg.Server.RetryCount)
	}
	if cfg.Logging.Level != "info" {
		t.Errorf("default log_level: got %s", cfg.Logging.Level)
	}
}

func TestAllDomains(t *testing.T) {
	cfg := validConfig()
	cfg.Upstream["a"] = ProviderConfig{Enabled: true, Domains: []string{"a.com", "b.com"}}
	cfg.Upstream["b"] = ProviderConfig{Enabled: true, Domains: []string{"b.com", "c.com"}}
	cfg.Upstream["c"] = ProviderConfig{Enabled: false, Domains: []string{"d.com"}}

	domains := cfg.AllDomains()
	seen := make(map[string]bool)
	for _, d := range domains {
		seen[d] = true
	}
	if !seen["a.com"] || !seen["b.com"] || !seen["c.com"] {
		t.Errorf("missing expected domains: %v", domains)
	}
	if seen["d.com"] {
		t.Error("disabled provider domain should not appear")
	}
	// b.com should be deduplicated
	count := 0
	for _, d := range domains {
		if d == "b.com" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("b.com should appear once, got %d", count)
	}
}

func TestProviderForDomain(t *testing.T) {
	cfg := validConfig()
	name, p, ok := cfg.ProviderForDomain("api.anthropic.com")
	if !ok {
		t.Fatal("should find anthropic")
	}
	if name != "anthropic" {
		t.Errorf("name: got %s, want anthropic", name)
	}
	if p.Upstream != "https://api.anthropic.com" {
		t.Errorf("upstream mismatch: %s", p.Upstream)
	}

	_, _, ok = cfg.ProviderForDomain("unknown.com")
	if ok {
		t.Error("should not find unknown domain")
	}
}

func validConfig() *Config {
	return &Config{
		Server: ServerConfig{
			ListenPort:     443,
			CADownloadPort: 8080,
			CADir:          "/tmp/ca",
			CertCacheDir:   "/tmp/certs",
			MaxBodySize:    10 * 1024 * 1024,
			RetryCount:     2,
		},
		Upstream: map[string]ProviderConfig{
			"anthropic": {
				Enabled:  true,
				Upstream: "https://api.anthropic.com",
				Domains:  []string{"api.anthropic.com"},
				APIKey:   "sk-ant-test-key",
			},
		},
		Identity: IdentityConfig{
			DeviceID: "auto",
			Email:    "test@example.com",
		},
		Process: ProcessConfig{
			RSSRange:       [2]int64{100, 200},
			HeapTotalRange: [2]int64{40, 80},
			HeapUsedRange:  [2]int64{100, 200},
		},
		Logging: LoggingConfig{
			Level: "info",
			Audit: true,
		},
	}
}

func containsStr(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsSubstring(s, substr))
}

func containsSubstring(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
