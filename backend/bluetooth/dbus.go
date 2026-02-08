package bluetooth

import (
	"time"

	"github.com/godbus/dbus/v5"
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

func (b *BluetoothBackend) isAdapterOn() (bool, error) {
	v, err := b.getAdapterProp(BT_STATE_POWERED)
	if err != nil {
		return false, err
	}
	powered, ok := extractBool(v)
	if !ok {
		return false, &dbusParseError{}
	}
	return powered, nil
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
        return nil, err
    }
    devices, ok := props.Value().([]dbus.ObjectPath)
    if !ok {
        return nil, &dbusParseError{}
    }
    return devices, nil
}

// Vérifie si un device est Trusted
func (b *BluetoothBackend) isDeviceTrusted(path dbus.ObjectPath) (bool, bool) {
    obj := b.getObj(BLUETOOTH_PREFIX, string(path))
    v, err := b.getProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.toString())
    if err != nil {
        return false, false
    }
    t, ok := extractBool(v)
    return t, ok
}

// Définit Trusted = true pour un device
func (b *BluetoothBackend) trustDevice(path dbus.ObjectPath) error {
    obj := b.getObj(BLUETOOTH_PREFIX, string(path))
    return b.setProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.toString(), true)
}
