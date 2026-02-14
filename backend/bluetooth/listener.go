package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// SignalCallback is called for each received signal.
// Returns true to stop the listener, false to continue.
type SignalCallback func(*dbus.Signal) bool

type DBusListener struct {
	conn      *dbus.Conn
	ctx       context.Context
	matchRule string
	callback  SignalCallback
	signals   chan *dbus.Signal
}

func NewDBusListener(conn *dbus.Conn, ctx context.Context, matchRule string, callback SignalCallback) *DBusListener {
	return &DBusListener{
		conn:      conn,
		ctx:       ctx,
		matchRule: matchRule,
		callback:  callback,
		signals:   make(chan *dbus.Signal, 10),
	}
}

func (l *DBusListener) Start() error {
	l.conn.Signal(l.signals)
	return l.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, l.matchRule).Err
}

func (l *DBusListener) Listen() {
	for {
		select {
		case <-l.ctx.Done():
			return
		case sig := <-l.signals:
			if l.callback(sig) {
				return
			}
		}
	}
}

func (l *DBusListener) Stop() {
	l.conn.RemoveSignal(l.signals)
	close(l.signals)
}
