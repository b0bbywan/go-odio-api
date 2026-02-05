package mpris

import (
	"context"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// NewListener creates a new MPRIS listener
func NewListener(backend *MPRISBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	return &Listener{
		backend:   backend,
		ctx:       ctx,
		cancel:    cancel,
		lastState: make(map[string]PlaybackStatus),
	}
}

// Start starts listening to MPRIS D-Bus signals
func (l *Listener) Start() error {
	// Use the backend connection
	conn := l.backend.conn

	if err := l.backend.addListenMatchRules(); err != nil {
		return err
	}

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go l.listen(ch)

	logger.Info("[mpris] listener started (D-Bus signal-based)")
	return nil
}

// listen continuously listens to D-Bus signals
func (l *Listener) listen(ch <-chan *dbus.Signal) {
	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			logger.Debug("[mpris] received signal: %s from %s", sig.Name, sig.Sender)
			l.handleSignal(sig)
		}
	}
}

// handleSignal processes a D-Bus signal
func (l *Listener) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case DBUS_PROP_CHANGED_SIGNAL:
		l.handlePropertiesChanged(sig)
	case DBUS_NAME_OWNER_CHANGED:
		l.handleNameOwnerChanged(sig)
	default:
		logger.Debug("[mpris] unhandled signal: %s", sig.Name)
	}
}

// handlePropertiesChanged processes MPRIS property changes
func (l *Listener) handlePropertiesChanged(sig *dbus.Signal) {
	// Body[0] = interface name
	// Body[1] = changed properties (map[string]Variant)
	// Body[2] = invalidated properties ([]string)

	if len(sig.Body) < 2 {
		return
	}

	iface, ok := sig.Body[0].(string)
	if !ok || iface != MPRIS_PLAYER_IFACE {
		// We only care about Player changes
		return
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return
	}

	// The signal contains the unique name (:1.107), not the well-known name
	// Find the corresponding MPRIS player
	busName := l.backend.findPlayerByUniqueName(sig.Sender)
	if busName == "" {
		// Signal from unknown player, ignore
		return
	}

	// Check if PlaybackStatus changed for deduplication
	if statusVar, hasStatus := changed["PlaybackStatus"]; hasStatus {
		if status, ok := extractString(statusVar); ok {
			newStatus := PlaybackStatus(status)

			// Deduplication
			l.lastStateMu.RLock()
			lastStatus := l.lastState[busName]
			l.lastStateMu.RUnlock()

			if lastStatus != newStatus {
				l.lastStateMu.Lock()
				l.lastState[busName] = newStatus
				l.lastStateMu.Unlock()

				logger.Debug("[mpris] player %s changed status: %s -> %s", busName, lastStatus, newStatus)

				// If player switches to Playing, ensure heartbeat is running
				if newStatus == StatusPlaying {
					l.backend.heartbeat.Start()
				}
			}
		}
	}

	// Log the properties that will be updated
	propNames := make([]string, 0, len(changed))
	for propName := range changed {
		propNames = append(propNames, propName)
	}
	logger.Debug("[mpris] updating %s properties: %v", busName, propNames)

	// Update properties directly from signal (no D-Bus calls!)
	if err := l.backend.UpdatePlayerProperties(busName, changed); err != nil {
		logger.Error("[mpris] failed to update player %s properties: %v", busName, err)
	}
}

// handleNameOwnerChanged detects when a player appears or disappears
func (l *Listener) handleNameOwnerChanged(sig *dbus.Signal) {
	// Body[0] = bus name
	// Body[1] = old owner
	// Body[2] = new owner

	if len(sig.Body) < 3 {
		return
	}

	busName, ok := sig.Body[0].(string)
	if !ok || !strings.HasPrefix(busName, MPRIS_PREFIX+".") {
		return
	}

	oldOwner, _ := sig.Body[1].(string)
	newOwner, _ := sig.Body[2].(string)

	if oldOwner == "" && newOwner != "" {
		// New player appeared
		logger.Info("[mpris] new player detected: %s", busName)
		if _, err := l.backend.ReloadPlayerFromDBus(busName); err != nil {
			logger.Error("[mpris] failed to add new player %s: %v", busName, err)
		}
	} else if oldOwner != "" && newOwner == "" {
		// Player disappeared
		logger.Info("[mpris] player removed: %s", busName)
		if err := l.backend.RemovePlayer(busName); err != nil {
			logger.Error("[mpris] failed to remove player %s: %v", busName, err)
		}
	}
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[mpris] stopping listener")
	l.cancel()
	logger.Debug("[mpris] listener stopped")
}
