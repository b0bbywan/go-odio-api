package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/b0bbywan/go-odio-api/ui"
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
		Addr:    s.config.Listen,
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

	logger.Info("[api] http server running on %s", s.config.Listen)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server error: %w", err)
	}

	return nil

}

func (s *Server) register(b *backend.Backend) {
	if b == nil {
		return
	}

	// 404 on root for security
	s.mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.NotFound(w, r)
			return
		}
		// Return 404 for all other unmatched paths
		http.NotFound(w, r)
	})

	// UI routes
	s.registerUIRoutes()

	// server routes
	s.registerServerRoutes(b)

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

func (s *Server) registerUIRoutes() {
	uiHandler := ui.NewHandler(s.config.Port)
	uiHandler.RegisterRoutes(s.mux)
	logger.Info("[api] UI routes registered at /ui")
}
