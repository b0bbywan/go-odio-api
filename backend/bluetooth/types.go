package bluetooth

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"
)

type BluetoothBackend struct {
	conn           *dbus.Conn
	ctx            context.Context
	timeout        time.Duration
	pairingTimeout time.Duration
	agent          *bluezAgent
	pairingMu      sync.Mutex
}

type dbusTimeoutError struct{}

func (e *dbusTimeoutError) Error() string {
	return "D-Bus call timeout"
}

type bluetoothUnsupportedError struct{}

func (e *bluetoothUnsupportedError) Error() string {
	return "bluetooth not supported"
}
