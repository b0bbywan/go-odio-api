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

	// Context global pour toute l'application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize backends
	b, err := backend.New(ctx, cfg.Systemd, cfg.Pulseaudio, cfg.MPRIS)
	if err != nil {
		logger.Fatal("[%s] Backend initialization failed: %v", config.AppName, err)
	}

	// systemd backend
	if err := b.Start(); err != nil {
		logger.Fatal("[%s] Backend start failed: %v", config.AppName, err)
	}

	server := api.NewServer(http.NewServeMux(), cfg.Api, b)

	// Goroutine pour signal handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("[%s] Shutdown signal received, stopping server...", config.AppName)

		// Cancel le context global - arrête tous les listeners
		cancel()

		// Cleanup des backends
		b.Close()
	}()

	logger.Info("[%s] started", config.AppName)
	if server != nil {
		if err := server.Run(ctx); err != nil && err != http.ErrServerClosed {
			logger.Error("[%s] http server error: %v", config.AppName, err)
		}
	}
	logger.Info("[%s] stopped", config.AppName)
}
