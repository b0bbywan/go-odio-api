package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// SignalCallback is called for each received D-Bus signal.
type SignalCallback func(*dbus.Signal) error

// DBusListener subscribes to D-Bus signals matching a rule and dispatches them to a callback.
type DBusListener struct {
	conn      *dbus.Conn
	ctx       context.Context
	matchRule string
	callback  SignalCallback
	signals   chan *dbus.Signal
}

// NewDBusListener creates a new D-Bus signal listener.
func NewDBusListener(conn *dbus.Conn, ctx context.Context, matchRule string, callback SignalCallback) *DBusListener {
	return &DBusListener{
		conn:      conn,
		ctx:       ctx,
		matchRule: matchRule,
		callback:  callback,
		signals:   make(chan *dbus.Signal, 10),
	}
}

// Start subscribes to D-Bus signals and begins dispatching to the callback.
func (l *DBusListener) Start() error {
	if err := l.conn.BusObject().Call(DBUS_ADD_MATCH_METHOD, 0, l.matchRule).Err; err != nil {
		return err
	}

	l.conn.Signal(l.signals)
	go l.listen()

	return nil
}

// listen dispatches signals to the callback until context is cancelled.
func (l *DBusListener) listen() {
	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-l.signals:
			if !ok {
				return
			}
			l.callback(sig)
		}
	}
}

// Stop removes the signal subscription and closes the channel.
func (l *DBusListener) Stop() {
	l.conn.RemoveSignal(l.signals)
	close(l.signals)
}
