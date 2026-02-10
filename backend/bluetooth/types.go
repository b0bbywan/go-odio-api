package bluetooth

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
)

type BluetoothBackend struct {
	conn           *dbus.Conn
	ctx            context.Context
	timeout        time.Duration
	pairingTimeout time.Duration
	agent          *bluezAgent
	pairingMu      sync.Mutex
	// permanent cache (no expiration) for status tracking
	statusCache *cache.Cache[BluetoothStatus]
}

type dbusTimeoutError struct{}

func (e *dbusTimeoutError) Error() string {
	return "D-Bus call timeout"
}

type bluetoothUnsupportedError struct{}

func (e *bluetoothUnsupportedError) Error() string {
	return "bluetooth not supported"
}

// BluetoothDevice represents a known Bluetooth device
type BluetoothDevice struct {
	Address   string `json:"address"`
	Name      string `json:"name"`
	Trusted   bool   `json:"trusted"`
	Connected bool   `json:"connected"`
}

// BluetoothStatus represents the current Bluetooth state
type BluetoothStatus struct {
	Powered       bool              `json:"powered"`
	Discoverable  bool              `json:"discoverable"`
	Pairable      bool              `json:"pairable"`
	PairingActive bool              `json:"pairing_active"`
	PairingUntil  *time.Time        `json:"pairing_until,omitempty"`
	KnownDevices  []BluetoothDevice `json:"known_devices,omitempty"`
}
