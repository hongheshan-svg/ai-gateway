package identity

import (
	"encoding/hex"
	"testing"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
)

func TestGenerateDeviceID(t *testing.T) {
	id := GenerateDeviceID()

	if len(id) != 64 {
		t.Errorf("expected 64-char device ID, got %d chars", len(id))
	}

	// Must be valid hex
	if _, err := hex.DecodeString(id); err != nil {
		t.Errorf("device ID is not valid hex: %v", err)
	}

	// Two calls should produce different IDs
	id2 := GenerateDeviceID()
	if id == id2 {
		t.Error("two GenerateDeviceID calls should produce different values")
	}
}

func TestRandomInRange(t *testing.T) {
	min, max := int64(100), int64(200)
	for i := 0; i < 100; i++ {
		val := RandomInRange(min, max)
		if val < min || val >= max {
			t.Errorf("RandomInRange(%d, %d) = %d, out of range", min, max, val)
		}
	}
}

func TestRandomInRangeEqualMinMax(t *testing.T) {
	val := RandomInRange(42, 42)
	if val != 42 {
		t.Errorf("RandomInRange(42, 42) = %d, want 42", val)
	}
}

func TestBuildCanonicalEnv(t *testing.T) {
	cfg := &config.Config{
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
	}

	env := BuildCanonicalEnv(cfg)

	expected := map[string]any{
		"platform":               "darwin",
		"arch":                   "arm64",
		"node_version":           "v24.3.0",
		"terminal":               "iTerm2.app",
		"version":                "2.1.81",
		"vcs":                    "git",
		"is_ci":                  false,
		"is_claubbit":            false,
		"is_claude_code_remote":  false,
		"is_local_agent_mode":    false,
		"is_conductor":           false,
		"is_github_action":       false,
		"is_claude_code_action":  false,
	}

	for key, want := range expected {
		if got, ok := env[key]; !ok {
			t.Errorf("missing key: %s", key)
		} else if got != want {
			t.Errorf("env[%s] = %v, want %v", key, got, want)
		}
	}
}

func TestBuildCanonicalProcess(t *testing.T) {
	cfg := &config.Config{
		Process: config.ProcessConfig{
			ConstrainedMemory: 34359738368,
			RSSRange:          [2]int64{300000000, 500000000},
			HeapTotalRange:    [2]int64{40000000, 80000000},
			HeapUsedRange:     [2]int64{100000000, 200000000},
		},
	}

	original := map[string]any{
		"uptime": 123.45,
	}

	proc := BuildCanonicalProcess(original, cfg)

	// Should preserve original fields
	if proc["uptime"] != 123.45 {
		t.Error("uptime should be preserved")
	}

	// Should have canonical fields
	if proc["constrainedMemory"] != int64(34359738368) {
		t.Errorf("constrainedMemory = %v", proc["constrainedMemory"])
	}

	rss, ok := proc["rss"].(int64)
	if !ok {
		t.Fatal("rss should be int64")
	}
	if rss < 300000000 || rss >= 500000000 {
		t.Errorf("rss out of range: %d", rss)
	}

	heapTotal, ok := proc["heapTotal"].(int64)
	if !ok {
		t.Fatal("heapTotal should be int64")
	}
	if heapTotal < 40000000 || heapTotal >= 80000000 {
		t.Errorf("heapTotal out of range: %d", heapTotal)
	}

	heapUsed, ok := proc["heapUsed"].(int64)
	if !ok {
		t.Fatal("heapUsed should be int64")
	}
	if heapUsed < 100000000 || heapUsed >= 200000000 {
		t.Errorf("heapUsed out of range: %d", heapUsed)
	}
}
