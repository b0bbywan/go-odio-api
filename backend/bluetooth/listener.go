package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// SignalCallback is called for each received signal.
// Returns true to stop the listener, false to continue.
type SignalCallback func(*dbus.Signal) bool

type DBusListener struct {
	conn       *dbus.Conn
	ctx        context.Context
	matchRules []string
	callback   SignalCallback
	signals    chan *dbus.Signal
}

func NewDBusListener(conn *dbus.Conn, ctx context.Context, matchRules []string, callback SignalCallback) *DBusListener {
	return &DBusListener{
		conn:       conn,
		ctx:        ctx,
		matchRules: matchRules,
		signals:    make(chan *dbus.Signal, 10),
		callback:   callback,
	}
}

func (l *DBusListener) Start() error {
	l.conn.Signal(l.signals)
	for _, rule := range l.matchRules {
		if err := l.conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, rule).Err; err != nil {
			return err
		}
	}
	return nil
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
	if l.conn != nil {
		l.conn.RemoveSignal(l.signals)
		for _, rule := range l.matchRules {
			_ = l.conn.BusObject().Call("org.freedesktop.DBus.RemoveMatch", 0, rule).Err
		}
	}
	close(l.signals)
}
