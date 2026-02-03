package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/b0bbywan/go-odio-api/api"
	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

func main() {
	cfg, err := config.New()
	if err != nil {
		logger.Fatal("Failed to load config: %v", err)
	}

	// Set log level from config
	logger.SetLevel(cfg.LogLevel)

	// Context global pour toute l'application
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// PulseAudio backend
	b, err := backend.New(ctx, cfg.Systemd, cfg.Pulseaudio)
	if err != nil {
		logger.Fatal("Backend initialization failed: %v", err)
	}

	// systemd backend
	if err := b.Start(); err != nil {
		logger.Fatal("Backend start failed: %v", err)
	}

	mux := http.NewServeMux()
	api.Register(mux, b)

	port := fmt.Sprintf(":%d",cfg.Port)
	// HTTP server
	srv := &http.Server{
		Addr:    port,
		Handler: mux,
	}

	// Goroutine pour signal handling
	go func() {
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
		<-sigChan

		logger.Info("Shutdown signal received, stopping server...")

		// Cancel le context global - arrête tous les listeners
		cancel()

		// Cleanup des backends
		b.Close()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Error("Server shutdown error: %v", err)
		}
	}()

	logger.Info("Odio Audio API running on %s", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Error("Server error: %v", err)
	}
	logger.Info("Server stopped")
}
