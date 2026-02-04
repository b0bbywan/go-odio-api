package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

func withService(
	sd *systemd.SystemdBackend,
	fn func(string, systemd.UnitScope) error,
) http.HandlerFunc {

	return func(w http.ResponseWriter, r *http.Request) {
		scope, ok := systemd.ParseUnitScope(r.PathValue("scope"))
		if !ok {
			http.Error(w, "invalid scope", http.StatusNotFound)
			return
		}

		unit := r.PathValue("unit")
		if unit == "" {
			http.Error(w, "missing unit name", http.StatusNotFound)
			return
		}

		if err := fn(unit, scope); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
