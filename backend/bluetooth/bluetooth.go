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

	b.startMainListener()

	logger.Info("[bluetooth] Bluetooth ready to connect to already known devices")
	return nil
}

func (b *BluetoothBackend) startMainListener() {
	if b.listenerCancel != nil {
		logger.Debug("[bluetooth] main listener already running")
		return
	}
	matchRule := "type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Device1'"
	logger.Debug("[bluetooth] starting main listener (matchRule=%s)", matchRule)
	ctx, cancel := context.WithCancel(b.ctx)
	listener := NewDBusListener(b.conn, ctx, matchRule, b.onBluetoothSignal)
	if err := listener.Start(); err != nil {
		cancel()
		logger.Warn("[bluetooth] failed to start main listener: %v", err)
		return
	}
	b.listenerCancel = cancel
	go func() {
		listener.Listen()
		listener.Stop()
		logger.Debug("[bluetooth] main listener stopped")
	}()
	logger.Debug("[bluetooth] main listener started")
}

func (b *BluetoothBackend) stopMainListener() {
	if b.listenerCancel != nil {
		b.listenerCancel()
		b.listenerCancel = nil
	}
}

func (b *BluetoothBackend) PowerDown() error {
	if powered := b.isAdapterOn(); !powered {
		return nil
	}

	b.stopMainListener()
	b.cancelIdleTimer()
	b.stopPairingMode()

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
	b.pairingMu.Lock()
	if b.isPairing {
		b.pairingMu.Unlock()
		logger.Info("[bluetooth] pairing already in progress")
		return nil
	}
	b.isPairing = true
	b.pairingMu.Unlock()

	// RegisterAgent
	if err := b.registerAgent(); err != nil {
		if dbusErr, ok := err.(*dbus.Error); ok && dbusErr.Name == "org.bluez.Error.AlreadyExists" {
			logger.Info("[bluetooth] agent already registered")
		} else {
			logger.Warn("[bluetooth] failed to register agent: %v", err)
			b.pairingMu.Lock()
			b.isPairing = false
			b.pairingMu.Unlock()
			return err
		}
	}

	// Bluetooth ON (also starts the main listener if not already running)
	if err := b.PowerUp(); err != nil {
		b.pairingMu.Lock()
		b.isPairing = false
		b.pairingMu.Unlock()
		return err
	}

	// Timeouts (in seconds)
	if err := b.SetTimeOut(DISCOVERABLE_TIMEOUT); err != nil {
		b.pairingMu.Lock()
		b.isPairing = false
		b.pairingMu.Unlock()
		return err
	}

	if err := b.SetTimeOut(PAIRABLE_TIMEOUT); err != nil {
		b.pairingMu.Lock()
		b.isPairing = false
		b.pairingMu.Unlock()
		return err
	}

	// pairing mode
	if err := b.SetDiscoverableAndPairable(true); err != nil {
		b.pairingMu.Lock()
		b.isPairing = false
		b.pairingMu.Unlock()
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

	// Start pairing timeout timer
	b.pairingMu.Lock()
	b.pairingTimer = time.AfterFunc(b.pairingTimeout, b.stopPairingMode)
	b.pairingMu.Unlock()

	logger.Info("[bluetooth] Bluetooth pairing mode enabled (timeout=%v)", b.pairingTimeout)
	return nil
}

func (b *BluetoothBackend) stopPairingMode() {
	b.pairingMu.Lock()
	defer b.pairingMu.Unlock()

	if !b.isPairing {
		return
	}

	if b.pairingTimer != nil {
		b.pairingTimer.Stop()
		b.pairingTimer = nil
	}
	b.isPairing = false

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
	logger.Info("[bluetooth] pairing mode stopped")
}

func (b *BluetoothBackend) onBluetoothSignal(sig *dbus.Signal) bool {
	if sig == nil || len(sig.Body) < 2 {
		return false
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return false
	}

	logger.Debug("[bluetooth] signal from %s: changed=%v", sig.Path, changedKeys(changed))

	// Handle pairing
	if pairedVar, ok := changed["Paired"]; ok {
		if paired, ok := pairedVar.Value().(bool); ok && paired {
			b.pairingMu.Lock()
			isPairing := b.isPairing
			b.pairingMu.Unlock()

			if isPairing {
				logger.Info("[bluetooth] device %s paired successfully", sig.Path)
				if b.trustDevice(sig.Path) {
					logger.Info("[bluetooth] device %s trusted", sig.Path)
					b.refreshKnownDevices()
					b.stopPairingMode()
				} else {
					logger.Warn("[bluetooth] failed to trust device %s", sig.Path)
				}
			}
		}
	}

	// Handle connection change (for idle timer / auto power-off)
	if connectedVar, ok := changed["Connected"]; ok {
		if connected, ok := connectedVar.Value().(bool); ok {
			logger.Debug("[bluetooth] device %s Connected=%v", sig.Path, connected)
			if connected {
				b.cancelIdleTimer()
			} else {
				b.checkAndStartIdleTimer()
			}
		}
	}

	return false // never stop the listener
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
	if b.idleTimeout == 0 {
		logger.Debug("[bluetooth] idle timeout disabled, skipping idle timer")
		return
	}

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
