package dbus

import (
	"context"
	"sync"

	"github.com/godbus/dbus/v5"
)

// BusType identifies which D-Bus bus to use.
type BusType int

const (
	SystemBus  BusType = iota
	SessionBus BusType = iota
)

// DBusBackend owns the system and session D-Bus connections and distributes
// signals to subscribers. Connections are opened lazily on first use.
// Signal dispatching is set up lazily on first Subscribe call.
type DBusBackend struct {
	ctx         context.Context
	mu          sync.Mutex
	sysConn     *dbus.Conn
	sessionConn *dbus.Conn
	sysDisp     *signalDispatcher
	sessionDisp *signalDispatcher
}

type signalDispatcher struct {
	broadcaster *Broadcaster[*dbus.Signal]
	mu          sync.Mutex
	subs        map[chan *dbus.Signal][]string // match rules per subscriber
}

func NewDBusBackend(ctx context.Context) *DBusBackend {
	return &DBusBackend{ctx: ctx}
}

// SystemConn returns the shared system bus connection, opening it if needed.
func (d *DBusBackend) SystemConn() (*dbus.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensureConn(SystemBus)
}

// SessionConn returns the shared session bus connection, opening it if needed.
func (d *DBusBackend) SessionConn() (*dbus.Conn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensureConn(SessionBus)
}

// Subscribe registers match rules on the given bus and returns a channel that
// receives matching signals. An optional filter further narrows delivery.
func (d *DBusBackend) Subscribe(bus BusType, matchRules []string, filter func(*dbus.Signal) bool) (chan *dbus.Signal, error) {
	d.mu.Lock()
	conn, err := d.ensureConn(bus)
	if err != nil {
		d.mu.Unlock()
		return nil, err
	}
	disp := d.ensureDisp(bus, conn)
	d.mu.Unlock()

	disp.mu.Lock()
	defer disp.mu.Unlock()

	for _, rule := range matchRules {
		if err := AddMatchRule(conn, rule); err != nil {
			return nil, err
		}
	}
	ch := disp.broadcaster.SubscribeFunc(filter)
	disp.subs[ch] = matchRules
	return ch, nil
}

// Unsubscribe removes match rules and closes the subscriber channel.
func (d *DBusBackend) Unsubscribe(bus BusType, ch chan *dbus.Signal) {
	d.mu.Lock()
	conn := d.conn(bus)
	disp := d.disp(bus)
	d.mu.Unlock()

	if disp == nil || conn == nil {
		return
	}

	disp.mu.Lock()
	rules := disp.subs[ch]
	delete(disp.subs, ch)
	disp.mu.Unlock()

	for _, rule := range rules {
		_ = RemoveMatchRule(conn, rule)
	}
	disp.broadcaster.Unsubscribe(ch)
}

// Close closes both bus connections.
func (d *DBusBackend) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.sysConn != nil {
		_ = d.sysConn.Close()
		d.sysConn = nil
		d.sysDisp = nil
	}
	if d.sessionConn != nil {
		_ = d.sessionConn.Close()
		d.sessionConn = nil
		d.sessionDisp = nil
	}
}

// ensureConn opens the connection for the given bus if not already open.
// Must be called with d.mu held.
func (d *DBusBackend) ensureConn(bus BusType) (*dbus.Conn, error) {
	switch bus {
	case SystemBus:
		if d.sysConn == nil {
			conn, err := dbus.ConnectSystemBus()
			if err != nil {
				return nil, err
			}
			d.sysConn = conn
		}
		return d.sysConn, nil
	case SessionBus:
		if d.sessionConn == nil {
			conn, err := dbus.ConnectSessionBus()
			if err != nil {
				return nil, err
			}
			d.sessionConn = conn
		}
		return d.sessionConn, nil
	}
	return nil, nil
}

// ensureDisp sets up the signal dispatcher for the given bus if not already done.
// Must be called with d.mu held.
func (d *DBusBackend) ensureDisp(bus BusType, conn *dbus.Conn) *signalDispatcher {
	switch bus {
	case SystemBus:
		if d.sysDisp == nil {
			d.sysDisp = newSignalDispatcher(d.ctx, conn)
		}
		return d.sysDisp
	case SessionBus:
		if d.sessionDisp == nil {
			d.sessionDisp = newSignalDispatcher(d.ctx, conn)
		}
		return d.sessionDisp
	}
	return nil
}

// conn returns the connection for the given bus without locking.
// Must be called with d.mu held.
func (d *DBusBackend) conn(bus BusType) *dbus.Conn {
	switch bus {
	case SystemBus:
		return d.sysConn
	case SessionBus:
		return d.sessionConn
	}
	return nil
}

// disp returns the dispatcher for the given bus without locking.
// Must be called with d.mu held.
func (d *DBusBackend) disp(bus BusType) *signalDispatcher {
	switch bus {
	case SystemBus:
		return d.sysDisp
	case SessionBus:
		return d.sessionDisp
	}
	return nil
}

func newSignalDispatcher(ctx context.Context, conn *dbus.Conn) *signalDispatcher {
	signals := make(chan *dbus.Signal, 64)
	conn.Signal(signals)
	return &signalDispatcher{
		broadcaster: NewBroadcaster[*dbus.Signal](ctx, signals),
		subs:        make(map[chan *dbus.Signal][]string),
	}
}
