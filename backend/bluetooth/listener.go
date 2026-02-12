package bluetooth

import (
	"context"
	"time"

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
// Pairing Listener - Auto-managed lifecycle like MPRIS heartbeat
// ============================================================================

// PairingListener listens for device connections and auto-stops after pairing
type PairingListener struct {
	backend *BluetoothBackend
	ctx     context.Context
	cancel  context.CancelFunc
	signals chan *dbus.Signal
}

// NewPairingListener creates a pairing listener with auto-managed lifecycle
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

// run is the main pairing loop (like MPRIS heartbeat)
func (l *PairingListener) run(matchRule string) {
	defer func() {
		if err := l.backend.removeMatchRule(matchRule); err != nil {
			logger.Warn("[bluetooth] failed to remove pairing match rule: %v", err)
		}
		l.backend.conn.RemoveSignal(l.signals)
		logger.Debug("[bluetooth] pairing listener stopped")
	}()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-l.ctx.Done():
			return

		case <-ticker.C:
			// Periodic check: did we pair a device?
			hasPaired := l.checkPairingComplete()
			if hasPaired {
				logger.Info("[bluetooth] device paired, auto-stopping listener")
				return // Auto-stop like MPRIS heartbeat ✓
			}

		case sig, ok := <-l.signals:
			if !ok {
				logger.Debug("[bluetooth] pairing signal channel closed")
				return
			}
			l.handleSignal(sig)
		}
	}
}

// handleSignal processes PropertiesChanged signals for device connections
func (l *PairingListener) handleSignal(sig *dbus.Signal) {
	// Only handle PropertiesChanged signals
	if sig.Name != DBUS_PROP_CHANGED_SIGNAL {
		return
	}

	if len(sig.Body) < 2 {
		return
	}

	// Check interface is org.bluez.Device1
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

	// Device attempting connection - try to pair
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
		// Note: listener will auto-stop on next ticker check
	} else {
		logger.Warn("[bluetooth] device %v paired but failed to trust", devicePath)
	}
}

// checkPairingComplete checks if any device has been successfully paired
// Returns true if pairing is complete (should stop listening)
func (l *PairingListener) checkPairingComplete() bool {
	devices, err := l.backend.listKnownDevices()
	if err != nil {
		return false
	}

	// Check if we have at least one trusted device that wasn't there before
	// For simplicity, we check if ANY device is both Connected and Trusted
	for _, device := range devices {
		if device.Connected && device.Trusted {
			logger.Debug("[bluetooth] found paired device: %s (%s)", device.Name, device.Address)
			return true
		}
	}

	return false
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
//     handler := func(sig *dbus.Signal) {
//         // Track last activity time
//         // If no devices connected for idleTimeout, power off
//         ...
//     }
//
//     return NewBluetoothListener(backend, ctx, matchRule, filter, handler, "idle")
// }
