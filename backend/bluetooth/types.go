package bluetooth

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

// managedTimer is a self-locking one-shot timer handle shared by the idle and
// scan auto-stop timers.
type managedTimer struct {
	mu    sync.Mutex
	timer *time.Timer
}

// Start arms fn after d and reports whether it armed a new timer. A zero d
// disables it; if already armed, it is left untouched (no reset).
func (t *managedTimer) Start(d time.Duration, fn func()) bool {
	if d == 0 {
		return false
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer != nil {
		return false
	}
	t.timer = time.AfterFunc(d, fn)
	return true
}

// Cancel stops the timer if armed and reports whether it did.
func (t *managedTimer) Cancel() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.timer == nil {
		return false
	}
	t.timer.Stop()
	t.timer = nil
	return true
}

type BluetoothBackend struct {
	conn           *dbus.Conn
	ctx            context.Context
	timeout        time.Duration
	pairingTimeout time.Duration
	idleTimeout    time.Duration
	powerOnStart   bool
	agent          *bluezAgent
	idleTimer      managedTimer
	listener       *DBusListener
	// permanent cache (no expiration) for status tracking
	statusCache *cache.Cache[BluetoothStatus]
	events      chan events.Event
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
