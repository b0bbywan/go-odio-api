package systemd

import (
	"context"
	"log"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"
)

// Listener écoute les changements systemd via signaux D-Bus (pas de polling)
type Listener struct {
	backend *SystemdBackend
	ctx     context.Context
	cancel  context.CancelFunc
	watched map[string]bool

	// Déduplication : dernier état connu par service/scope
	lastState   map[string]string
	lastStateMu sync.RWMutex
}

func NewListener(backend *SystemdBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	// Map pour filtrage rapide
	watched := make(map[string]bool, len(backend.serviceNames))
	for _, name := range backend.serviceNames {
		watched[name] = true
	}

	return &Listener{
		backend:   backend,
		ctx:       ctx,
		cancel:    cancel,
		watched:   watched,
		lastState: make(map[string]string),
	}
}

// Start démarre l'écoute des signaux D-Bus
func (l *Listener) Start() error {
	// Subscribe aux signaux D-Bus pour chaque connexion
	if err := l.backend.sysConn.Subscribe(); err != nil {
		return err
	}
	if err := l.backend.userConn.Subscribe(); err != nil {
		return err
	}

	// Channels pour recevoir les mises à jour (signaux D-Bus natifs, pas de polling)
	sysUpdateCh := make(chan *dbus.SubStateUpdate, 10)
	sysErrCh := make(chan error, 10)
	userUpdateCh := make(chan *dbus.SubStateUpdate, 10)
	userErrCh := make(chan error, 10)

	// Enregistrer les subscribers
	l.backend.sysConn.SetSubStateSubscriber(sysUpdateCh, sysErrCh)
	l.backend.userConn.SetSubStateSubscriber(userUpdateCh, userErrCh)

	// Goroutines d'écoute
	go l.listen(sysUpdateCh, sysErrCh, ScopeSystem)
	go l.listen(userUpdateCh, userErrCh, ScopeUser)

	log.Println("Systemd listener started (signal-based)")
	return nil
}

// stateKey génère une clé unique pour le couple service/scope
func stateKey(name string, scope UnitScope) string {
	return string(scope) + "/" + name
}

func (l *Listener) listen(updateCh <-chan *dbus.SubStateUpdate, errCh <-chan error, scope UnitScope) {
	for {
		select {
		case <-l.ctx.Done():
			return

		case _, ok := <-errCh:
			if !ok {
				return
			}
			// Ignorer les erreurs (souvent spam au shutdown)

		case update, ok := <-updateCh:
			if !ok {
				return
			}

			// Filtrer : uniquement les services surveillés
			if !l.watched[update.UnitName] {
				continue
			}

			// Déduplication : ignorer si même état que précédemment
			key := stateKey(update.UnitName, scope)
			l.lastStateMu.RLock()
			lastState := l.lastState[key]
			l.lastStateMu.RUnlock()

			if lastState == update.SubState {
				continue
			}

			// Mettre à jour le dernier état connu
			l.lastStateMu.Lock()
			l.lastState[key] = update.SubState
			l.lastStateMu.Unlock()

			log.Printf("Unit changed: %s/%s -> %s", scope, update.UnitName, update.SubState)
			if _, err := l.backend.RefreshService(update.UnitName, scope); err != nil {
				log.Printf("Failed to refresh service %s/%s: %v", scope, update.UnitName, err)
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	log.Println("Stopping systemd listener")
	l.cancel()
	l.backend.sysConn.Unsubscribe()
	l.backend.userConn.Unsubscribe()
	log.Println("Stopped")
}
