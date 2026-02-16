package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/backend/login1"
	"github.com/b0bbywan/go-odio-api/backend/mpris"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

func (s *Server) registerServerRoutes(b *backend.Backend) {
	s.mux.HandleFunc(
		"/server",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.GetServerDeviceInfo()
		}),
	)
}

func (s *Server) registerLogin1Routes(b *login1.Login1Backend) {
	return
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
		"POST /services/{scope}/{unit}/start",
		withService(b, b.StartService),
	)
	s.mux.HandleFunc(
		"POST /services/{scope}/{unit}/stop",
		withService(b, b.StopService),
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
