package bluetooth

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

type BluetoothBackend struct {
	dbus           *idbus.DBusBackend
	conn           *dbus.Conn // borrowed from dbus, not owned
	ctx            context.Context
	pairingTimeout time.Duration
	idleTimeout    time.Duration
	agent          *bluezAgent
	idleTimer      *time.Timer
	idleTimerMu    sync.Mutex
	sigCh          chan *dbus.Signal
	// permanent cache (no expiration) for status tracking
	statusCache *cache.Cache[BluetoothStatus]
	events      chan events.Event
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
