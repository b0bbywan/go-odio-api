package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

func Register(mux *http.ServeMux, pa *pulseaudio.PulseAudioBackend, sd *systemd.SystemdBackend) {
	// pulse routes
	mux.HandleFunc(
		"/audio/server",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return pa.ServerInfo()
		}),
	)
	mux.HandleFunc(
		"/audio/clients",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return pa.ListClients()
		}),
	)
	mux.HandleFunc(
		"POST /audio/clients/{sink}/mute", 
		MuteClientHandler(pa),
	)

	// systemd routes
	mux.HandleFunc(
		"/services", 
		 JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return sd.ListServices()
		}),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/enable",
		withService(sd, sd.EnableService),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/disable",
		withService(sd, sd.DisableService),
	)
	mux.HandleFunc(
		"POST /services/{scope}/{unit}/restart",
		withService(sd, sd.RestartService),
	)
}
