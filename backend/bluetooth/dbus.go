package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// callWithContext executes a D-Bus method call with a standalone timeout.
// Uses context.Background() so that calls still work during shutdown cleanup.
// The timeout is the only safeguard needed against hanging calls.
func (b *BluetoothBackend) callWithContext(obj dbus.BusObject, method string, args ...interface{}) *dbus.Call {
	ctx, cancel := context.WithTimeout(context.Background(), b.timeout)
	defer cancel()
	return obj.CallWithContext(ctx, method, 0, args...)
}

// callMethod calls a D-Bus method with timeout and returns the error
func (b *BluetoothBackend) callMethod(obj dbus.BusObject, method string, args ...interface{}) error {
	return b.callWithContext(obj, method, args...).Err
}

func (b *BluetoothBackend) setProperty(obj dbus.BusObject, iface, prop string, value interface{}) error {
	return b.callMethod(obj, DBUS_PROP_SET, iface, prop, dbus.MakeVariant(value))
}

// getProperty retrieves a property from D-Bus for a given busName
func (b *BluetoothBackend) getProperty(obj dbus.BusObject, iface, prop string) (dbus.Variant, error) {
	var v dbus.Variant
	call := b.callWithContext(obj, DBUS_PROP_GET, iface, prop)
	if call.Err != nil {
		return dbus.Variant{}, call.Err
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
	return b.callMethod(b.adapter(), DBUS_PROP_SET, BLUETOOTH_ADAPTER, prop, dbus.MakeVariant(value))
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

// iterateAdapterDevices iterates over all devices belonging to our adapter
// and calls the provided function for each device
func (b *BluetoothBackend) iterateAdapterDevices(fn func(path dbus.ObjectPath, props map[string]dbus.Variant) bool) error {
	managedObjects, err := b.getManagedObjects()
	if err != nil {
		return err
	}

	for path, ifaces := range managedObjects {
		dev, ok := ifaces[BLUETOOTH_DEVICE]
		if !ok {
			continue
		}

		// Filter by adapter
		if adapterPathVar, ok := dev[BT_PROP_ADAPTER]; ok {
			adapterPath, ok := adapterPathVar.Value().(dbus.ObjectPath)
			if !ok {
				logger.Warn("[bluetooth] invalid adapter path type for device %v", path)
				continue
			}
			if string(adapterPath) != BLUETOOTH_PATH {
				continue
			}
		}

		// Call the callback, stop if it returns false
		if !fn(path, dev) {
			break
		}
	}

	return nil
}

func (b *BluetoothBackend) listKnownDevices() ([]BluetoothDevice, error) {
	devices := []BluetoothDevice{}

	err := b.iterateAdapterDevices(func(path dbus.ObjectPath, props map[string]dbus.Variant) bool {
		// Only keep trusted devices
		trustedVar, ok := props[BT_PROP_TRUSTED]
		if !ok {
			return true
		}
		trusted, ok := trustedVar.Value().(bool)
		if !ok || !trusted {
			return true
		}

		// Extract device info
		device := BluetoothDevice{
			Address:   extractString(props, BT_PROP_ADDRESS),
			Name:      extractString(props, BT_PROP_NAME),
			Trusted:   trusted,
			Connected: extractBoolProp(props, BT_PROP_CONNECTED),
		}

		devices = append(devices, device)
		return true
	})

	return devices, err
}

func extractString(props map[string]dbus.Variant, key string) string {
	if v, ok := props[key]; ok {
		if s, ok := v.Value().(string); ok {
			return s
		}
	}
	return ""
}

func extractBoolProp(props map[string]dbus.Variant, key string) bool {
	if v, ok := props[key]; ok {
		if b, ok := v.Value().(bool); ok {
			return b
		}
	}
	return false
}

func (b *BluetoothBackend) hasConnectedDevices() bool {
	connected := false
	err := b.iterateAdapterDevices(func(path dbus.ObjectPath, props map[string]dbus.Variant) bool {
		if extractBoolProp(props, BT_PROP_CONNECTED) {
			connected = true
			return false // stop iterating
		}
		return true
	})
	if err != nil {
		logger.Warn("[bluetooth] failed to check connected devices: %v", err)
	}
	return connected
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

func (b *BluetoothBackend) isDeviceTrusted(path dbus.ObjectPath) (bool, bool) {
	obj := b.getObj(BLUETOOTH_PREFIX, string(path))
	v, err := b.getProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.toString())
	if err != nil {
		logger.Warn("[bluetooth] failed to get %s trust state: %v", path, err)
		return false, false
	}
	return extractBool(v)
}

func (b *BluetoothBackend) pairDevice(path dbus.ObjectPath) error {
	logger.Debug("[bluetooth] attempting to pair device %v", path)
	obj := b.getObj(BLUETOOTH_PREFIX, string(path))
	if err := b.callMethod(obj, DEVICE_PAIR_METHOD); err != nil {
		logger.Warn("[bluetooth] failed to pair device %v: %v", path, err)
		return err
	}
	logger.Debug("[bluetooth] device %v paired successfully", path)
	return nil
}

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

func (b *BluetoothBackend) SetDiscoverableAndPairable(state bool) error {
	if err := b.SetDiscoverable(state); err != nil {
		return err
	}

	if err := b.SetPairable(state); err != nil {
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

func (b *BluetoothBackend) getManagedObjects() (map[dbus.ObjectPath]map[string]map[string]dbus.Variant, error) {
	objManager := b.getObj(BLUETOOTH_PREFIX, "/")
	var managedObjects map[dbus.ObjectPath]map[string]map[string]dbus.Variant
	call := b.callWithContext(objManager, MANAGED_OBJECTS)
	if call.Err != nil {
		logger.Warn("[bluetooth] failed to query BlueZ managed objects: %v", call.Err)
		return nil, call.Err
	}
	if err := call.Store(&managedObjects); err != nil {
		logger.Warn("[bluetooth] failed to decode BlueZ managed objects: %v", err)
		return nil, err
	}
	return managedObjects, nil
}

func (b *BluetoothBackend) CheckBluetoothSupport() error {
	managedObjects, err := b.getManagedObjects()
	if err != nil {
		return err
	}

	found := false
	for _, ifaces := range managedObjects {
		if _, ok := ifaces["org.bluez.Adapter1"]; ok {
			found = true
			break
		}
	}
	if !found {
		logger.Info("[bluetooth] no adapter found, Bluetooth not supported")
		return &bluetoothUnsupportedError{}
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
		logger.Warn("[bluetooth] failed to unregister agent %s: %v", AGENT_PATH, err)
	}

	b.agent = nil
}

// StartDiscovery starts scanning for nearby Bluetooth devices
func (b *BluetoothBackend) StartDiscovery() error {
	adapter := b.getObj(BLUETOOTH_PREFIX, BLUETOOTH_PATH)
	if err := b.callMethod(adapter, BLUETOOTH_ADAPTER+".StartDiscovery"); err != nil {
		return err
	}
	logger.Debug("[bluetooth] discovery scan started")
	return nil
}

// StopDiscovery stops scanning for nearby Bluetooth devices
func (b *BluetoothBackend) StopDiscovery() error {
	if b.conn == nil {
		return nil // Connection already closed, nothing to do
	}
	adapter := b.getObj(BLUETOOTH_PREFIX, BLUETOOTH_PATH)
	if err := b.callMethod(adapter, BLUETOOTH_ADAPTER+".StopDiscovery"); err != nil {
		return err
	}
	logger.Debug("[bluetooth] discovery scan stopped")
	return nil
}

// addMatchRule adds a D-Bus match rule to filter signals
func (b *BluetoothBackend) addMatchRule(rule string) error {
	return b.callMethod(b.conn.BusObject(), DBUS_ADD_MATCH_METHOD, rule)
}

// removeMatchRule removes a D-Bus match rule
func (b *BluetoothBackend) removeMatchRule(rule string) error {
	return b.callMethod(b.conn.BusObject(), DBUS_REMOVE_MATCH_METHOD, rule)
}
