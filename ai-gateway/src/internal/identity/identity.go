package identity

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"math/big"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

// GenerateDeviceID generates a random 64-character hex device ID.
func GenerateDeviceID() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("failed to generate random device ID: %v", err))
	}
	return hex.EncodeToString(b)
}

// RandomInRange returns a random int64 in [min, max).
func RandomInRange(min, max int64) int64 {
	if min >= max {
		return min
	}
	diff := max - min
	n, _ := rand.Int(rand.Reader, big.NewInt(diff))
	return min + n.Int64()
}

// BuildCanonicalEnv constructs the full canonical environment object from config.
func BuildCanonicalEnv(cfg *config.Config) map[string]any {
	return map[string]any{
		"platform":               cfg.Env.Platform,
		"platform_raw":           cfg.Env.PlatformRaw,
		"arch":                   cfg.Env.Arch,
		"node_version":           cfg.Env.NodeVersion,
		"terminal":               cfg.Env.Terminal,
		"package_managers":       cfg.Env.PackageManagers,
		"runtimes":               cfg.Env.Runtimes,
		"is_running_with_bun":    cfg.Env.IsRunningWithBun,
		"is_ci":                  false,
		"is_claubbit":            false,
		"is_claude_code_remote":  false,
		"is_local_agent_mode":    false,
		"is_conductor":           false,
		"is_github_action":       false,
		"is_claude_code_action":  false,
		"is_claude_ai_auth":      cfg.Env.IsClaudeAIAuth,
		"version":                cfg.Env.Version,
		"version_base":           cfg.Env.VersionBase,
		"build_time":             cfg.Env.BuildTime,
		"deployment_environment": cfg.Env.DeploymentEnvironment,
		"vcs":                    cfg.Env.VCS,
	}
}

// BuildCanonicalProcess generates realistic process metrics from config ranges.
func BuildCanonicalProcess(original map[string]any, cfg *config.Config) map[string]any {
	result := make(map[string]any)
	for k, v := range original {
		result[k] = v
	}
	result["constrainedMemory"] = cfg.Process.ConstrainedMemory
	result["rss"] = RandomInRange(cfg.Process.RSSRange[0], cfg.Process.RSSRange[1])
	result["heapTotal"] = RandomInRange(cfg.Process.HeapTotalRange[0], cfg.Process.HeapTotalRange[1])
	result["heapUsed"] = RandomInRange(cfg.Process.HeapUsedRange[0], cfg.Process.HeapUsedRange[1])
	return result
}
