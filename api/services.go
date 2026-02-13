package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

// handleSystemdError handles systemd errors and returns the appropriate HTTP response
func handleSystemdError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Handle system scope permission errors - always forbidden
	var permSysErr *systemd.PermissionSystemError
	if errors.As(err, &permSysErr) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// Handle user scope permission errors - forbidden for non-whitelisted units
	var permUserErr *systemd.PermissionUserError
	if errors.As(err, &permUserErr) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	// All other errors are internal server errors
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

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

		handleSystemdError(w, fn(unit, scope))
	}
}
