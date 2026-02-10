package api

import (
	"net/http"
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
