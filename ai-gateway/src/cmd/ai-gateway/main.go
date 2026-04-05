package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid config: %v\n", err)
		os.Exit(1)
	}

	// Auto-generate device_id if not set
	if cfg.Identity.DeviceID == "" || cfg.Identity.DeviceID == "auto" {
		cfg.Identity.DeviceID = identity.GenerateDeviceID()
		logger.Info("Auto-generated device_id: %s...", cfg.Identity.DeviceID[:8])
	}

	logger.SetLevel(cfg.Logging.Level)

	logger.Info("AI Gateway %s starting...", version)
	if len(cfg.Identity.DeviceID) >= 8 {
		logger.Info("Canonical device_id: %s...", cfg.Identity.DeviceID[:8])
	} else {
		logger.Info("Canonical device_id: %s", cfg.Identity.DeviceID)
	}
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

	// Error channel to catch server failures
	errCh := make(chan error, 2)

	// Start CA download HTTP server in background
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("CA download server panic: %v", r)
				errCh <- fmt.Errorf("CA download server panic: %v", r)
			}
		}()
		if err := proxy.StartCADownloadServer(cfg, tlsMgr); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("CA download server: %w", err)
		}
	}()

	// Start HTTPS proxy server in background
	srv := proxy.NewServer(cfg, tlsMgr)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				logger.Error("Proxy server panic: %v", r)
				errCh <- fmt.Errorf("proxy server panic: %v", r)
			}
		}()
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("proxy server: %w", err)
		}
	}()

	// Wait for shutdown signal or fatal server error
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	select {
	case sig := <-sigCh:
		logger.Info("Received %s, shutting down...", sig)
	case err := <-errCh:
		logger.Error("Fatal server error: %v", err)
	}

	// Graceful shutdown: drain active connections with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("Shutdown error: %v", err)
	}
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
