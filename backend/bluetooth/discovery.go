package bluetooth

import (
	"fmt"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// StartScan begins active device discovery. Discovered devices appear in the
// status device list and are pushed through bluetooth.discovered events; the
// scan runs until StopScan is called or scanTimeout elapses.
func (b *BluetoothBackend) StartScan() error {
	if !b.isAdapterOn() {
		if err := b.PowerUp(); err != nil {
			return err
		}
	}

	b.scanMu.Lock()
	defer b.scanMu.Unlock()

	if b.GetStatus().Scanning {
		return nil
	}

	// Best-effort: focus on classic audio devices, but keep scanning on failure.
	if err := b.setDiscoveryFilter(); err != nil {
		logger.Warn("[bluetooth] discovery filter not applied, scanning unfiltered: %v", err)
	}

	if err := b.startDiscovery(); err != nil {
		return err
	}

	b.startDiscoveryListener()
	b.startScanTimer()
	// A scan is activity: don't let the idle timer power the adapter off mid-scan.
	b.cancelIdleTimer()

	b.updateStatus(func(s *BluetoothStatus) {
		s.Scanning = true
	})
	logger.Info("[bluetooth] scan started")
	return nil
}

// StopScan ends active discovery. It is idempotent. Scanning is cleared
// unconditionally: StopScan also runs on the power-down path (adapter already
// off) where StopDiscovery fails, and we must still converge to not-scanning;
// the BlueZ error is surfaced to the caller.
func (b *BluetoothBackend) StopScan() error {
	b.scanMu.Lock()
	defer b.scanMu.Unlock()

	if !b.GetStatus().Scanning {
		return nil
	}

	b.stopDiscoveryListener()
	err := b.stopDiscovery()
	b.updateStatus(func(s *BluetoothStatus) {
		s.Scanning = false
	})
	b.refreshDevices()
	// Re-arm the idle timer we suppressed in StartScan; it no-ops if a device
	// got connected, otherwise it resumes the auto-power-off countdown.
	b.checkAndStartIdleTimer()
	logger.Info("[bluetooth] scan stopped")
	return err
}

// startScanTimer schedules an automatic StopScan after scanTimeout (0 disables it).
func (b *BluetoothBackend) startScanTimer() {
	armed := b.scanTimer.Start(b.scanTimeout, func() {
		logger.Info("[bluetooth] scan timeout reached after %v, stopping scan", b.scanTimeout)
		if err := b.StopScan(); err != nil {
			logger.Warn("[bluetooth] failed to stop scan on timeout: %v", err)
		}
	})
	if armed {
		logger.Info("[bluetooth] scan timer started (%v)", b.scanTimeout)
	}
}

// cancelScanTimer stops the auto-stop timer.
func (b *BluetoothBackend) cancelScanTimer() {
	b.scanTimer.Cancel()
}

func (b *BluetoothBackend) startDiscoveryListener() {
	rules := []string{
		fmt.Sprintf("type='signal',interface='%s',member='InterfacesAdded'", DBUS_OBJ_MANAGER),
	}
	listener := NewDBusListener(b.conn, b.ctx, rules, b.onInterfacesAdded)
	if err := listener.Start(); err != nil {
		listener.Stop()
		logger.Warn("[bluetooth] failed to start discovery listener: %v", err)
		return
	}
	b.discoveryListener = listener
	go listener.Listen()
}

func (b *BluetoothBackend) stopDiscoveryListener() {
	if b.discoveryListener != nil {
		b.discoveryListener.Stop()
		b.discoveryListener = nil
		logger.Debug("[bluetooth] listener stopped")
	}
	b.cancelScanTimer()
}

func (b *BluetoothBackend) onInterfacesAdded(sig *dbus.Signal) bool {
	if sig == nil {
		return true // channel closed
	}
	path, props, ok := parseInterfacesAdded(sig)
	if !ok {
		return false
	}
	b.handleDiscoveredDevice(path, props)
	return false
}

// handleDiscoveredDevice reacts to a device appearing during a scan by pushing
// it as a discovery event, built straight from the signal's properties. The
// settled known_devices list is reconciled with BlueZ on the next real
// transition (connect/disconnect/pair) and when the scan stops.
func (b *BluetoothBackend) handleDiscoveredDevice(path dbus.ObjectPath, props map[string]dbus.Variant) {
	// Ignore devices belonging to another adapter.
	if adapterVar, ok := props[BT_PROP_ADAPTER]; ok {
		if ap, ok := adapterVar.Value().(dbus.ObjectPath); ok && string(ap) != BLUETOOTH_PATH {
			return
		}
	}
	address := extractString(props, BT_PROP_ADDRESS)
	if address == "" {
		address = addressFromPath(path)
	}
	if address == "" {
		return
	}

	// Hold scanMu across the check and the emit so a concurrent StopScan can't
	// slip a discovered event out after the scan is reported stopped.
	b.scanMu.Lock()
	defer b.scanMu.Unlock()
	if !b.GetStatus().Scanning {
		return
	}
	b.notifyDiscovered(BluetoothDevice{
		Address:   address,
		Name:      extractString(props, BT_PROP_NAME),
		Paired:    extractBoolProp(props, BT_STATE_PAIRED),
		Trusted:   extractBoolProp(props, BT_STATE_TRUSTED),
		Connected: extractBoolProp(props, BT_STATE_CONNECTED),
	})
}

func (b *BluetoothBackend) notifyDiscovered(device BluetoothDevice) {
	select {
	case b.events <- events.Event{Type: events.TypeBluetoothDiscovered, Data: device}:
	default:
		logger.Warn("[bluetooth] event channel full, dropping %s event", events.TypeBluetoothDiscovered)
	}
}
