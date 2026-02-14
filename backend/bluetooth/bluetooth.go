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
		s.Discoverable = false
		s.Pairable = false
	})
	b.refreshKnownDevices()

	b.startIdleListener()

	logger.Info("[bluetooth] Bluetooth ready to connect to already known devices")
	return nil
}

func (b *BluetoothBackend) startIdleListener() {
	if b.idleTimeout == 0 {
		logger.Debug("[bluetooth] idle timeout disabled, skipping idle listener")
		return
	}
	matchRule := "type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Device1'"
	logger.Debug("[bluetooth] starting idle listener (timeout=%v, matchRule=%s)", b.idleTimeout, matchRule)
	listener := NewDBusListener(b.conn, b.ctx, matchRule, b.onDeviceConnectionChange)
	if err := listener.Start(); err != nil {
		logger.Warn("[bluetooth] failed to start idle timeout: %v", err)
		return
	}
	go listener.Listen()
	logger.Debug("[bluetooth] idle listener started")
}

func (b *BluetoothBackend) PowerDown() error {
	if powered := b.isAdapterOn(); !powered {
		return nil
	}

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
	// Prevent concurrent pairing sessions
	if !b.pairingMu.TryLock() {
		logger.Info("[bluetooth] pairing already in progress")
		return nil
	}
	// Unlock in NewPairing on error only
	unlocked := false
	defer func() {
		if !unlocked {
			b.pairingMu.Unlock()
		}
	}()

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

	// Timeouts (in seconds)
	if err := b.SetTimeOut(DISCOVERABLE_TIMEOUT); err != nil {
		return err
	}

	if err := b.SetTimeOut(PAIRABLE_TIMEOUT); err != nil {
		return err
	}

	// pairing mode
	if err := b.SetDiscoverableAndPairable(true); err != nil {
		return err
	}

	// Track pairing state
	pairingUntil := time.Now().Add(b.pairingTimeout)
	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = true
		s.Discoverable = true
		s.Pairable = true
		s.PairingActive = true
		s.PairingUntil = &pairingUntil
	})

	// Unlock in waitPairing
	unlocked = true
	go b.waitPairing(b.ctx)
	logger.Info("[bluetooth] Bluetooth pairing mode enabled")

	return nil
}

func (b *BluetoothBackend) waitPairing(ctx context.Context) {
	logger.Debug("[bluetooth] waitPairing started (timeout=%v)", b.pairingTimeout)
	subCtx, cancel := context.WithTimeout(ctx, b.pairingTimeout)
	defer func() {
		logger.Info("[bluetooth] resetting adapter state after pairing")
		if err := b.SetDiscoverableAndPairable(false); err != nil {
			logger.Warn("[bluetooth] failed to reset adapter state after pairing: %v", err)
		}
		b.updateStatus(func(s *BluetoothStatus) {
			s.Discoverable = false
			s.Pairable = false
			s.PairingActive = false
			s.PairingUntil = nil
		})
		cancel()
		b.pairingMu.Unlock()
		logger.Debug("[bluetooth] waitPairing cleanup complete, mutex released")
	}()

	matchRule := "type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Device1'"
	logger.Debug("[bluetooth] pairing listener matchRule=%s", matchRule)

	listener := NewDBusListener(b.conn, subCtx, matchRule, b.onDevicePaired)
	if err := listener.Start(); err != nil {
		logger.Warn("[bluetooth] failed to start listener: %v", err)
		return
	}
	defer listener.Stop()

	logger.Debug("[bluetooth] pairing listener started, waiting for signals")
	listener.Listen()
	logger.Info("[bluetooth] pairing listener stopped")
}

func (b *BluetoothBackend) onDevicePaired(sig *dbus.Signal) bool {
	logger.Debug("[bluetooth] pairing signal received from %s (body length=%d)", sig.Path, len(sig.Body))

	if len(sig.Body) < 2 {
		logger.Debug("[bluetooth] pairing signal from %s ignored: body too short", sig.Path)
		return false
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		logger.Debug("[bluetooth] pairing signal from %s ignored: body[1] is not map[string]Variant", sig.Path)
		return false
	}

	logger.Debug("[bluetooth] pairing signal from %s: changed properties=%v", sig.Path, changedKeys(changed))

	pairedVar, ok := changed["Paired"]
	if !ok {
		logger.Debug("[bluetooth] pairing signal from %s ignored: no Paired property in changed set", sig.Path)
		return false
	}

	paired, ok := pairedVar.Value().(bool)
	if !ok {
		logger.Debug("[bluetooth] pairing signal from %s ignored: Paired value is not bool (got %T)", sig.Path, pairedVar.Value())
		return false
	}
	if !paired {
		logger.Debug("[bluetooth] pairing signal from %s ignored: Paired=false", sig.Path)
		return false
	}

	logger.Info("[bluetooth] device %s paired successfully", sig.Path)

	if !b.trustDevice(sig.Path) {
		logger.Warn("[bluetooth] failed to trust device %s", sig.Path)
		return false
	}

	logger.Info("[bluetooth] device %s trusted", sig.Path)
	b.refreshKnownDevices()
	return true
}

func (b *BluetoothBackend) onDeviceConnectionChange(sig *dbus.Signal) bool {
	logger.Debug("[bluetooth] connection signal received from %s (body length=%d)", sig.Path, len(sig.Body))

	if len(sig.Body) < 2 {
		logger.Debug("[bluetooth] connection signal from %s ignored: body too short", sig.Path)
		return false
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		logger.Debug("[bluetooth] connection signal from %s ignored: body[1] is not map[string]Variant", sig.Path)
		return false
	}

	logger.Debug("[bluetooth] connection signal from %s: changed properties=%v", sig.Path, changedKeys(changed))

	connectedVar, ok := changed["Connected"]
	if !ok {
		logger.Debug("[bluetooth] connection signal from %s ignored: no Connected property in changed set", sig.Path)
		return false
	}

	connected, ok := connectedVar.Value().(bool)
	if !ok {
		logger.Debug("[bluetooth] connection signal from %s ignored: Connected value is not bool (got %T)", sig.Path, connectedVar.Value())
		return false
	}

	logger.Debug("[bluetooth] device %s Connected=%v", sig.Path, connected)

	if connected {
		b.cancelIdleTimer()
	} else {
		b.checkAndStartIdleTimer()
	}

	return false
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
