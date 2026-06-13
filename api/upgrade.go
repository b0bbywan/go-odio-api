package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/upgrade"
)

func (s *Server) registerUpgradeRoutes(b *upgrade.UpgradeBackend) {
	s.mux.HandleFunc(
		"GET /upgrade",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.StatusResponse(), nil
		}),
	)
	s.mux.HandleFunc(
		"POST /upgrade/check",
		withUpgradeAction(b.CheckNow),
	)
	s.mux.HandleFunc(
		"POST /upgrade/start",
		withUpgradeAction(b.StartUpgrade),
	)
}

func withUpgradeAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		err := action()
		switch {
		case err == nil:
			w.WriteHeader(http.StatusAccepted)
		case errors.Is(err, upgrade.ErrUnitNotConfigured):
			http.Error(w, err.Error(), http.StatusNotFound)
		case errors.Is(err, upgrade.ErrUpgradeInProgress):
			http.Error(w, err.Error(), http.StatusConflict)
		default:
			// systemd/D-Bus trigger failure upstream of us.
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	}
}
