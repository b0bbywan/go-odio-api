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
		conn:      backend.conn,
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
	l.conn.Signal(l.signals)

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
		l.conn.RemoveSignal(l.signals)
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

	logger.Debug("[bluetooth] %s: signal matched filter (path=%v)", l.name, sig.Path)

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
// Shared helpers for listener construction
// ============================================================================

// devicePropertiesMatchRule returns the D-Bus match rule for BlueZ device property changes
func devicePropertiesMatchRule() string {
	return "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',path_namespace='/org/bluez'"
}

// deviceConnectedFilter returns a SignalFilter that passes PropertiesChanged signals
// for org.bluez.Device1 where the Connected property is present
func deviceConnectedFilter() SignalFilter {
	return func(sig *dbus.Signal) bool {
		if sig.Name != DBUS_PROP_CHANGED_SIGNAL || len(sig.Body) < 2 {
			return false
		}
		iface, ok := sig.Body[0].(string)
		if !ok || iface != BLUETOOTH_DEVICE {
			return false
		}
		props, ok := sig.Body[1].(map[string]dbus.Variant)
		if !ok {
			return false
		}
		_, hasConnected := props[BT_PROP_CONNECTED]
		if !hasConnected {
			for k, v := range props {
				switch k {
				case "RSSI", "TxPower", "ManufacturerData":
					continue
				default:
					logger.Debug("[bluetooth] device %v: property %s=%v (filtered out, not Connected)", sig.Path, k, v.Value())
				}
			}
		}
		return hasConnected
	}
}

// ============================================================================
// Pairing Listener
// ============================================================================

// NewPairingListener creates a listener that handles device pairing.
// When a new untrusted device connects, it pairs and trusts it, then stops.
// onPaired is called on successful pairing to signal completion (e.g. cancel a parent context).
func NewPairingListener(backend *BluetoothBackend, ctx context.Context, onPaired func()) *BluetoothListener {
	handler := func(sig *dbus.Signal) bool {
		props := sig.Body[1].(map[string]dbus.Variant)

		connected, ok := props[BT_PROP_CONNECTED].Value().(bool)
		if !ok || !connected {
			logger.Debug("[bluetooth] pairing: device %v Connected=%v (ignoring disconnect)", sig.Path, connected)
			return true
		}

		devicePath := sig.Path
		logger.Info("[bluetooth] device %v attempting connection, initiating pairing", devicePath)

		trusted, ok := backend.isDeviceTrusted(devicePath)
		if !ok {
			logger.Debug("[bluetooth] unable to check trust state for %v", devicePath)
			return true
		}

		if trusted {
			logger.Debug("[bluetooth] device %v is already trusted", devicePath)
			return true
		}

		if err := backend.pairDevice(devicePath); err != nil {
			logger.Warn("[bluetooth] pairing failed for %v: %v", devicePath, err)
			return true
		}

		if ok := backend.trustDevice(devicePath); ok {
			logger.Info("[bluetooth] device %v paired and trusted successfully", devicePath)
			backend.refreshKnownDevices()
			if onPaired != nil {
				onPaired()
			}
			return false // Stop listening - mission complete!
		}

		logger.Warn("[bluetooth] device %v paired but failed to trust", devicePath)
		return true
	}

	return NewBluetoothListener(backend, ctx, devicePropertiesMatchRule(), deviceConnectedFilter(), handler, "pairing")
}

// ============================================================================
// Idle Listener - Powers down Bluetooth after prolonged inactivity
// ============================================================================

// NewIdleListener creates a listener that monitors device connections.
// When no devices are connected for the specified idle timeout, it powers down Bluetooth.
func NewIdleListener(backend *BluetoothBackend, ctx context.Context, idleTimeout time.Duration) *BluetoothListener {
	var idleTimer *time.Timer

	handler := func(sig *dbus.Signal) bool {
		props := sig.Body[1].(map[string]dbus.Variant)

		connected, ok := props[BT_PROP_CONNECTED].Value().(bool)
		if !ok {
			return true
		}

		logger.Debug("[bluetooth] idle: device %v Connected=%v", sig.Path, connected)

		if connected {
			// A device connected - cancel idle timer
			if idleTimer != nil {
				idleTimer.Stop()
				idleTimer = nil
				logger.Debug("[bluetooth] idle timer cancelled - device connected")
			}
			return true
		}

		// A device disconnected - check if any device is still connected
		if backend.hasConnectedDevices() {
			logger.Debug("[bluetooth] device disconnected but other devices still connected")
			return true
		}

		// No devices connected - start/reset idle timer
		if idleTimer != nil {
			idleTimer.Stop()
		}
		logger.Info("[bluetooth] no connected devices, starting idle timer (%v)", idleTimeout)
		idleTimer = time.AfterFunc(idleTimeout, func() {
			logger.Info("[bluetooth] idle timeout reached, powering down")
			if err := backend.PowerDown(); err != nil {
				logger.Warn("[bluetooth] idle power down failed: %v", err)
			}
		})

		return true
	}

	return NewBluetoothListener(backend, ctx, devicePropertiesMatchRule(), deviceConnectedFilter(), handler, "idle")
}
