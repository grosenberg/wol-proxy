package main

import (
	"log"

	"github.com/grosenberg/wol-proxy/internal/config"
	"github.com/grosenberg/wol-proxy/internal/proxy"
	"github.com/grosenberg/wol-proxy/pkg/logger"
)

func main() {
	// Load configuration
	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Setup logging
	logger.Setup(cfg.LogLevel)

	// Create and run proxy server
	proxyServer := proxy.NewServer(cfg)
	if err := proxyServer.Run(); err != nil {
		log.Fatalf("Failed to run proxy: %v", err)
	}
}
