package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// NewPairingListener creates a new pairing listener
func NewPairingListener(backend *BluetoothBackend, ctx context.Context) *PairingListener {
	listenerCtx, cancel := context.WithCancel(ctx)

	return &PairingListener{
		backend: backend,
		ctx:     listenerCtx,
		cancel:  cancel,
		signals: make(chan *dbus.Signal, 10),
	}
}

// Start starts listening to D-Bus signals for device connection attempts
func (l *PairingListener) Start() error {
	// Register signal channel
	l.backend.conn.Signal(l.signals)

	// Subscribe to PropertiesChanged signals for Bluetooth devices
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',path_namespace='/org/bluez'"
	if err := l.backend.addMatchRule(matchRule); err != nil {
		return err
	}

	go l.listen(matchRule)

	logger.Info("[bluetooth] pairing listener started, waiting for device connection attempts")
	return nil
}

// listen continuously listens to D-Bus signals
func (l *PairingListener) listen(matchRule string) {
	defer func() {
		l.backend.removeMatchRule(matchRule)
		l.backend.conn.RemoveSignal(l.signals)
	}()

	for {
		select {
		case <-l.ctx.Done():
			logger.Debug("[bluetooth] pairing listener stopped")
			return
		case sig, ok := <-l.signals:
			if !ok {
				logger.Debug("[bluetooth] signal channel closed")
				return
			}
			l.handleSignal(sig)
		}
	}
}

// handleSignal processes a D-Bus signal
func (l *PairingListener) handleSignal(sig *dbus.Signal) {
	// Only handle PropertiesChanged signals
	if sig.Name != DBUS_PROP_CHANGED_SIGNAL {
		return
	}

	// sig.Body[0] = interface name (should be "org.bluez.Device1")
	// sig.Body[1] = changed properties map
	// sig.Body[2] = invalidated properties (optional)
	if len(sig.Body) < 2 {
		return
	}

	iface, ok := sig.Body[0].(string)
	if !ok || iface != BLUETOOTH_DEVICE {
		return
	}

	props, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return
	}

	// Check if Connected property changed to true
	connectedVar, hasConnected := props[BT_PROP_CONNECTED]
	if !hasConnected {
		return
	}

	connected, ok := connectedVar.Value().(bool)
	if !ok || !connected {
		return
	}

	// A device is trying to connect!
	devicePath := sig.Path
	logger.Info("[bluetooth] device %v attempting connection, initiating pairing", devicePath)

	// Check if already trusted
	trusted, ok := l.backend.isDeviceTrusted(devicePath)
	if !ok {
		logger.Debug("[bluetooth] unable to check trust state for %v", devicePath)
		return
	}

	if trusted {
		logger.Debug("[bluetooth] device %v is already trusted", devicePath)
		return
	}

	// Attempt pairing
	if err := l.backend.pairDevice(devicePath); err != nil {
		logger.Warn("[bluetooth] pairing failed for %v: %v", devicePath, err)
		return
	}

	// After successful pairing, trust the device
	if ok := l.backend.trustDevice(devicePath); ok {
		logger.Info("[bluetooth] device %v paired and trusted successfully", devicePath)
		l.backend.refreshKnownDevices()
		// Stop the listener - pairing is complete
		l.Stop()
	} else {
		logger.Warn("[bluetooth] device %v paired but failed to trust", devicePath)
	}
}

// Stop stops the listener
func (l *PairingListener) Stop() {
	logger.Debug("[bluetooth] stopping pairing listener")
	l.cancel()
}
