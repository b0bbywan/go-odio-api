package bluetooth

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New creates a new MPRIS backend
func New(ctx context.Context, cfg *config.BluetoothConfig) (*BluetoothBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	conn, err := dbus.ConnectSystemBus()
	if err != nil {
		return nil, err
	}

	return &BluetoothBackend{
		conn:           conn,
		ctx:            ctx,
		timeout:        cfg.Timeout,
		pairingTimeout: cfg.PairingTimeout,
	}, nil
}

func (b *BluetoothBackend) Start() error {
	// Connexion Adapter
	return nil
}

func (b *BluetoothBackend) PowerOn() error {
	if powered := b.isAdapterOn(); powered {
		return nil
	}

	if err := b.PowerOnAdapter(true); err != nil {
		return err
	}

	if err := b.SetDiscoverable(false); err != nil {
		return err
	}

	if err := b.SetPairable(false); err != nil {
		return err
	}

	return nil
}

func (b *BluetoothBackend) PowerDown() error {
	if powered := b.isAdapterOn(); !powered {
		return nil
	}

	if err := b.PowerOnAdapter(false); err != nil {
		return err
	}
	return nil
}

func (b *BluetoothBackend) NewPairing() error {
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

	// Timeouts (en secondes)
	if err := b.SetTimeOut(DISCOVERABLE_TIMEOUT); err != nil {
		return err
	}

	if err := b.SetTimeOut(PAIRABLE_TIMEOUT); err != nil {
		return err
	}

	// pairing mode
	if err := b.SetDiscoverable(true); err != nil {
		return err
	}

	if err := b.SetPairable(true); err != nil {
		return err
	}

	go b.waitPairing(b.ctx)

	return nil
}

func (b *BluetoothBackend) waitPairing(ctx context.Context) {
	subCtx, cancel := context.WithTimeout(ctx, b.pairingTimeout)
	defer cancel()

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
						return
					}
				}
			}
		}
	}

}

func (b *BluetoothBackend) Close() {
	b.unregisterAgent()

	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			logger.Info("[bluetooth] Failed to close D-Bus connection: %v", err)
		}
		b.conn = nil
	}
}

func (b *BluetoothBackend) registerAgent() error {
	if b.agent != nil {
		return nil
	}

	// Export de l'objet sur la connexion system bus
	agent := bluezAgent{}
	if err := b.exportAgent(&agent); err != nil {
		return err
	}

	manager := b.getObj(BLUETOOTH_PREFIX, BLUEZ_PATH)
	if err := b.RequestNoInputOutputAgent(manager); err != nil {
		return err
	}

	b.agent = &agent

	return nil
}
