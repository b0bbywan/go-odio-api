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
	powered, err := b.isAdapterOn()
	if err != nil {
		return err
	}
	if powered {
		return nil
	}

	if err := b.setAdapterProp(BT_STATE_POWERED.toString(), true); err != nil {
		return err
	}

	if err := b.setAdapterProp(BT_STATE_DISCOVERABLE.toString(), false); err != nil {
		return err
	}

	if err := b.setAdapterProp(BT_STATE_PAIRABLE.toString(), false); err != nil {
		return err
	}

	return nil
}

func (b *BluetoothBackend) NewPairing() error {
	// RegisterAgent
	if err := b.registerAgent(); err != nil {
		if dbusErr, ok := err.(*dbus.Error); ok {
			if dbusErr.Name == "org.bluez.Error.AlreadyExists" {
				return nil
			}
		}
	}

	// Bluetooth ON
	var powered bool
	var err error
	if powered, err = b.isAdapterOn(); err != nil {
		return err
	}
	if !powered {
		if err := b.setAdapterProp(BT_STATE_POWERED.toString(), true); err != nil {
			return err
		}
	}

	// Timeouts (en secondes)
	if err := b.setAdapterProp(DISCOVERABLE_TIMEOUT, uint32(b.pairingTimeout.Seconds())); err != nil {
		return err
	}

	if err := b.setAdapterProp(PAIRABLE_TIMEOUT, uint32(b.pairingTimeout.Seconds())); err != nil {
		return err
	}

	// Mode pairing
	if err := b.setAdapterProp(BT_STATE_DISCOVERABLE.toString(), true); err != nil {
		return err
	}

	if err := b.setAdapterProp(BT_STATE_PAIRABLE.toString(), true); err != nil {
		return err
	}

 	// 5️⃣ Attendre qu'un device apparaisse et le marquer Trusted
    //    Ici on fait un simple polling toutes les 500ms pour exemple
    timeout := time.After(b.pairingTimeout)
    ticker := time.NewTicker(500 * time.Millisecond)
    defer ticker.Stop()

    for {
        select {
        case <-timeout:
            return nil // Timeout atteint, pairing terminé
        case <-ticker.C:
            devices, _ := b.listDevices()
            for _, d := range devices {
                // Si device n'est pas encore Trusted, on le trust
                trusted, _ := b.isDeviceTrusted(d)
                if !trusted {
                    _ = b.trustDevice(d)
                }
            }
        }
    }


	return nil
}

func (b *BluetoothBackend) Close() {
	if b.conn != nil {
		if err := b.conn.Close(); err != nil {
			logger.Info("Failed to close D-Bus connection: %v", err)
		}
		b.conn = nil
	}
}

func (b *BluetoothBackend) registerAgent() error {
	// Export de l'objet sur la connexion system bus
	agent := bluezAgent{}
	if err := b.exportAgent(&agent); err != nil {
		return err
	}

	manager := b.getObj(BLUETOOTH_PREFIX, BLUEZ_PATH)
	if err := b.callMethod(
		manager,
		REGISTER_AGENT,
		dbus.ObjectPath(AGENT_PATH),
		AGENT_CAPABILITY,
	); err != nil {
		return err
	}

	if err := b.callMethod(
		manager,
		REQUEST_AGENT,
		dbus.ObjectPath(AGENT_PATH),
	); err != nil {
		return err
	}

	return nil
}
