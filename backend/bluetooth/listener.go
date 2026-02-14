package bluetooth

import (
	"context"
	"errors"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

var errPairingComplete = errors.New("pairing complete")

// SignalCallback is called for each received D-Bus signal.
// Return nil to continue listening, non-nil to signal completion.
type SignalCallback func(*dbus.Signal) error

// DBusListener subscribes to D-Bus signals matching a rule and dispatches them to a callback.
type DBusListener struct {
	conn      *dbus.Conn
	ctx       context.Context
	cancel    context.CancelFunc
	matchRule string
	callback  SignalCallback
	done      chan struct{}
}

// NewDBusListener creates a new D-Bus signal listener.
func NewDBusListener(conn *dbus.Conn, ctx context.Context, matchRule string, callback SignalCallback) *DBusListener {
	listenerCtx, cancel := context.WithCancel(ctx)

	return &DBusListener{
		conn:      conn,
		ctx:       listenerCtx,
		cancel:    cancel,
		matchRule: matchRule,
		callback:  callback,
		done:      make(chan struct{}),
	}
}

// Start subscribes to D-Bus signals and begins dispatching to the callback.
func (l *DBusListener) Start() error {
	if err := l.conn.BusObject().Call(DBUS_ADD_MATCH_METHOD, 0, l.matchRule).Err; err != nil {
		return err
	}

	ch := make(chan *dbus.Signal, 10)
	l.conn.Signal(ch)

	go l.listen(ch)

	return nil
}

// listen dispatches signals to the callback until context cancellation or callback completion.
func (l *DBusListener) listen(ch chan *dbus.Signal) {
	defer l.conn.RemoveSignal(ch)

	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if err := l.callback(sig); err != nil {
				close(l.done)
				return
			}
		}
	}
}

// Done returns a channel that is closed when the callback signals completion.
func (l *DBusListener) Done() <-chan struct{} {
	return l.done
}

// Stop cancels the listener context, causing the listen goroutine to exit.
func (l *DBusListener) Stop() {
	l.cancel()
}

// onDevicePaired handles a D-Bus PropertiesChanged signal during pairing.
// Returns errPairingComplete when a device is successfully paired and trusted.
func (b *BluetoothBackend) onDevicePaired(sig *dbus.Signal) error {
	if sig.Name != DBUS_PROP_CHANGED_SIGNAL || len(sig.Body) < 2 {
		return nil
	}

	iface, ok := sig.Body[0].(string)
	if !ok || iface != BLUETOOTH_DEVICE {
		return nil
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return nil
	}

	if !strings.HasPrefix(string(sig.Path), BLUETOOTH_PATH+"/") {
		return nil
	}

	pairedVar, hasPaired := changed["Paired"]
	if !hasPaired {
		return nil
	}

	paired, ok := extractBool(pairedVar)
	if !ok || !paired {
		return nil
	}

	logger.Info("[bluetooth] device %s paired via D-Bus signal", sig.Path)

	if b.trustDevice(sig.Path) {
		logger.Info("[bluetooth] device %s trusted", sig.Path)
		b.refreshKnownDevices()
		return errPairingComplete
	}

	return nil
}
