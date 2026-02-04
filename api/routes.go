package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend"
)

func Register(mux *http.ServeMux, b *backend.Backend) {
	// pulse routes
	mux.HandleFunc(
		"/audio/server",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.Pulse.ServerInfo()
		}),
	)
	mux.HandleFunc(
		"POST /audio/server/mute",
		MuteMasterHandler(b.Pulse),
	)
	mux.HandleFunc(
		"POST /audio/server/volume",
		SetVolumeMasterHandler(b.Pulse),
	)
	mux.HandleFunc(
		"/audio/clients",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.Pulse.ListClients()
		}),
	)
	mux.HandleFunc(
		"POST /audio/clients/{sink}/mute",
		MuteClientHandler(b.Pulse),
	)
	mux.HandleFunc(
		"POST /audio/clients/{sink}/volume",
		SetVolumeClientHandler(b.Pulse),
	)

	// systemd routes
	mux.HandleFunc(
		"/services",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.Systemd.ListServices()
		}),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/enable",
		withService(b.Systemd, b.Systemd.EnableService),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/disable",
		withService(b.Systemd, b.Systemd.DisableService),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/restart",
		withService(b.Systemd, b.Systemd.RestartService),
	)

	// mpris routes
	if b.MPRIS != nil {
		mux.HandleFunc(
			"/players",
			ListPlayersHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/play",
			PlayHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/pause",
			PauseHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/play_pause",
			PlayPauseHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/stop",
			StopHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/next",
			NextHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/previous",
			PreviousHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/seek",
			SeekHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/position",
			SetPositionHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/volume",
			SetVolumeHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/loop",
			SetLoopHandler(b.MPRIS),
		)
		mux.HandleFunc(
			"POST /players/{player}/shuffle",
			SetShuffleHandler(b.MPRIS),
		)
	}
}
