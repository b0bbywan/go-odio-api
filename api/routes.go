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
}
