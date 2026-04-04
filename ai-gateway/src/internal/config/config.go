package config

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the runtime configuration, loaded from UCI or a YAML file.
type Config struct {
	Server   ServerConfig              `yaml:"server"`
	Upstream map[string]ProviderConfig `yaml:"providers"` // keyed by provider name
	Identity IdentityConfig            `yaml:"identity"`
	Env      EnvConfig                 `yaml:"env"`
	PromptEnv PromptEnvConfig          `yaml:"prompt_env"`
	Process  ProcessConfig             `yaml:"process"`
	Logging  LoggingConfig             `yaml:"logging"`
}

type ServerConfig struct {
	ListenPort     int    `yaml:"listen_port"`
	CADownloadPort int    `yaml:"ca_download_port"`
	CADir          string `yaml:"ca_dir"`
	CertCacheDir   string `yaml:"cert_cache_dir"`
}

type ProviderConfig struct {
	Enabled           bool     `yaml:"enabled"`
	Upstream          string   `yaml:"upstream"`
	Domains           []string `yaml:"domains"`
	APIKey            string   `yaml:"api_key"`
	OAuthAccessToken  string   `yaml:"oauth_access_token"`
	OAuthRefreshToken string   `yaml:"oauth_refresh_token"`
}

type IdentityConfig struct {
	DeviceID string `yaml:"device_id"`
	Email    string `yaml:"email"`
}

type EnvConfig struct {
	Platform              string `yaml:"platform"`
	PlatformRaw           string `yaml:"platform_raw"`
	Arch                  string `yaml:"arch"`
	NodeVersion           string `yaml:"node_version"`
	Terminal              string `yaml:"terminal"`
	PackageManagers       string `yaml:"package_managers"`
	Runtimes              string `yaml:"runtimes"`
	IsRunningWithBun      bool   `yaml:"is_running_with_bun"`
	IsCI                  bool   `yaml:"is_ci"`
	IsClaudeAIAuth        bool   `yaml:"is_claude_ai_auth"`
	Version               string `yaml:"version"`
	VersionBase           string `yaml:"version_base"`
	BuildTime             string `yaml:"build_time"`
	DeploymentEnvironment string `yaml:"deployment_environment"`
	VCS                   string `yaml:"vcs"`
}

type PromptEnvConfig struct {
	Platform   string `yaml:"platform"`
	Shell      string `yaml:"shell"`
	OSVersion  string `yaml:"os_version"`
	WorkingDir string `yaml:"working_dir"`
}

type ProcessConfig struct {
	ConstrainedMemory int64    `yaml:"constrained_memory"`
	RSSRange          [2]int64 `yaml:"rss_range"`
	HeapTotalRange    [2]int64 `yaml:"heap_total_range"`
	HeapUsedRange     [2]int64 `yaml:"heap_used_range"`
}

type LoggingConfig struct {
	Level string `yaml:"level"`
	Audit bool   `yaml:"audit"`
}

// LoadFromYAML loads config from a YAML file.
func LoadFromYAML(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	applyDefaults(&cfg)
	return &cfg, nil
}

// LoadFromUCI reads config from OpenWrt UCI system.
func LoadFromUCI() (*Config, error) {
	cfg := &Config{}

	// Global settings
	cfg.Server.ListenPort = uciGetInt("ai_gateway.global.listen_port", 443)
	cfg.Server.CADownloadPort = uciGetInt("ai_gateway.global.ca_download_port", 8080)
	cfg.Server.CADir = uciGet("ai_gateway.global.ca_dir", "/etc/ai-gateway/ca")
	cfg.Server.CertCacheDir = uciGet("ai_gateway.global.cert_cache_dir", "/tmp/ai-gateway/certs")
	cfg.Logging.Level = uciGet("ai_gateway.global.log_level", "info")
	cfg.Logging.Audit = uciGet("ai_gateway.global.audit", "1") == "1"

	// Identity
	cfg.Identity.DeviceID = uciGet("ai_gateway.canonical.device_id", "")
	cfg.Identity.Email = uciGet("ai_gateway.canonical.email", "user@example.com")

	// Env fingerprint
	cfg.Env.Platform = uciGet("ai_gateway.canonical.platform", "darwin")
	cfg.Env.PlatformRaw = uciGet("ai_gateway.canonical.platform_raw", "darwin")
	cfg.Env.Arch = uciGet("ai_gateway.canonical.arch", "arm64")
	cfg.Env.NodeVersion = uciGet("ai_gateway.canonical.node_version", "v24.3.0")
	cfg.Env.Terminal = uciGet("ai_gateway.canonical.terminal", "iTerm2.app")
	cfg.Env.PackageManagers = uciGet("ai_gateway.canonical.package_managers", "npm,pnpm")
	cfg.Env.Runtimes = uciGet("ai_gateway.canonical.runtimes", "node")
	cfg.Env.IsClaudeAIAuth = uciGet("ai_gateway.canonical.is_claude_ai_auth", "1") == "1"
	cfg.Env.Version = uciGet("ai_gateway.canonical.version", "2.1.81")
	cfg.Env.VersionBase = uciGet("ai_gateway.canonical.version_base", "2.1.81")
	cfg.Env.BuildTime = uciGet("ai_gateway.canonical.build_time", "2026-03-20T21:26:18Z")
	cfg.Env.DeploymentEnvironment = uciGet("ai_gateway.canonical.deployment_environment", "unknown-darwin")
	cfg.Env.VCS = uciGet("ai_gateway.canonical.vcs", "git")

	// Prompt env
	cfg.PromptEnv.Platform = uciGet("ai_gateway.canonical.prompt_platform", cfg.Env.Platform)
	cfg.PromptEnv.Shell = uciGet("ai_gateway.canonical.prompt_shell", "zsh")
	cfg.PromptEnv.OSVersion = uciGet("ai_gateway.canonical.prompt_os_version", "Darwin 24.4.0")
	cfg.PromptEnv.WorkingDir = uciGet("ai_gateway.canonical.prompt_working_dir", "/Users/jack/projects")

	// Process metrics
	cfg.Process.ConstrainedMemory = int64(uciGetInt("ai_gateway.canonical.constrained_memory", 34359738368))
	cfg.Process.RSSRange = [2]int64{
		int64(uciGetInt("ai_gateway.canonical.rss_min", 300000000)),
		int64(uciGetInt("ai_gateway.canonical.rss_max", 500000000)),
	}
	cfg.Process.HeapTotalRange = [2]int64{
		int64(uciGetInt("ai_gateway.canonical.heap_total_min", 40000000)),
		int64(uciGetInt("ai_gateway.canonical.heap_total_max", 80000000)),
	}
	cfg.Process.HeapUsedRange = [2]int64{
		int64(uciGetInt("ai_gateway.canonical.heap_used_min", 100000000)),
		int64(uciGetInt("ai_gateway.canonical.heap_used_max", 200000000)),
	}

	// Providers
	cfg.Upstream = make(map[string]ProviderConfig)

	for _, name := range []string{"anthropic", "openai", "gemini"} {
		section := "ai_gateway." + name
		enabled := uciGet(section+".enabled", "0") == "1"
		if !enabled {
			continue
		}
		p := ProviderConfig{
			Enabled:           true,
			Upstream:          uciGet(section+".upstream", ""),
			APIKey:            uciGet(section+".api_key", ""),
			OAuthAccessToken:  uciGet(section+".oauth_access_token", ""),
			OAuthRefreshToken: uciGet(section+".oauth_refresh_token", ""),
		}
		p.Domains = uciGetList(section + ".domains")
		if len(p.Domains) == 0 {
			switch name {
			case "anthropic":
				p.Domains = []string{"api.anthropic.com"}
				if p.Upstream == "" {
					p.Upstream = "https://api.anthropic.com"
				}
			case "openai":
				p.Domains = []string{"api.openai.com"}
				if p.Upstream == "" {
					p.Upstream = "https://api.openai.com"
				}
			case "gemini":
				p.Domains = []string{"generativelanguage.googleapis.com"}
				if p.Upstream == "" {
					p.Upstream = "https://generativelanguage.googleapis.com"
				}
			}
		}
		cfg.Upstream[name] = p
	}

	applyDefaults(cfg)
	return cfg, nil
}

// AllDomains returns a deduplicated list of all intercepted domains.
func (c *Config) AllDomains() []string {
	seen := make(map[string]bool)
	var result []string
	for _, p := range c.Upstream {
		if !p.Enabled {
			continue
		}
		for _, d := range p.Domains {
			if !seen[d] {
				seen[d] = true
				result = append(result, d)
			}
		}
	}
	return result
}

// ProviderForDomain returns the provider config matching the given domain.
func (c *Config) ProviderForDomain(domain string) (string, *ProviderConfig, bool) {
	for name, p := range c.Upstream {
		if !p.Enabled {
			continue
		}
		for _, d := range p.Domains {
			if d == domain {
				return name, &p, true
			}
		}
	}
	return "", nil, false
}

func applyDefaults(cfg *Config) {
	if cfg.Server.ListenPort == 0 {
		cfg.Server.ListenPort = 443
	}
	if cfg.Server.CADownloadPort == 0 {
		cfg.Server.CADownloadPort = 8080
	}
	if cfg.Server.CADir == "" {
		cfg.Server.CADir = "/etc/ai-gateway/ca"
	}
	if cfg.Server.CertCacheDir == "" {
		cfg.Server.CertCacheDir = "/tmp/ai-gateway/certs"
	}
	if cfg.Logging.Level == "" {
		cfg.Logging.Level = "info"
	}
	if cfg.Env.PlatformRaw == "" {
		cfg.Env.PlatformRaw = cfg.Env.Platform
	}
	if cfg.Env.VersionBase == "" {
		cfg.Env.VersionBase = cfg.Env.Version
	}
	if cfg.Process.ConstrainedMemory == 0 {
		cfg.Process.ConstrainedMemory = 34359738368
	}
	if cfg.Process.RSSRange == [2]int64{0, 0} {
		cfg.Process.RSSRange = [2]int64{300000000, 500000000}
	}
	if cfg.Process.HeapTotalRange == [2]int64{0, 0} {
		cfg.Process.HeapTotalRange = [2]int64{40000000, 80000000}
	}
	if cfg.Process.HeapUsedRange == [2]int64{0, 0} {
		cfg.Process.HeapUsedRange = [2]int64{100000000, 200000000}
	}
}

// uciGet calls `uci get <key>` and returns the result, or defaultVal on error.
func uciGet(key, defaultVal string) string {
	out, err := exec.Command("uci", "get", key).Output()
	if err != nil {
		return defaultVal
	}
	return strings.TrimSpace(string(out))
}

// uciGetInt calls `uci get <key>` and parses as int, or returns defaultVal.
func uciGetInt(key string, defaultVal int) int {
	s := uciGet(key, "")
	if s == "" {
		return defaultVal
	}
	var v int
	if _, err := fmt.Sscanf(s, "%d", &v); err != nil {
		return defaultVal
	}
	return v
}

// uciGetList calls `uci get <key>` for list options.
func uciGetList(key string) []string {
	out, err := exec.Command("uci", "get", key).Output()
	if err != nil {
		return nil
	}
	s := strings.TrimSpace(string(out))
	if s == "" {
		return nil
	}
	return strings.Fields(s)
}
