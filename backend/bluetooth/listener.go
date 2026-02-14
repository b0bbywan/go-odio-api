package bluetooth

import (
	"context"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// BluetoothListener listens to BlueZ D-Bus signals during pairing.
// It is ephemeral: created and destroyed within a single pairing session.
type BluetoothListener struct {
	backend *BluetoothBackend
	ctx     context.Context
	cancel  context.CancelFunc
	done    chan struct{}
}

// NewBluetoothListener creates a new listener scoped to the given context.
func NewBluetoothListener(backend *BluetoothBackend, ctx context.Context) *BluetoothListener {
	listenerCtx, cancel := context.WithCancel(ctx)

	return &BluetoothListener{
		backend: backend,
		ctx:     listenerCtx,
		cancel:  cancel,
		done:    make(chan struct{}),
	}
}

// Start subscribes to BlueZ PropertiesChanged signals and starts listening.
func (l *BluetoothListener) Start() error {
	matchRule := "type='signal',interface='" + DBUS_PROP_IFACE + "',member='PropertiesChanged',arg0='" + BLUETOOTH_DEVICE + "'"
	if err := l.backend.addMatchRule(matchRule); err != nil {
		return err
	}

	ch := make(chan *dbus.Signal, 10)
	l.backend.conn.Signal(ch)

	go l.listen(ch)

	logger.Info("[bluetooth] pairing listener started (D-Bus signal-based)")
	return nil
}

// listen continuously listens to D-Bus signals until context is cancelled.
func (l *BluetoothListener) listen(ch chan *dbus.Signal) {
	defer l.backend.conn.RemoveSignal(ch)

	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			logger.Debug("[bluetooth] received signal: %s from %s (path: %s)", sig.Name, sig.Sender, sig.Path)
			l.handleSignal(sig)
		}
	}
}

// handleSignal dispatches a D-Bus signal to the appropriate handler.
func (l *BluetoothListener) handleSignal(sig *dbus.Signal) {
	if sig.Name == DBUS_PROP_CHANGED_SIGNAL {
		l.handlePropertiesChanged(sig)
	}
}

// handlePropertiesChanged processes org.bluez.Device1 PropertiesChanged signals.
// When "Paired" becomes true, it trusts the device and signals completion.
func (l *BluetoothListener) handlePropertiesChanged(sig *dbus.Signal) {
	// Body[0] = interface name (string)
	// Body[1] = changed properties (map[string]dbus.Variant)
	// Body[2] = invalidated properties ([]string)

	if len(sig.Body) < 2 {
		return
	}

	iface, ok := sig.Body[0].(string)
	if !ok || iface != BLUETOOTH_DEVICE {
		return
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return
	}

	// Check if the device path belongs to our adapter
	devicePath := sig.Path
	if !strings.HasPrefix(string(devicePath), BLUETOOTH_PATH+"/") {
		return
	}

	pairedVar, hasPaired := changed["Paired"]
	if !hasPaired {
		return
	}

	paired, ok := extractBool(pairedVar)
	if !ok || !paired {
		return
	}

	logger.Info("[bluetooth] device %s paired via D-Bus signal", devicePath)

	if l.backend.trustDevice(devicePath) {
		logger.Info("[bluetooth] device %s trusted", devicePath)
		l.backend.refreshKnownDevices()
		close(l.done)
	}
}

// Done returns a channel that is closed when pairing completes successfully.
func (l *BluetoothListener) Done() <-chan struct{} {
	return l.done
}

// Stop cancels the listener context, causing the listen goroutine to exit.
func (l *BluetoothListener) Stop() {
	logger.Debug("[bluetooth] stopping pairing listener")
	l.cancel()
}
