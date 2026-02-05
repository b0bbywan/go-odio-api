package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/backend/mpris"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

type Server struct {
	mux    *http.ServeMux
	config *config.ApiConfig
}

func NewServer(mux *http.ServeMux, cfg *config.ApiConfig, b *backend.Backend) *Server {
	if cfg == nil || !cfg.Enabled {
		return nil
	}

	server := &Server{
		mux:    mux,
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

	// Goroutine pour signal handling
	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			logger.Info("Server shutdown error: %v", err)
		}
	}()

	logger.Info("Odio Audio API running on %d", s.config.Port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("Server error: %w", err)
	}

	return nil

}

func (s *Server) register(b *backend.Backend) {
	if b == nil {
		return
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

func (s *Server) registerPulseRoutes(b *pulseaudio.PulseAudioBackend) {
	s.mux.HandleFunc(
		"/audio/server",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.ServerInfo()
		}),
	)
	s.mux.HandleFunc(
		"POST /audio/server/mute",
		MuteMasterHandler(b),
	)
	s.mux.HandleFunc(
		"POST /audio/server/volume",
		SetVolumeMasterHandler(b),
	)
	s.mux.HandleFunc(
		"/audio/clients",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.ListClients()
		}),
	)
	s.mux.HandleFunc(
		"POST /audio/clients/{sink}/mute",
		MuteClientHandler(b),
	)
	s.mux.HandleFunc(
		"POST /audio/clients/{sink}/volume",
		SetVolumeClientHandler(b),
	)
}

func (s *Server) registerSystemdRoutes(b *systemd.SystemdBackend) {
	s.mux.HandleFunc(
		"/services",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.ListServices()
		}),
	)
	s.mux.HandleFunc(
		"POST /services/{scope}/{unit}/enable",
		withService(b, b.EnableService),
	)
	s.mux.HandleFunc(
		"POST /services/{scope}/{unit}/disable",
		withService(b, b.DisableService),
	)
	s.mux.HandleFunc(
		"POST /services/{scope}/{unit}/restart",
		withService(b, b.RestartService),
	)
}

func (s *Server) registerMPRISRoutes(b *mpris.MPRISBackend) {
	s.mux.HandleFunc(
		"/players",
		ListPlayersHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/play",
		PlayHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/pause",
		PauseHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/play_pause",
		PlayPauseHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/stop",
		StopHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/next",
		NextHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/previous",
		PreviousHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/seek",
		SeekHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/position",
		SetPositionHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/volume",
		SetVolumeHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/loop",
		SetLoopHandler(b),
	)
	s.mux.HandleFunc(
		"POST /players/{player}/shuffle",
		SetShuffleHandler(b),
	)
}
