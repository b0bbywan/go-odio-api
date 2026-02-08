package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

type Server struct {
	mux    *http.ServeMux
	config *config.ApiConfig
}

func NewServer(cfg *config.ApiConfig, b *backend.Backend) *Server {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	server := &Server{
		mux:    http.NewServeMux(),
		config: cfg,
	}
	server.register(b)
	return server
}

func (s *Server) Run(ctx context.Context) error {
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", s.config.Port),
		Handler: s.mux,
	}

	// Goroutine for signal handling
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Info("[api] Server shutdown error: %v", err)
		}
	}()

	logger.Info("[api] http server running on %d", s.config.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server error: %w", err)
	}

	return nil

}

func (s *Server) register(b *backend.Backend) {
	if b == nil {
		return
	}

	// server routes
	s.registerServerRoutes(b)

	if b.Bluetooth != nil {
		s.registerBluetoothRoutes(b.Bluetooth)
	}

	// pulse routes
	if b.Pulse != nil {
		s.registerPulseRoutes(b.Pulse)
	}

	// systemd routes
	if b.Systemd != nil {
		s.registerSystemdRoutes(b.Systemd)
	}

	// mpris routes
	if b.MPRIS != nil {
		s.registerMPRISRoutes(b.MPRIS)
	}
}
