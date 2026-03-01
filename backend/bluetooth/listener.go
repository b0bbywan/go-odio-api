package bluetooth

import (
	"context"

	"github.com/godbus/dbus/v5"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
)

// SignalCallback is called for each received signal.
// Returns true to stop the listener, false to continue.
type SignalCallback func(*dbus.Signal) bool

type DBusListener struct {
	conn       *dbus.Conn
	ctx        context.Context
	cancel     context.CancelFunc
	matchRules []string
	callback   SignalCallback
	signals    chan *dbus.Signal
}

func NewDBusListener(conn *dbus.Conn, parentCtx context.Context, matchRules []string, callback SignalCallback) *DBusListener {
	ctx, cancel := context.WithCancel(parentCtx)
	return &DBusListener{
		conn:       conn,
		ctx:        ctx,
		cancel:     cancel,
		matchRules: matchRules,
		signals:    make(chan *dbus.Signal, 10),
		callback:   callback,
	}
}

func (l *DBusListener) Start() error {
	l.conn.Signal(l.signals)
	for _, rule := range l.matchRules {
		if err := idbus.AddMatchRule(l.conn, rule); err != nil {
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
	l.cancel()
	if l.conn != nil {
		l.conn.RemoveSignal(l.signals)
		for _, rule := range l.matchRules {
			_ = idbus.RemoveMatchRule(l.conn, rule)
		}
	}
	close(l.signals)
}
