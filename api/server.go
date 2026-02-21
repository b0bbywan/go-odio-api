package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"slices"
	"sync"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

type Server struct {
	mux         *http.ServeMux
	config      *config.ApiConfig
	ui          bool
	broadcaster *backend.Broadcaster
}

func NewServer(ctx context.Context, cfg *config.ApiConfig, b *backend.Backend) *Server {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	var broadcaster *backend.Broadcaster
	if b != nil {
		broadcaster = b.NewBroadcaster(ctx)
	}

	server := &Server{
		mux:         http.NewServeMux(),
		config:      cfg,
		ui:          cfg.UI != nil && cfg.UI.Enabled,
		broadcaster: broadcaster,
	}
	server.register(b)
	return server
}

func (s *Server) Run(ctx context.Context) error {
	var handler http.Handler = s.mux
	if s.config.CORS != nil {
		handler = corsMiddleware(s.config.CORS)(handler)
	}

	servers := make([]*http.Server, len(s.config.Listens))
	for i, addr := range s.config.Listens {
		servers[i] = &http.Server{
			Addr:    addr,
			Handler: handler,
			// Derive request contexts from ctx so that long-lived handlers
			// (e.g. SSE) exit cleanly when the application shuts down,
			// without waiting for the graceful-shutdown timeout.
			BaseContext: func(_ net.Listener) context.Context { return ctx },
		}
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

	if b.Login1 != nil {
		s.registerLogin1Routes(b.Login1)
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

func corsMiddleware(cfg *config.CORSConfig) func(http.Handler) http.Handler {
	wildcard := slices.Contains(cfg.Origins, "*")
	logger.Info("[api] CORS enabled, origins: %v", cfg.Origins)

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			origin := r.Header.Get("Origin")
			if origin != "" {
				if wildcard {
					w.Header().Set("Access-Control-Allow-Origin", "*")
				} else if slices.Contains(cfg.Origins, origin) {
					w.Header().Set("Access-Control-Allow-Origin", origin)
					w.Header().Add("Vary", "Origin")
				}
			}

			if r.Method == http.MethodOptions {
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
				w.WriteHeader(http.StatusNoContent)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}
