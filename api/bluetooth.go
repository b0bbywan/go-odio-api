package api

import (
	"encoding/json"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/bluetooth"
)

func handleBluetoothError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func withBluetoothAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleBluetoothError(w, action())
	}
}

func handleBluetoothStatus(b *bluetooth.BluetoothBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		status := b.GetStatus()
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}
}
