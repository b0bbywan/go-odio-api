package mpris

import (
	"context"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// NewListener crée un nouveau listener MPRIS
func NewListener(backend *MPRISBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	return &Listener{
		backend:   backend,
		ctx:       ctx,
		cancel:    cancel,
		lastState: make(map[string]PlaybackStatus),
	}
}

// Start démarre l'écoute des signaux D-Bus MPRIS
func (l *Listener) Start() error {
	// Utiliser la connexion du backend
	conn := l.backend.conn

	// S'abonner aux signaux PropertiesChanged pour tous les lecteurs MPRIS
	matchRule := "type='signal',interface='" + dbusPropIface + "',member='PropertiesChanged',arg0namespace='" + mprisPrefix + "'"
	if err := conn.BusObject().Call(dbusAddMatchMethod, 0, matchRule).Err; err != nil {
		return err
	}

	// S'abonner aux signaux NameOwnerChanged pour détecter les nouveaux/anciens lecteurs
	ownerMatchRule := "type='signal',interface='" + dbusInterface + "',member='NameOwnerChanged',arg0namespace='" + mprisPrefix + "'"
	if err := conn.BusObject().Call(dbusAddMatchMethod, 0, ownerMatchRule).Err; err != nil {
		return err
	}

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go l.listen(ch)

	logger.Info("[mpris] listener started (D-Bus signal-based)")
	return nil
}

// listen écoute les signaux D-Bus en continu
func (l *Listener) listen(ch <-chan *dbus.Signal) {
	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			l.handleSignal(sig)
		}
	}
}

// handleSignal traite un signal D-Bus
func (l *Listener) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case dbusPropChangedSignal:
		l.handlePropertiesChanged(sig)
	case dbusNameOwnerChanged:
		l.handleNameOwnerChanged(sig)
	}
}

// handlePropertiesChanged traite les changements de propriétés MPRIS
func (l *Listener) handlePropertiesChanged(sig *dbus.Signal) {
	// Le sender est le bus name du lecteur
	busName := sig.Sender

	if !strings.HasPrefix(busName, mprisPrefix+".") {
		return
	}

	// Body[0] = interface name
	// Body[1] = changed properties (map[string]Variant)
	// Body[2] = invalidated properties ([]string)

	if len(sig.Body) < 2 {
		return
	}

	iface, ok := sig.Body[0].(string)
	if !ok || iface != mprisPlayerIface {
		// On ne s'intéresse qu'aux changements du Player
		return
	}

	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return
	}

	// Vérifier si PlaybackStatus a changé pour la déduplication
	if statusVar, hasStatus := changed["PlaybackStatus"]; hasStatus {
		if status, ok := statusVar.Value().(string); ok {
			newStatus := PlaybackStatus(status)

			// Déduplication
			l.lastStateMu.RLock()
			lastStatus := l.lastState[busName]
			l.lastStateMu.RUnlock()

			if lastStatus != newStatus {
				l.lastStateMu.Lock()
				l.lastState[busName] = newStatus
				l.lastStateMu.Unlock()

				logger.Debug("[mpris] player %s changed status: %s -> %s", busName, lastStatus, newStatus)

				// Si le player passe en Playing, s'assurer que le heartbeat tourne
				if newStatus == StatusPlaying {
					l.backend.ensureHeartbeatRunning()
				}
			}
		}
	}

	// Logger les propriétés qui vont être mises à jour
	propNames := make([]string, 0, len(changed))
	for propName := range changed {
		propNames = append(propNames, propName)
	}
	logger.Debug("[mpris] updating %s properties: %v", busName, propNames)

	// Mettre à jour les propriétés directement depuis le signal (pas d'appels D-Bus!)
	if err := l.backend.UpdatePlayerProperties(busName, changed); err != nil {
		logger.Error("[mpris] failed to update player %s properties: %v", busName, err)
	}
}

// handleNameOwnerChanged détecte quand un lecteur apparaît ou disparaît
func (l *Listener) handleNameOwnerChanged(sig *dbus.Signal) {
	// Body[0] = bus name
	// Body[1] = old owner
	// Body[2] = new owner

	if len(sig.Body) < 3 {
		return
	}

	busName, ok := sig.Body[0].(string)
	if !ok || !strings.HasPrefix(busName, mprisPrefix+".") {
		return
	}

	oldOwner, _ := sig.Body[1].(string)
	newOwner, _ := sig.Body[2].(string)

	if oldOwner == "" && newOwner != "" {
		// Nouveau lecteur apparu
		logger.Info("[mpris] new player detected: %s", busName)
		if _, err := l.backend.RefreshPlayer(busName); err != nil {
			logger.Error("[mpris] failed to add new player %s: %v", busName, err)
		}
	} else if oldOwner != "" && newOwner == "" {
		// Lecteur disparu
		logger.Info("[mpris] player removed: %s", busName)
		if err := l.backend.RemovePlayer(busName); err != nil {
			logger.Error("[mpris] failed to remove player %s: %v", busName, err)
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	logger.Info("[mpris] stopping listener")
	l.cancel()
	logger.Debug("[mpris] listener stopped")
}
