package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/bluetooth"
)

type bluetoothAddressRequest struct {
	Address string `json:"address"`
}

func handleBluetoothError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if errors.Is(err, bluetooth.ErrInvalidAddress) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// Everything else here is a BlueZ/device operation failure upstream of us.
	http.Error(w, err.Error(), http.StatusBadGateway)
}

func withBluetoothAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleBluetoothError(w, action())
	}
}

// withBluetoothAddress decodes a {"address": "..."} body and runs an
// address-keyed action; the address itself is validated by the backend.
func withBluetoothAddress(action func(string) error) http.HandlerFunc {
	return withBody(nil, func(w http.ResponseWriter, r *http.Request, req *bluetoothAddressRequest) {
		handleBluetoothError(w, action(req.Address))
	})
}
