package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/login1"
)

// handleLogin1Error handles login1 errors and returns the appropriate HTTP response.
func handleLogin1Error(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var capErr *login1.CapabilityError
	if errors.As(err, &capErr) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// withLogin1 wraps a no-arg login1 action into an http.HandlerFunc.
func withLogin1(fn func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleLogin1Error(w, fn())
	}
}
