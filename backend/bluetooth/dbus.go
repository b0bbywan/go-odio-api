package bluetooth

import (
	"github.com/godbus/dbus/v5"
	"github.com/godbus/dbus/v5/introspect"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/logger"
)

func (b *BluetoothBackend) callMethod(obj dbus.BusObject, method string, args ...interface{}) error {
	return idbus.CallMethod(obj, method, args...)
}

func (b *BluetoothBackend) setProperty(obj dbus.BusObject, iface, prop string, value interface{}) error {
	return idbus.SetProperty(obj, iface, prop, value)
}

func (b *BluetoothBackend) getProperty(obj dbus.BusObject, iface, prop string) (dbus.Variant, error) {
	return idbus.GetProperty(obj, iface, prop)
}

func (b *BluetoothBackend) getAdapterProp(prop BluetoothState) (dbus.Variant, error) {
	return b.getProperty(b.adapter(), BLUETOOTH_ADAPTER, prop.String())
}

func (b *BluetoothBackend) adapter() dbus.BusObject {
	return b.getObj(BLUETOOTH_PREFIX, BLUETOOTH_PATH)
}

func (b *BluetoothBackend) setAdapterProp(prop string, value interface{}) error {
	return idbus.SetProperty(b.adapter(), BLUETOOTH_ADAPTER, prop, value)
}

// mapBTBool extracts a bool from a props map using a BluetoothState key.
func mapBTBool(props map[string]dbus.Variant, key BluetoothState) bool {
	return idbus.MapBool(props, key.String())
}

// mapBTBoolOK extracts a bool from a props map using a BluetoothState key, with existence check.
func mapBTBoolOK(props map[string]dbus.Variant, key BluetoothState) (bool, bool) {
	return idbus.MapBoolOK(props, key.String())
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
		idbus.INTROSPECTABLE,
	)
}

func (b *BluetoothBackend) getObj(prefix, path string) dbus.BusObject {
	return idbus.GetObject(b.conn, prefix, path)
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
		if trusted, ok := mapBTBoolOK(props, BT_STATE_TRUSTED); ok && trusted {
			device := BluetoothDevice{
				Address:   idbus.MapString(props, BT_PROP_ADDRESS),
				Name:      idbus.MapString(props, BT_PROP_NAME),
				Trusted:   trusted,
				Connected: mapBTBool(props, BT_STATE_CONNECTED),
			}
			devices = append(devices, device)
		}
		return true
	})

	return devices, err
}

func (b *BluetoothBackend) getAdapterBoolProp(prop BluetoothState) bool {
	v, err := b.getAdapterProp(prop)
	if err != nil {
		logger.Warn("[bluetooth] failed to get adapter %s: %v", prop, err)
		return false
	}
	val, _ := idbus.ExtractBool(v)
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
		if mapBTBool(props, BT_STATE_CONNECTED) {
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
		logger.Warn("[bluetooth] failed to trust device: %v", err)
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
	if err := objManager.Call(idbus.MANAGED_OBJECTS, 0).Store(&managedObjects); err != nil {
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
		if _, ok := ifaces[BLUETOOTH_ADAPTER]; ok {
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
		logger.Warn("[bluetooth] failed to register agent with capability %s: %v", AGENT_CAPABILITY, err)
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
