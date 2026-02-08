package bluetooth

import (
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// callWithTimeout executes a D-Bus call with timeout
func callWithTimeout(call *dbus.Call, timeout time.Duration) error {
	done := make(chan error, 1)

	go func() {
		done <- call.Err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		return &dbusTimeoutError{}
	}
}

// callWithTimeout receiver method for MPRISBackend
func (b *BluetoothBackend) callWithTimeout(call *dbus.Call) error {
	return callWithTimeout(call, b.timeout)
}

// callMethod calls a method on an object with timeout
func (b *BluetoothBackend) callMethod(obj dbus.BusObject, method string, args ...interface{}) error {
	return b.callWithTimeout(obj.Call(method, 0, args...))
}

// setProperty générique pour n’importe quel objet/interface
func (b *BluetoothBackend) setProperty(obj dbus.BusObject, iface, prop string, value interface{}) error {
	call := obj.Call(DBUS_PROP_SET, 0, iface, prop, dbus.MakeVariant(value))
	return b.callWithTimeout(call)
}

// getProperty retrieves a property from D-Bus for a given busName
func (b *BluetoothBackend) getProperty(obj dbus.BusObject, iface, prop string) (dbus.Variant, error) {
	var v dbus.Variant
	call := obj.Call(DBUS_PROP_GET, 0, iface, prop)
	if err := b.callWithTimeout(call); err != nil {
		return dbus.Variant{}, err
	}
	if err := call.Store(&v); err != nil {
		return dbus.Variant{}, err
	}
	return v, nil
}

func (b *BluetoothBackend) getAdapterProp(prop BluetoothState) (dbus.Variant, error) {
	return b.getProperty(b.adapter(), BLUETOOTH_ADAPTER, prop.toString())
}

func (b *BluetoothBackend) adapter() dbus.BusObject {
	return b.conn.Object(BLUETOOTH_PREFIX, dbus.ObjectPath(BLUETOOTH_PATH))
}

func (b *BluetoothBackend) setAdapterProp(prop string, value interface{}) error {
	call := b.adapter().Call(
		DBUS_PROP_SET,
		0,
		BLUETOOTH_ADAPTER,
		prop,
		dbus.MakeVariant(value),
	)
	return b.callWithTimeout(call)
}

func extractBool(v dbus.Variant) (bool, bool) {
	val, ok := v.Value().(bool)
	return val, ok
}

func (b *BluetoothBackend) exportAgent(agent *bluezAgent) error {
	return b.conn.Export(
		agent,
		dbus.ObjectPath(AGENT_PATH),
		AGENT_IFACE,
	)
}

func (b *BluetoothBackend) getObj(prefix, path string) dbus.BusObject {
	return b.conn.Object(prefix, dbus.ObjectPath(path))
}

// Retourne la liste des devices connus par l'adapter
func (b *BluetoothBackend) listDevices() ([]dbus.ObjectPath, error) {
	obj := b.getObj(BLUETOOTH_PREFIX, BLUETOOTH_PATH)
	props, err := b.getProperty(obj, BLUETOOTH_ADAPTER, "Devices")
	if err != nil {
		logger.Warn("[bluetooth] failed to query adapter %s properties: %v", BLUETOOTH_ADAPTER, err)
		return nil, err
	}
	devices, ok := props.Value().([]dbus.ObjectPath)
	if !ok {
		logger.Warn("[bluetooth] failed list bluetooth devices: %v", err)
		return nil, &dbusParseError{}
	}
	return devices, nil
}

func (b *BluetoothBackend) isAdapterOn() bool {
	v, err := b.getAdapterProp(BT_STATE_POWERED)
	if err != nil {
		logger.Warn("[bluetooth] failed to get adapter power state: %v", err)
		return false
	}
	powered, _ := extractBool(v)
	return powered
}

// Vérifie si un device est Trusted
func (b *BluetoothBackend) isDeviceTrusted(path dbus.ObjectPath) (bool, bool) {
	obj := b.getObj(BLUETOOTH_PREFIX, string(path))
	v, err := b.getProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.toString())
	if err != nil {
		logger.Warn("[bluetooth] failed to get %s trust state: %v", path, err)
		return false, false
	}
	return extractBool(v)
}

// Définit Trusted = true pour un device
func (b *BluetoothBackend) trustDevice(path dbus.ObjectPath) bool {
	obj := b.getObj(BLUETOOTH_PREFIX, string(path))
	if err := b.setProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.toString(), true); err != nil {
		logger.Warn("[bluetooth] failed to trust devices: %v", err)
		return false
	}
	return true
}

func (b *BluetoothBackend) SetPairable(state bool) error {
	if err := b.setAdapterProp(BT_STATE_PAIRABLE.toString(), state); err != nil {
		logger.Warn("[bluetooth] failed to set adapter pairable %v: %v", state, err)
		return err
	}
	return nil
}

func (b *BluetoothBackend) SetDiscoverable(state bool) error {
	if err := b.setAdapterProp(BT_STATE_DISCOVERABLE.toString(), state); err != nil {
		logger.Warn("[bluetooth] failed to set adapter discoverable %v: %v", state, err)
		return err
	}
	return nil
}

func (b *BluetoothBackend) PowerOnAdapter(state bool) error {
	if err := b.setAdapterProp(BT_STATE_POWERED.toString(), state); err != nil {
		logger.Warn("[bluetooth] failed to set adapter discoverable %v: %v", state, err)
		return err
	}
	return nil
}

func (b *BluetoothBackend) SetTimeOut(prop string) error {
	if err := b.setAdapterProp(prop, uint32(b.pairingTimeout.Seconds())); err != nil {
		logger.Warn("[bluetooth] failed to set adapter %s timeout: %v", prop, err)
		return err
	}
	return nil
}

func (b *BluetoothBackend) RequestNoInputOutputAgent(manager dbus.BusObject) error {
	if err := b.callMethod(
		manager,
		REGISTER_AGENT,
		dbus.ObjectPath(AGENT_PATH),
		AGENT_CAPABILITY,
	); err != nil {
		logger.Warn("[bluetooth] failed to set agent capability %s timeout: %v", AGENT_CAPABILITY, err)
		return err
	}

	if err := b.callMethod(
		manager,
		REQUEST_AGENT,
		dbus.ObjectPath(AGENT_PATH),
	); err != nil {
		logger.Warn("[bluetooth] failed to request default agent: %v", err)
		return err
	}
	logger.Debug("[bluetooth] Agent configuration success")
	return nil
}

func (b *BluetoothBackend) unregisterAgent() {
	if b.agent == nil {
		return
	}

	manager := b.getObj(BLUETOOTH_PREFIX, BLUEZ_PATH)
	if err := b.callMethod(
		manager,
		UNREGISTER_AGENT,
		dbus.ObjectPath(AGENT_PATH),
	); err != nil {
		logger.Warn("[bluetooth] failed to unregister agent %s: %v", err)
	}

	// côté Go, on oublie juste la ref
	b.agent = nil
}
