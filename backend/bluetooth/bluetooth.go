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
		statusCache:    cache.New[BluetoothStatus](0), // no expiration
	}

	if err = backend.CheckBluetoothSupport(); err != nil {
		logger.Error("[bluetooth] Not Supported")
		backend.Close()
		return nil, nil
	}

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

	logger.Info("[bluetooth] Bluetooth ready to connect to already known devices")
	return nil
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
		return &PairingInProgressError{}
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

	go b.waitPairing(b.ctx)
	logger.Info("[bluetooth] Bluetooth pairing mode enabled")

	return nil
}

func (b *BluetoothBackend) waitPairing(ctx context.Context) {
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
	}()

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-subCtx.Done():
			logger.Info("[bluetooth] pairing stopped")
			return
		case <-ticker.C:
			devices, err := b.listDevices()
			if err != nil {
				logger.Warn("[bluetooth] failed to list devices: %v", err)
			}
			for _, d := range devices {
				trusted, ok := b.isDeviceTrusted(d)
				if !ok {
					continue
				}
				if !trusted {
					if ok := b.trustDevice(d); ok {
						logger.Info("[bluetooth] New device %v trusted", d)
						b.refreshKnownDevices()
						return
					}
				}
			}
		}
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

	agent := bluezAgent{}
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
