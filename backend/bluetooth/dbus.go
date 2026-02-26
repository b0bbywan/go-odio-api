package bluetooth

import (
	"fmt"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	"github.com/b0bbywan/go-odio-api/logger"
)

func filterSignal(sig *dbus.Signal) (map[string]dbus.Variant, string, error) {
	if sig == nil {
		return nil, "", fmt.Errorf("channel closed")
	}

	if len(sig.Body) < 2 {
		return nil, "", fmt.Errorf("signal from %s ignored: body too short", sig.Path)
	}

	iface, ok := sig.Body[0].(string)
	if !ok {
		return nil, "", fmt.Errorf("failed to parse iface")
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return nil, "", fmt.Errorf("signal from %s ignored: body[1] is not map[string]Variant", sig.Path)
	}

	return changed, iface, nil
}

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
	return b.getProperty(b.adapter(), BLUETOOTH_ADAPTER, prop.String())
}

func (b *BluetoothBackend) adapter() dbus.BusObject {
	return b.getObj(BLUETOOTH_PREFIX, BLUETOOTH_PATH)
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

func extractMapBool(v map[string]dbus.Variant, value BluetoothState) (bool, bool) {
	if extractVar, ok := v[value.String()]; ok {
		return extractBool(extractVar)
	}
	return false, false
}

func changedKeys(changed map[string]dbus.Variant) []string {
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	return keys
}

func (b *BluetoothBackend) exportAgent(agent *bluezAgent) error {
	path := agent.Path()
	iface := agent.Interface()

	// Export the agent object to D-Bus
	if err := b.conn.Export(agent, path, iface); err != nil {
		return err
	}

	// Export introspection data so BlueZ can discover the agent's methods
	node := &introspect.Node{
		Interfaces: []introspect.Interface{
			introspect.IntrospectData,
			{
				Name:    iface,
				Methods: introspect.Methods(agent),
			},
		},
	}
	return b.conn.Export(
		introspect.NewIntrospectable(node),
		path,
		DBUS_INTROSPECTABLE,
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
		if trusted, ok := extractMapBool(props, BT_STATE_TRUSTED); ok && trusted {
			// Extract device info
			device := BluetoothDevice{
				Address:   extractString(props, BT_PROP_ADDRESS),
				Name:      extractString(props, BT_PROP_NAME),
				Trusted:   trusted,
				Connected: extractBoolProp(props, BT_STATE_CONNECTED),
			}
			devices = append(devices, device)
		}
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

func extractBoolProp(props map[string]dbus.Variant, key BluetoothState) bool {
	if v, ok := props[key.String()]; ok {
		if b, ok := v.Value().(bool); ok {
			return b
		}
	}
	return false
}

func (b *BluetoothBackend) getAdapterBoolProp(prop BluetoothState) bool {
	v, err := b.getAdapterProp(prop)
	if err != nil {
		logger.Warn("[bluetooth] failed to get adapter %s: %v", prop, err)
		return false
	}
	val, _ := extractBool(v)
	return val
}

func (b *BluetoothBackend) isAdapterOn() bool {
	return b.getAdapterBoolProp(BT_STATE_POWERED)
}

func (b *BluetoothBackend) isPairable() bool {
	return b.getAdapterBoolProp(BT_STATE_PAIRABLE)
}

func (b *BluetoothBackend) isDiscoverable() bool {
	return b.getAdapterBoolProp(BT_STATE_DISCOVERABLE)
}

func (b *BluetoothBackend) hasConnectedDevices() bool {
	connected := false
	err := b.iterateAdapterDevices(func(path dbus.ObjectPath, props map[string]dbus.Variant) bool {
		if extractBoolProp(props, BT_STATE_CONNECTED) {
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

func (b *BluetoothBackend) trustDevice(path dbus.ObjectPath) bool {
	obj := b.getObj(BLUETOOTH_PREFIX, string(path))
	if err := b.setProperty(obj, BLUETOOTH_DEVICE, BT_STATE_TRUSTED.String(), true); err != nil {
		logger.Warn("[bluetooth] failed to trust devices: %v", err)
		return false
	}
	return true
}

func (b *BluetoothBackend) SetPairable(state bool) error {
	if err := b.setAdapterProp(BT_STATE_PAIRABLE.String(), state); err != nil {
		logger.Warn("[bluetooth] failed to set adapter pairable %v: %v", state, err)
		return err
	}
	return nil
}

func (b *BluetoothBackend) SetDiscoverable(state bool) error {
	if err := b.setAdapterProp(BT_STATE_DISCOVERABLE.String(), state); err != nil {
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
	if err := b.setAdapterProp(BT_STATE_POWERED.String(), state); err != nil {
		logger.Warn("[bluetooth] failed to set adapter powered %v: %v", state, err)
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
	if err := objManager.Call(MANAGED_OBJECTS, 0).Store(&managedObjects); err != nil {
		logger.Warn("[bluetooth] failed to query BlueZ managed objects: %v", err)
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
