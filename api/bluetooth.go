package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/bluetooth"
)

func handleBluetoothError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Check for specific error types
	var pairingInProgress *bluetooth.PairingInProgressError
	if errors.As(err, &pairingInProgress) {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func withBluetoothAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleBluetoothError(w, action())
	}
}
