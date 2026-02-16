package api

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
	"github.com/b0bbywan/go-odio-api/ui"
)

type Server struct {
	mux    *http.ServeMux
	config *config.ApiConfig
	ui     bool
}

func NewServer(cfg *config.ApiConfig, b *backend.Backend) *Server {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	server := &Server{
		mux:    http.NewServeMux(),
		config: cfg,
		ui:     cfg.UI != nil && cfg.UI.Enabled,
	}
	server.register(b)
	return server
}

func (s *Server) Run(ctx context.Context) error {
	servers := make([]*http.Server, len(s.config.Listens))
	for i, addr := range s.config.Listens {
		servers[i] = &http.Server{Addr: addr, Handler: s.mux}
	}

	// Shutdown all servers on context cancellation
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		for _, srv := range servers {
			if err := srv.Shutdown(shutdownCtx); err != nil {
				logger.Info("[api] server %s shutdown error: %v", srv.Addr, err)
			}
		}
	}()

	// Start one goroutine per listen address
	errCh := make(chan error, len(servers))
	var wg sync.WaitGroup
	for _, srv := range servers {
		wg.Add(1)
		go func(srv *http.Server) {
			defer wg.Done()
			logger.Info("[api] http server running on %s", srv.Addr)
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				errCh <- fmt.Errorf("server %s: %w", srv.Addr, err)
			}
		}(srv)
	}

	wg.Wait()
	close(errCh)
	return <-errCh
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

	// server routes
	s.registerServerRoutes(b)

	// UI routes
	if s.ui {
		s.registerUIRoutes()
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

func (s *Server) registerUIRoutes() {
	uiHandler := ui.NewHandler(s.config.Port)
	uiHandler.RegisterRoutes(s.mux)
	logger.Info("[api] UI routes registered at /ui")
}
