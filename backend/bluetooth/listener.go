package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// NewBluetoothListener creates a new generic Bluetooth D-Bus signal listener
func NewBluetoothListener(
	backend *BluetoothBackend,
	ctx context.Context,
	matchRule string,
	filter SignalFilter,
	handler SignalHandler,
	name string,
) *BluetoothListener {
	listenerCtx, cancel := context.WithCancel(ctx)

	return &BluetoothListener{
		backend:   backend,
		ctx:       listenerCtx,
		cancel:    cancel,
		signals:   make(chan *dbus.Signal, 10),
		matchRule: matchRule,
		filter:    filter,
		handler:   handler,
		name:      name,
	}
}

// Start starts listening to D-Bus signals
func (l *BluetoothListener) Start() error {
	// Register signal channel
	l.backend.conn.Signal(l.signals)

	// Subscribe to signals with the provided match rule
	if err := l.backend.addMatchRule(l.matchRule); err != nil {
		return err
	}

	go l.listen()

	logger.Info("[bluetooth] %s listener started", l.name)
	return nil
}

// listen continuously listens to D-Bus signals
func (l *BluetoothListener) listen() {
	defer func() {
		l.backend.removeMatchRule(l.matchRule)
		l.backend.conn.RemoveSignal(l.signals)
		logger.Debug("[bluetooth] %s listener stopped", l.name)
	}()

	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-l.signals:
			if !ok {
				logger.Debug("[bluetooth] %s signal channel closed", l.name)
				return
			}
			l.handleSignal(sig)
		}
	}
}

// handleSignal processes a D-Bus signal using the filter and handler
func (l *BluetoothListener) handleSignal(sig *dbus.Signal) {
	// Apply filter - skip if not interested
	if l.filter != nil && !l.filter(sig) {
		return
	}

	// Execute handler
	if l.handler != nil {
		l.handler(sig)
	}
}

// Stop stops the listener
func (l *BluetoothListener) Stop() {
	logger.Debug("[bluetooth] stopping %s listener", l.name)
	l.cancel()
}

// ============================================================================
// Pairing Listener - Detects device connection attempts during pairing mode
// ============================================================================

// NewPairingListener creates a listener for device pairing
func NewPairingListener(backend *BluetoothBackend, ctx context.Context) *BluetoothListener {
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',path_namespace='/org/bluez'"

	filter := func(sig *dbus.Signal) bool {
		// Only handle PropertiesChanged signals
		if sig.Name != DBUS_PROP_CHANGED_SIGNAL {
			return false
		}

		if len(sig.Body) < 2 {
			return false
		}

		// Check interface is org.bluez.Device1
		iface, ok := sig.Body[0].(string)
		if !ok || iface != BLUETOOTH_DEVICE {
			return false
		}

		props, ok := sig.Body[1].(map[string]dbus.Variant)
		if !ok {
			return false
		}

		// Check if Connected property changed to true
		connectedVar, hasConnected := props[BT_PROP_CONNECTED]
		if !hasConnected {
			return false
		}

		connected, ok := connectedVar.Value().(bool)
		return ok && connected
	}

	handler := func(sig *dbus.Signal) {
		devicePath := sig.Path
		logger.Info("[bluetooth] device %v attempting connection, initiating pairing", devicePath)

		// Check if already trusted
		trusted, ok := backend.isDeviceTrusted(devicePath)
		if !ok {
			logger.Debug("[bluetooth] unable to check trust state for %v", devicePath)
			return
		}

		if trusted {
			logger.Debug("[bluetooth] device %v is already trusted", devicePath)
			return
		}

		// Attempt pairing
		if err := backend.pairDevice(devicePath); err != nil {
			logger.Warn("[bluetooth] pairing failed for %v: %v", devicePath, err)
			return
		}

		// After successful pairing, trust the device
		if ok := backend.trustDevice(devicePath); ok {
			logger.Info("[bluetooth] device %v paired and trusted successfully", devicePath)
			backend.refreshKnownDevices()
			// Note: caller should stop the listener after successful pairing
		} else {
			logger.Warn("[bluetooth] device %v paired but failed to trust", devicePath)
		}
	}

	return NewBluetoothListener(backend, ctx, matchRule, filter, handler, "pairing")
}

// ============================================================================
// Future: Idle Listener - Detects when no devices are connected for X time
// ============================================================================

// NewIdleListener creates a listener to detect Bluetooth inactivity
// Useful for auto-powering off Bluetooth after 30 minutes of no connections
//
// Example usage (commented out for now):
// func NewIdleListener(backend *BluetoothBackend, ctx context.Context, idleTimeout time.Duration) *BluetoothListener {
//     matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',path_namespace='/org/bluez'"
//
//     filter := func(sig *dbus.Signal) bool {
//         // Filter for Connected property changes
//         ...
//     }
//
//     handler := func(sig *dbus.Signal) {
//         // Track last activity time
//         // If no devices connected for idleTimeout, power off
//         ...
//     }
//
//     return NewBluetoothListener(backend, ctx, matchRule, filter, handler, "idle")
// }
