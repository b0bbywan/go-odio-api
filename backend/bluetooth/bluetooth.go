package bluetooth

import (
	"context"
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
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
		scanTimeout:    cfg.ScanTimeout,
		powerOnStart:   cfg.PowerOnStart,
		statusCache:    cache.New[BluetoothStatus](0), // no expiration
		events:         make(chan events.Event, 16),
	}

	if err = backend.CheckBluetoothSupport(); err != nil {
		logger.Error("[bluetooth] Not Supported")
		backend.Close()
		return nil, nil
	}

	backend.syncAdapterState()
	return &backend, nil
}

func (b *BluetoothBackend) Start() error {
	if !b.powerOnStart {
		return nil
	}
	return b.PowerUp()
}

func (b *BluetoothBackend) syncAdapterState() {
	powered := b.isAdapterOn()
	pairable := b.isPairable()
	discoverable := b.isDiscoverable()

	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = powered
		s.Pairable = pairable
		s.Discoverable = discoverable
	})
	logger.Info("[bluetooth] backend started (powered=%v pairable=%v discoverable=%v)", powered, pairable, discoverable)

	if powered {
		b.refreshDevices()
		b.startListener()
	}
}

func (b *BluetoothBackend) PowerUp() error {
	if powered := b.isAdapterOn(); powered {
		return nil
	}

	unblockIfSoftBlocked()

	if err := b.PowerOnAdapter(true); err != nil {
		return err
	}

	if err := b.SetDiscoverableAndPairable(false); err != nil {
		return err
	}

	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = true
	})
	b.refreshDevices()
	b.startListener()

	logger.Info("[bluetooth] Bluetooth ready to connect to already known devices")
	return nil
}

func (b *BluetoothBackend) startListener() {
	if b.listener != nil {
		logger.Debug("[bluetooth] listener already running, skipping")
		return
	}
	matchRules := []string{
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Device1'",
		"type='signal',interface='org.freedesktop.DBus.Properties',member='PropertiesChanged',arg0='org.bluez.Adapter1'",
	}
	logger.Debug("[bluetooth] starting listener (device + adapter)")
	listener := NewDBusListener(b.conn, b.ctx, matchRules, b.onPropertiesChanged)
	if err := listener.Start(); err != nil {
		listener.Stop()
		logger.Warn("[bluetooth] failed to start listener: %v", err)
		return
	}
	b.listener = listener
	go listener.Listen()
	logger.Debug("[bluetooth] listener started")
	b.checkAndStartIdleTimer()
}

func (b *BluetoothBackend) stopListener() {
	if b.listener != nil {
		b.listener.Stop()
		b.listener = nil
		logger.Debug("[bluetooth] listener stopped")
	}
	b.cancelIdleTimer()
}

func (b *BluetoothBackend) PowerDown() error {
	if powered := b.isAdapterOn(); !powered {
		return nil
	}

	if err := b.PowerOnAdapter(false); err != nil {
		return err
	}

	b.cleanupPoweredState()
	logger.Info("[bluetooth] Powered down")
	return nil
}

func (b *BluetoothBackend) cleanupPoweredState() {
	// Tear down the scan resources directly: powering off makes BlueZ stop
	// discovery on its own, so the user-facing StopScan (BlueZ call, refresh,
	// idle re-arm) doesn't belong here.
	b.scanMu.Lock()
	b.stopDiscoveryListener()
	b.scanMu.Unlock()

	b.stopListener()
	b.updateStatus(func(s *BluetoothStatus) {
		s.Powered = false
		s.Discoverable = false
		s.Pairable = false
		s.PairingActive = false
		s.PairingUntil = nil
		s.Scanning = false
		// Drop the device list: nothing is reachable while powered off, and the
		// disconnect events that would clear it may arrive after the listener is
		// gone. refreshDevices repopulates it on power-up.
		s.KnownDevices = nil
	})
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
		unblockIfSoftBlocked()
		if err := b.PowerOnAdapter(true); err != nil {
			return err
		}
		b.startListener()
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

// isDeviceBonded reports whether we still hold the device's pairing key, read
// from the cached device list — the same Bonded flag the UI shows, so the
// connect decision matches what the user saw. A bonded device reconnects
// without the adapter being pairable; only a new bond needs the pairable window.
func (b *BluetoothBackend) isDeviceBonded(address string) bool {
	for _, d := range b.GetStatus().KnownDevices {
		if d.Address == address {
			return d.Bonded
		}
	}
	return false
}

// Connect opens an outbound connection to a device by address and blocks until
// BlueZ answers, so the caller gets the real outcome. BlueZ may take a few
// seconds and trigger pairing; the device's Connected state also propagates
// through bluetooth.updated events from the listener.
func (b *BluetoothBackend) Connect(address string) error {
	if err := validateAddress(address); err != nil {
		return err
	}
	path := devicePath(address)
	logger.Info("[bluetooth] connecting to %s", address)
	// An unbonded target needs a fresh bond, so open the pairable window for the
	// connect only. A bonded device already has its key and reconnects without
	// the adapter ever being pairable. If pairing mode is already active the
	// adapter is pairable and the pairing-mode machinery owns that flag — leave
	// it alone, or disabling it here would cancel the user's pairing session.
	if !b.isDeviceBonded(address) && !b.GetStatus().PairingActive {
		if err := b.SetPairable(true); err != nil {
			logger.Warn("[bluetooth] failed to enable pairable before connect: %v", err)
		}
		defer func() {
			// Pairing mode may have started during the (seconds-long) connect; if
			// so it now owns Pairable, so don't pull it out from under it.
			if b.GetStatus().PairingActive {
				return
			}
			if err := b.SetPairable(false); err != nil {
				logger.Warn("[bluetooth] failed to disable pairable after connect: %v", err)
			}
		}()
	}
	if err := b.connectDevice(path); err != nil {
		logger.Warn("[bluetooth] failed to connect to %s: %v", address, err)
		return fmt.Errorf("could not connect to %s: %w", address, err)
	}
	if !b.trustDevice(path) {
		logger.Warn("[bluetooth] connected to %s but failed to trust it", address)
	}
	logger.Info("[bluetooth] connected to %s", address)
	// We have a target now; stop any active scan to free the adapter.
	if err := b.StopScan(); err != nil {
		logger.Warn("[bluetooth] failed to stop scan after connect: %v", err)
	}
	return nil
}

// Disconnect tears down the connection to a device by address.
func (b *BluetoothBackend) Disconnect(address string) error {
	if err := validateAddress(address); err != nil {
		return err
	}
	if err := b.disconnectDevice(devicePath(address)); err != nil {
		logger.Warn("[bluetooth] failed to disconnect %s: %v", address, err)
		return fmt.Errorf("could not disconnect from %s: %w", address, err)
	}
	return nil
}

func (b *BluetoothBackend) onPropertiesChanged(sig *dbus.Signal) bool {
	if sig == nil {
		return true // channel closed
	}
	changed, iface, err := filterSignal(sig)
	if err != nil {
		logger.Debug("[bluetooth] signal filtered out: %v", err)
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
	// Ignore the RSSI/TxPower churn a scan generates.
	if !changedAny(changed, BT_STATE_CONNECTED, BT_STATE_PAIRED) {
		return
	}

	refresh := false
	defer func() {
		if refresh {
			b.refreshDevices()
		}
	}()

	if connected, ok := extractMapBool(changed, BT_STATE_CONNECTED); ok {
		logger.Info("[bluetooth] device %s Connected=%v", path, connected)
		if connected {
			b.cancelIdleTimer()
			refresh = true
			return
		}
		b.checkAndStartIdleTimer()
		refresh = true
		return
	}

	if paired, ok := extractMapBool(changed, BT_STATE_PAIRED); ok && paired {
		logger.Info("[bluetooth] device %s paired successfully", path)
		if ok = b.trustDevice(path); !ok {
			logger.Warn("[bluetooth] failed to trust device %s", path)
			return
		}

		logger.Info("[bluetooth] device %s trusted", path)
		refresh = true
		if err := b.SetDiscoverableAndPairable(false); err != nil {
			logger.Warn("[bluetooth] failed to stop pairing mode: %v", err)
		}
	}
}

func (b *BluetoothBackend) onAdapterPropertiesChanged(changed map[string]dbus.Variant) {
	if discoverable, ok := extractMapBool(changed, BT_STATE_DISCOVERABLE); ok {
		logger.Debug("[bluetooth] adapter Discoverable=%v", discoverable)
		b.updateStatus(func(s *BluetoothStatus) {
			s.Discoverable = discoverable
		})
	}

	if pairable, ok := extractMapBool(changed, BT_STATE_PAIRABLE); ok {
		logger.Debug("[bluetooth] adapter Pairable=%v", pairable)
		b.updateStatus(func(s *BluetoothStatus) {
			s.Pairable = pairable
			if !pairable {
				logger.Info("[bluetooth] pairing mode ended")
				s.PairingActive = false
				s.PairingUntil = nil
			}
		})
	}
}

func (b *BluetoothBackend) cancelIdleTimer() {
	if b.idleTimer.Cancel() {
		logger.Info("[bluetooth] idle timer cancelled")
	}
}

func (b *BluetoothBackend) checkAndStartIdleTimer() {
	if b.hasConnectedDevices() {
		logger.Debug("[bluetooth] still has connected devices, skipping idle timer")
		return
	}

	armed := b.idleTimer.Start(b.idleTimeout, func() {
		logger.Info("[bluetooth] idle timeout reached after %v, powering down", b.idleTimeout)
		if err := b.PowerOnAdapter(false); err != nil {
			logger.Warn("[bluetooth] failed to power down: %v", err)
		}
		b.cleanupPoweredState()
	})
	if armed {
		logger.Info("[bluetooth] idle timer started (%v)", b.idleTimeout)
	}
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
	b.notify(status)
}

func (b *BluetoothBackend) notify(status BluetoothStatus) {
	select {
	case b.events <- events.Event{Type: events.TypeBluetoothUpdated, Data: status}:
	default:
		logger.Warn("[bluetooth] event channel full, dropping %s event", events.TypeBluetoothUpdated)
	}
}

func (b *BluetoothBackend) Events() <-chan events.Event {
	return b.events
}

// refreshDevices rebuilds the device list from BlueZ, the source of truth.
func (b *BluetoothBackend) refreshDevices() {
	if b.conn == nil {
		return
	}
	devices, err := b.listDevices()
	if err != nil {
		logger.Warn("[bluetooth] failed to list devices: %v", err)
		return
	}
	b.updateStatus(func(s *BluetoothStatus) {
		s.KnownDevices = devices
	})
}

// GetDevices returns the current device list.
func (b *BluetoothBackend) GetDevices() []BluetoothDevice {
	devices := b.GetStatus().KnownDevices
	if devices == nil {
		return []BluetoothDevice{}
	}
	return devices
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
