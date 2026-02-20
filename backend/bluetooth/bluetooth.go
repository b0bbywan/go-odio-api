package bluetooth

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New creates a new Bluetooth backend
func New(ctx context.Context, cfg *config.BluetoothConfig) (*BluetoothBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	backend := BluetoothBackend{
		conn:           conn,
		ctx:            ctx,
		timeout:        cfg.Timeout,
		pairingTimeout: cfg.PairingTimeout,
		idleTimeout:    cfg.IdleTimeout,
		statusCache:    cache.New[BluetoothStatus](0), // no expiration
	}

	if err = backend.CheckBluetoothSupport(); err != nil {
		logger.Error("[bluetooth] Not Supported")
		backend.Close()
		return nil, nil
	}

	logger.Info("[bluetooth] backend started (powered off)")
	return &backend, nil
}

func (b *BluetoothBackend) PowerUp() error {
	if powered := b.isAdapterOn(); powered {
		return nil
	}

	if err := b.PowerOnAdapter(true); err != nil {
		return err
	}

	if err := b.SetDiscoverableAndPairable(false); err != nil {
		return err
	}

	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = true
	})
	b.refreshKnownDevices()

	b.startListener()

	logger.Info("[bluetooth] Bluetooth ready to connect to already known devices")
	return nil
}

func (b *BluetoothBackend) startListener() {
	matchRules := []string{
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Device1'",
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Adapter1'",
	}
	logger.Debug("[bluetooth] starting listener (device + adapter)")
	listenerCtx, cancel := context.WithCancel(b.ctx)
	listener := NewDBusListener(b.conn, listenerCtx, matchRules, b.onPropertiesChanged)
	if err := listener.Start(); err != nil {
		cancel()
		logger.Warn("[bluetooth] failed to start listener: %v", err)
		return
	}
	b.listener = listener
	b.listenerCancel = cancel
	go listener.Listen()
	logger.Debug("[bluetooth] listener started")
}

func (b *BluetoothBackend) stopListener() {
	if b.listenerCancel != nil {
		b.listenerCancel()
		b.listenerCancel = nil
	}
	if b.listener != nil {
		b.listener.Stop()
		b.listener = nil
	}
}

func (b *BluetoothBackend) PowerDown() error {
	if powered := b.isAdapterOn(); !powered {
		return nil
	}

	b.stopListener()
	b.cancelIdleTimer()

	if err := b.PowerOnAdapter(false); err != nil {
		return err
	}

	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = false
		s.Discoverable = false
		s.Pairable = false
		s.PairingActive = false
		s.PairingUntil = nil
	})

	return nil
}

func (b *BluetoothBackend) NewPairing() error {
	// Prevent resetting BlueZ timeouts on an already-active pairing session
	if b.isDiscoverable() {
		logger.Info("[bluetooth] pairing already in progress")
		return nil
	}

	// RegisterAgent
	if err := b.registerAgent(); err != nil {
		if dbusErr, ok := err.(*dbus.Error); ok && dbusErr.Name == "org.bluez.Error.AlreadyExists" {
			logger.Info("[bluetooth] agent already registered")
		} else {
			logger.Warn("[bluetooth] failed to register agent: %v", err)
			return err
		}
	}

	// Bluetooth ON
	if powered := b.isAdapterOn(); !powered {
		if err := b.PowerOnAdapter(true); err != nil {
			return err
		}
	}

	// Set BlueZ native timeouts
	if err := b.SetTimeOut(DISCOVERABLE_TIMEOUT); err != nil {
		return err
	}

	if err := b.SetTimeOut(PAIRABLE_TIMEOUT); err != nil {
		return err
	}

	// Enable pairing mode
	if err := b.SetDiscoverableAndPairable(true); err != nil {
		return err
	}

	pairingUntil := time.Now().Add(b.pairingTimeout)
	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = true
		s.PairingActive = true
		s.PairingUntil = &pairingUntil
	})

	logger.Info("[bluetooth] Bluetooth pairing mode enabled")
	return nil
}

func (b *BluetoothBackend) onPropertiesChanged(sig *dbus.Signal) bool {
	if sig == nil {
		return true // channel closed
	}

	if len(sig.Body) < 2 {
		logger.Debug("[bluetooth] signal from %s ignored: body too short", sig.Path)
		return false
	}

	iface, ok := sig.Body[0].(string)
	if !ok {
		return false
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		logger.Debug("[bluetooth] signal from %s ignored: body[1] is not map[string]Variant", sig.Path)
		return false
	}

	logger.Debug("[bluetooth] signal from %s (%s): changed properties=%v", sig.Path, iface, changedKeys(changed))

	switch iface {
	case BLUETOOTH_DEVICE:
		b.onDevicePropertiesChanged(sig.Path, changed)
	case BLUETOOTH_ADAPTER:
		b.onAdapterPropertiesChanged(changed)
	}

	return false
}

func (b *BluetoothBackend) onDevicePropertiesChanged(path dbus.ObjectPath, changed map[string]dbus.Variant) {
	if connectedVar, ok := changed["Connected"]; ok {
		if connected, ok := connectedVar.Value().(bool); ok {
			logger.Debug("[bluetooth] device %s Connected=%v", path, connected)
			if connected {
				b.cancelIdleTimer()
			} else if b.idleTimeout > 0 {
				b.checkAndStartIdleTimer()
			}
			b.refreshKnownDevices()
		}
	}

	if pairedVar, ok := changed["Paired"]; ok {
		if paired, ok := pairedVar.Value().(bool); ok && paired {
			logger.Info("[bluetooth] device %s paired successfully", path)
			if b.trustDevice(path) {
				logger.Info("[bluetooth] device %s trusted", path)
				b.refreshKnownDevices()
			} else {
				logger.Warn("[bluetooth] failed to trust device %s", path)
			}
			if err := b.SetDiscoverableAndPairable(false); err != nil {
				logger.Warn("[bluetooth] failed to stop pairing mode: %v", err)
			}
		}
	}
}

func (b *BluetoothBackend) onAdapterPropertiesChanged(changed map[string]dbus.Variant) {
	if discoverableVar, ok := changed[BT_STATE_DISCOVERABLE.toString()]; ok {
		if discoverable, ok := discoverableVar.Value().(bool); ok {
			logger.Debug("[bluetooth] adapter Discoverable=%v", discoverable)
			b.updateStatus(func(s *BluetoothStatus) {
				s.Discoverable = discoverable
			})
		}
	}

	if pairableVar, ok := changed[BT_STATE_PAIRABLE.toString()]; ok {
		if pairable, ok := pairableVar.Value().(bool); ok {
			logger.Debug("[bluetooth] adapter Pairable=%v", pairable)
			b.updateStatus(func(s *BluetoothStatus) {
				s.Pairable = pairable
				if !pairable {
					s.PairingActive = false
					s.PairingUntil = nil
				}
			})
		}
	}
}

func (b *BluetoothBackend) cancelIdleTimer() {
	b.idleTimerMu.Lock()
	defer b.idleTimerMu.Unlock()

	if b.idleTimer != nil {
		b.idleTimer.Stop()
		b.idleTimer = nil
		logger.Info("[bluetooth] idle timer cancelled")
	}
}

func (b *BluetoothBackend) checkAndStartIdleTimer() {
	if b.hasConnectedDevices() {
		logger.Debug("[bluetooth] still has connected devices, skipping idle timer")
		return
	}

	b.idleTimerMu.Lock()
	defer b.idleTimerMu.Unlock()

	if b.idleTimer != nil {
		logger.Debug("[bluetooth] idle timer already running, skipping")
		return
	}

	b.idleTimer = time.AfterFunc(b.idleTimeout, func() {
		logger.Info("[bluetooth] idle timeout reached after %v, powering down", b.idleTimeout)
		if err := b.PowerOnAdapter(false); err != nil {
			logger.Warn("[bluetooth] failed to power down: %v", err)
		}
		b.idleTimerMu.Lock()
		b.idleTimer = nil
		b.idleTimerMu.Unlock()
	})
	logger.Info("[bluetooth] idle timer started (%v)", b.idleTimeout)
}

func changedKeys(changed map[string]dbus.Variant) []string {
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	return keys
}

func (b *BluetoothBackend) Close() {
	b.unregisterAgent()
	if err := b.PowerDown(); err != nil {
		logger.Warn("[bluetooth] Failed power off adapter at shutdown: %v", err)
	}

	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			logger.Warn("[bluetooth] Failed to close D-Bus connection: %v", err)
		}
		b.conn = nil
	}
}

func (b *BluetoothBackend) GetStatus() BluetoothStatus {
	const statusKey = "current"
	status, ok := b.statusCache.Get(statusKey)
	if !ok {
		return BluetoothStatus{}
	}
	return status
}

func (b *BluetoothBackend) updateStatus(fn func(*BluetoothStatus)) {
	const statusKey = "current"
	status, _ := b.statusCache.Get(statusKey)
	fn(&status)
	b.statusCache.Set(statusKey, status)
}

func (b *BluetoothBackend) refreshKnownDevices() {
	if b.conn == nil {
		return
	}
	devices, err := b.listKnownDevices()
	if err != nil {
		logger.Warn("[bluetooth] failed to list known devices: %v", err)
		return
	}
	b.updateStatus(func(s *BluetoothStatus) {
		s.KnownDevices = devices
	})
}

func (b *BluetoothBackend) registerAgent() error {
	if b.agent != nil {
		return nil
	}

	agent := bluezAgent{backend: b}
	if err := b.exportAgent(&agent); err != nil {
		return err
	}

	manager := b.getObj(BLUETOOTH_PREFIX, BLUEZ_PATH)
	if err := b.RequestNoInputOutputAgent(manager); err != nil {
		return err
	}

	b.agent = &agent
	logger.Info("[bluetooth] agent successfully registered")
	return nil
}
