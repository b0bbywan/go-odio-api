package bluetooth

import (
	"context"

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
	// Bluetooth ON
	if err := b.setAdapterProp(BT_STATE_POWERED.toString(), true); err != nil {
		return err
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
	agentObj := &agent{}

	// Export de l'objet sur la connexion system bus
	if err := b.conn.Export(
		agentObj,
		dbus.ObjectPath(AGENT_PATH),
		AGENT_IFACE,
	); err != nil {
		return err
	}

	manager := b.conn.Object(BLUETOOTH_PREFIX, dbus.ObjectPath(BLUEZ_PATH))

	// RegisterAgent
	call := manager.Call(
		REGISTER_AGENT,
		0,
		dbus.ObjectPath(AGENT_PATH),
		AGENT_CAPABILITY,
	)
	if err := b.callWithTimeout(call); err != nil {
		return err
	}

	// RequestDefaultAgent
	call = manager.Call(
		REQUEST_AGENT,
		0,
		dbus.ObjectPath(AGENT_PATH),
	)
	if err := b.callWithTimeout(call); err != nil {
		return err
	}

	return nil
}
