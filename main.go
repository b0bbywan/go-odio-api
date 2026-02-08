package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/b0bbywan/go-odio-api/api"
	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		logger.Fatal("[%s] Failed to load config: %v", config.AppName, err)
	}

	// Set log level from config
	logger.SetLevel(cfg.LogLevel)

	// Global context for the entire application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize backends
	b, err := backend.New(ctx, cfg.Systemd, cfg.Pulseaudio, cfg.MPRIS, cfg.Zeroconf)
	if err != nil {
		logger.Fatal("[%s] Backend initialization failed: %v", config.AppName, err)
	}

	// Start enabled backend
	if err := b.Start(); err != nil {
		logger.Fatal("[%s] Backend start failed: %v", config.AppName, err)
	}

	// New api server
	server := api.NewServer(cfg.Api, b)

	// Channel to synchronize shutdown
	shutdownDone := make(chan struct{})
	// Goroutine for signal handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("[%s] Shutdown signal received, stopping server...", config.AppName)

		// Cancel the global context - stops all listeners
		cancel()

		// Cleanup backends
		b.Close()

		// Signal that cleanup is complete
		close(shutdownDone)
	}()

	logger.Info("[%s] started", config.AppName)
	if server != nil {
		if err := server.Run(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("[%s] http server error: %v", config.AppName, err)
		}
	}

	<-shutdownDone
	logger.Info("[%s] stopped", config.AppName)
}
