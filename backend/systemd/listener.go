package systemd

import (
	"context"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

func NewListener(backend *SystemdBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	// Map for fast filtering
	sysWatched := make(map[string]bool, len(backend.config.SystemServices))
	for _, name := range backend.config.SystemServices {
		sysWatched[name] = true
	}

	userWatched := make(map[string]bool, len(backend.config.UserServices))
	for _, name := range backend.config.UserServices {
		userWatched[name] = true
	}

	return &Listener{
		backend:      backend,
		ctx:          ctx,
		cancel:       cancel,
		sysWatched:   sysWatched,
		userWatched:  userWatched,
		supportsUTMP: backend.config.SupportsUTMP,
		lastState:    make(map[string]string),
	}
}

// Start starts listening for D-Bus signals directly via godbus
func (l *Listener) Start() error {
	// Raw D-Bus connections for signals
	if err := l.startScope(ScopeSystem, l.sysWatched); err != nil {
		return err
	}

	// In headless mode, use fsnotify instead of D-Bus for user services
	if !l.supportsUTMP {
		if err := l.StartFSNotifier(); err != nil {
			l.Stop()
			return err
		}
	} else {
		if err := l.startScope(ScopeUser, l.userWatched); err != nil {
			l.Stop()
			return err
		}
	}

	return nil
}

func (l *Listener) startScope(scope UnitScope, watched map[string]bool) error {
	if len(watched) == 0 {
		logger.Debug("[systemd] no units configured for %s scope, skipping listener", scope)
		return nil
	}

	var conn *dbus.Conn
	var err error
	switch scope {
	case ScopeSystem:
		conn, err = dbus.ConnectSystemBus()
	case ScopeUser:
		conn, err = dbus.ConnectSessionBus()
	}
	if err != nil {
		return err
	}

	// Subscribe to systemd signals (path filters on systemd1)
	matchRule := "type='signal',sender='org.freedesktop.systemd1'"

	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		if closeErr := conn.Close(); closeErr != nil {
			logger.Info("Failed to close D-Bus connection: %v", closeErr)
		}
		return err
	}
	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go l.listen(ch, conn, scope, watched)

	logger.Info("[systemd] %s listener started (D-Bus signal-based)", scope)
	return nil
}

func (l *Listener) checkUnit(sig *dbus.Signal, scope UnitScope) (string, bool) {
	// Extract unit name from the path
	unitName := unitNameFromPath(sig.Path)
	if unitName == "" {
		return unitName, false
	}

	// Filter: only monitored services
	if !l.Watched(unitName, scope) {
		return unitName, false
	}

	// Extract SubState from changed properties (PropertiesChanged signals)
	if len(sig.Body) < 2 {
		return unitName, false
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return unitName, false
	}

	subStateVar, hasSubState := changed["SubState"]
	if !hasSubState {
		return unitName, false
	}
	subState, ok := subStateVar.Value().(string)
	if !ok {
		return unitName, false
	}

	// Deduplication: ignore if same state as previously
	key := stateKey(unitName, scope)
	l.lastStateMu.RLock()
	lastState := l.lastState[key]
	l.lastStateMu.RUnlock()

	if lastState == subState {
		return unitName, false
	}

	// Update the last known state
	l.lastStateMu.Lock()
	l.lastState[key] = subState
	l.lastStateMu.Unlock()

	logger.Debug("[systemd] unit changed: %s/%s -> %s", scope, unitName, subState)
	return unitName, true
}

func (l *Listener) Watched(unitName string, scope UnitScope) bool {
	switch scope {
	case ScopeSystem:
		return l.sysWatched[unitName]
	case ScopeUser:
		return l.userWatched[unitName]
	default:
		return false
	}
}

func (l *Listener) listen(
	ch <-chan *dbus.Signal,
	conn *dbus.Conn,
	scope UnitScope,
	watched map[string]bool,
) {
	defer func() {
		if err := conn.Close(); err != nil {
			logger.Info("Failed to close D-Bus connection: %v", err)
		}
	}()

	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if unitName, ok := l.checkUnit(sig, scope); ok {
				if svc, err := l.backend.RefreshService(l.ctx, unitName, scope); err != nil {
					logger.Error("[systemd] failed to refresh service %s/%s: %v", scope, unitName, err)
				} else {
					l.backend.notify(events.Event{Type: events.TypeServiceUpdated, Data: *svc})
				}
			}
		}
	}
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[systemd] stopping listener")
	l.cancel()
	logger.Debug("[systemd] listener stopped")
}
