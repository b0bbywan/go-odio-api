package api

import (
	"net/http"
)

// handleBluetoothError standardise la gestion des erreurs Bluetooth
func handleBluetoothError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// withBluetoothActionPOST générique (POST uniquement)
func withBluetoothAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleBluetoothError(w, action())
	}
}
