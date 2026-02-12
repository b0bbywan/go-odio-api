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
		if err := l.backend.removeMatchRule(l.matchRule); err != nil {
			logger.Warn("[bluetooth] failed to remove match rule for %s listener: %v", l.name, err)
		}
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
			// Handler returns true to continue, false to stop
			if !l.handleSignal(sig) {
				return
			}
		}
	}
}

// handleSignal processes a D-Bus signal using the filter and handler.
// Returns true to continue listening, false to stop.
func (l *BluetoothListener) handleSignal(sig *dbus.Signal) bool {
	// Apply filter - skip if not interested
	if l.filter != nil && !l.filter(sig) {
		return true // Continue
	}

	// Execute handler - it returns true/false to control lifecycle
	if l.handler != nil {
		return l.handler(sig)
	}

	return true // Continue by default
}

// Stop stops the listener
func (l *BluetoothListener) Stop() {
	logger.Debug("[bluetooth] stopping %s listener", l.name)
	l.cancel()
}

// ============================================================================
// Pairing Listener - Simple event-driven, context-managed lifecycle
// ============================================================================

// PairingListener listens for device connections during pairing mode.
// Lifecycle managed by context timeout - stops when:
// 1. Device successfully paired (handler returns false)
// 2. Context timeout expires (from waitPairing)
type PairingListener struct {
	backend *BluetoothBackend
	ctx     context.Context
	cancel  context.CancelFunc
	signals chan *dbus.Signal
}

// NewPairingListener creates a pairing listener
func NewPairingListener(backend *BluetoothBackend, ctx context.Context) *PairingListener {
	listenerCtx, cancel := context.WithCancel(ctx)

	return &PairingListener{
		backend: backend,
		ctx:     listenerCtx,
		cancel:  cancel,
		signals: make(chan *dbus.Signal, 10),
	}
}

// Start starts the pairing listener
func (l *PairingListener) Start() error {
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',path_namespace='/org/bluez'"

	l.backend.conn.Signal(l.signals)

	if err := l.backend.addMatchRule(matchRule); err != nil {
		return err
	}

	go l.run(matchRule)

	logger.Info("[bluetooth] pairing listener started")
	return nil
}

// run is the main pairing loop - simple event-driven
func (l *PairingListener) run(matchRule string) {
	defer func() {
		if err := l.backend.removeMatchRule(matchRule); err != nil {
			logger.Warn("[bluetooth] failed to remove pairing match rule: %v", err)
		}
		l.backend.conn.RemoveSignal(l.signals)
		logger.Debug("[bluetooth] pairing listener stopped")
	}()

	for {
		select {
		case <-l.ctx.Done():
			// Timeout or explicit stop
			return

		case sig, ok := <-l.signals:
			if !ok {
				logger.Debug("[bluetooth] pairing signal channel closed")
				return
			}
			// Handle signal - returns false if pairing successful (stop listening)
			if !l.handleSignal(sig) {
				logger.Info("[bluetooth] pairing successful, stopping listener")
				return
			}
		}
	}
}

// handleSignal processes PropertiesChanged signals for device connections.
// Returns true to continue listening, false to stop (pairing successful).
func (l *PairingListener) handleSignal(sig *dbus.Signal) bool {
	// Only handle PropertiesChanged signals
	if sig.Name != DBUS_PROP_CHANGED_SIGNAL {
		return true
	}

	if len(sig.Body) < 2 {
		return true
	}

	// Check interface is org.bluez.Device1
	iface, ok := sig.Body[0].(string)
	if !ok || iface != BLUETOOTH_DEVICE {
		return true
	}

	props, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return true
	}

	// Check if Connected property changed to true
	connectedVar, hasConnected := props[BT_PROP_CONNECTED]
	if !hasConnected {
		return true
	}

	connected, ok := connectedVar.Value().(bool)
	if !ok || !connected {
		return true
	}

	// Device attempting connection - try to pair
	devicePath := sig.Path
	logger.Info("[bluetooth] device %v attempting connection, initiating pairing", devicePath)

	// Check if already trusted
	trusted, ok := l.backend.isDeviceTrusted(devicePath)
	if !ok {
		logger.Debug("[bluetooth] unable to check trust state for %v", devicePath)
		return true // Continue listening
	}

	if trusted {
		logger.Debug("[bluetooth] device %v is already trusted", devicePath)
		return true // Continue listening
	}

	// Attempt pairing
	if err := l.backend.pairDevice(devicePath); err != nil {
		logger.Warn("[bluetooth] pairing failed for %v: %v", devicePath, err)
		return true // Continue listening for next device
	}

	// After successful pairing, trust the device
	if ok := l.backend.trustDevice(devicePath); ok {
		logger.Info("[bluetooth] device %v paired and trusted successfully", devicePath)
		l.backend.refreshKnownDevices()
		return false // Stop listening - mission complete!
	}

	logger.Warn("[bluetooth] device %v paired but failed to trust", devicePath)
	return true // Continue listening
}

// Stop stops the pairing listener
func (l *PairingListener) Stop() {
	logger.Debug("[bluetooth] stopping pairing listener")
	l.cancel()
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
//     handler := func(sig *dbus.Signal) bool {
//         // Track last activity time
//         // If no devices connected for idleTimeout, power off
//         ...
//         return true // Continue
//     }
//
//     return NewBluetoothListener(backend, ctx, matchRule, filter, handler, "idle")
// }
