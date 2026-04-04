package main

import (
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/zhengshan/openwrt-ai-gateway/internal/config"
	"github.com/zhengshan/openwrt-ai-gateway/internal/identity"
	"github.com/zhengshan/openwrt-ai-gateway/internal/logger"
	"github.com/zhengshan/openwrt-ai-gateway/internal/proxy"
	tlsca "github.com/zhengshan/openwrt-ai-gateway/internal/tls"
)

var (
	version = "0.1.0"
	commit  = "dev"
)

func main() {
	configFile := flag.String("config", "", "Path to YAML config file (if not using UCI)")
	useUCI := flag.Bool("uci", false, "Load config from OpenWrt UCI")
	genIdentity := flag.Bool("gen-identity", false, "Generate a random device identity and exit")
	showVersion := flag.Bool("version", false, "Print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ai-gateway %s (%s)\n", version, commit)
		os.Exit(0)
	}

	if *genIdentity {
		deviceID := identity.GenerateDeviceID()
		fmt.Printf("device_id: %s\n", deviceID)
		os.Exit(0)
	}

	// Load configuration
	var cfg *config.Config
	var err error

	if *useUCI {
		cfg, err = config.LoadFromUCI()
	} else if *configFile != "" {
		cfg, err = config.LoadFromYAML(*configFile)
	} else {
		// Auto-detect: try UCI first (OpenWrt), then fallback to config.yaml
		cfg, err = config.LoadFromUCI()
		if err != nil {
			cfg, err = config.LoadFromYAML("config.yaml")
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", err)
		os.Exit(1)
	}

	// Auto-generate device_id if not set
	if cfg.Identity.DeviceID == "" || cfg.Identity.DeviceID == "auto" {
		cfg.Identity.DeviceID = identity.GenerateDeviceID()
		logger.Info("Auto-generated device_id: %s", cfg.Identity.DeviceID[:8]+"...")
	}

	logger.SetLevel(cfg.Logging.Level)

	logger.Info("AI Gateway %s starting...", version)
	logger.Info("Canonical device_id: %s...", cfg.Identity.DeviceID[:8])
	logger.Info("Canonical email: %s", cfg.Identity.Email)

	// Verify at least one provider is enabled
	enabledCount := 0
	for name, p := range cfg.Upstream {
		if p.Enabled {
			enabledCount++
			logger.Info("Provider [%s] enabled → %s (domains: %s)", name, p.Upstream, joinStrings(p.Domains))
		}
	}
	if enabledCount == 0 {
		fmt.Fprintf(os.Stderr, "No providers enabled. Enable at least one provider in config.\n")
		os.Exit(1)
	}

	// Initialize TLS CA manager
	tlsMgr, err := tlsca.NewManager(cfg.Server.CADir, cfg.Server.CertCacheDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize TLS CA: %v\n", err)
		os.Exit(1)
	}
	logger.Info("CA fingerprint: %s", tlsMgr.CACertFingerprint())

	// Start CA download HTTP server in background
	go func() {
		if err := proxy.StartCADownloadServer(cfg, tlsMgr); err != nil {
			logger.Error("CA download server failed: %v", err)
		}
	}()

	// Start HTTPS proxy server in background
	srv := proxy.NewServer(cfg, tlsMgr)
	go func() {
		if err := srv.Start(); err != nil {
			logger.Error("Proxy server failed: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	logger.Info("Received %s, shutting down...", sig)

	srv.Close()
	logger.Info("AI Gateway stopped.")
}

func joinStrings(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
