package bluetooth

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// onSignal dispatches PropertiesChanged (adapter/device) and InterfacesAdded
// (scan discovery) signals to their handlers.
func (b *BluetoothBackend) onSignal(sig *dbus.Signal) bool {
	if sig == nil {
		return true // channel closed
	}
	if sig.Name == DBUS_OBJ_MANAGER+".InterfacesAdded" {
		if path, props, ok := parseInterfacesAdded(sig); ok {
			b.handleDiscoveredDevice(path, props)
		}
		return false
	}
	changed, iface, err := filterSignal(sig)
	if err != nil {
		logger.Debug("[bluetooth] signal filtered out: %v", err)
		return false
	}
	logger.Debug("[bluetooth] signal from %s (%s): changed properties=%v", sig.Path, iface, changedKeys(changed))
	switch iface {
	case BLUETOOTH_ADAPTER:
		b.onAdapterPropertiesChanged(changed)
	case BLUETOOTH_DEVICE:
		b.onDevicePropertiesChanged(sig.Path, changed)
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
	if powered, ok := extractMapBool(changed, BT_STATE_POWERED); ok {
		b.onAdapterPoweredChanged(powered)
	}

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

// onAdapterPoweredChanged reacts to power transitions, ours or external (CLI/GNOME).
func (b *BluetoothBackend) onAdapterPoweredChanged(powered bool) {
	if powered {
		logger.Info("[bluetooth] adapter powered on")
		b.updateStatus(func(s *BluetoothStatus) {
			s.Powered = true
		})
		b.refreshDevices()
		b.checkAndStartIdleTimer()
		return
	}
	logger.Info("[bluetooth] adapter powered off")
	b.cancelIdleTimer()
	b.cleanupPoweredState()
}

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

// parseInterfacesAdded extracts the device path and Device1 properties from an
// InterfacesAdded signal. ok is false when the signal is not about a Device1.
func parseInterfacesAdded(sig *dbus.Signal) (dbus.ObjectPath, map[string]dbus.Variant, bool) {
	if sig == nil || len(sig.Body) < 2 {
		return "", nil, false
	}
	path, ok := sig.Body[0].(dbus.ObjectPath)
	if !ok {
		return "", nil, false
	}
	ifaces, ok := sig.Body[1].(map[string]map[string]dbus.Variant)
	if !ok {
		return "", nil, false
	}
	dev, ok := ifaces[BLUETOOTH_DEVICE]
	if !ok {
		return "", nil, false
	}
	return path, dev, true
}

func extractMapBool(v map[string]dbus.Variant, value BluetoothState) (bool, bool) {
	if extractVar, ok := v[value.String()]; ok {
		return extractBool(extractVar)
	}
	return false, false
}

func changedAny(changed map[string]dbus.Variant, keys ...BluetoothState) bool {
	for _, k := range keys {
		if _, ok := changed[k.String()]; ok {
			return true
		}
	}
	return false
}

func changedKeys(changed map[string]dbus.Variant) []string {
	keys := make([]string, 0, len(changed))
	for k := range changed {
		keys = append(keys, k)
	}
	return keys
}
