package systemd

import (
	"context"
	"log"

	"github.com/coreos/go-systemd/v22/dbus"
)

// Listener écoute les changements systemd via signaux D-Bus (pas de polling)
type Listener struct {
	backend *SystemdBackend
	ctx     context.Context
	cancel  context.CancelFunc
	watched map[string]bool
}

func NewListener(backend *SystemdBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	// Map pour filtrage rapide
	watched := make(map[string]bool, len(backend.serviceNames))
	for _, name := range backend.serviceNames {
		watched[name] = true
	}

	return &Listener{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
		watched: watched,
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
	sysErrCh := make(chan error, 1)
	userUpdateCh := make(chan *dbus.SubStateUpdate, 10)
	userErrCh := make(chan error, 1)

	// Enregistrer les subscribers
	l.backend.sysConn.SetSubStateSubscriber(sysUpdateCh, sysErrCh)
	l.backend.userConn.SetSubStateSubscriber(userUpdateCh, userErrCh)

	// Goroutines d'écoute
	go l.listen(sysUpdateCh, sysErrCh, ScopeSystem)
	go l.listen(userUpdateCh, userErrCh, ScopeUser)

	log.Println("Systemd listener started (signal-based)")
	return nil
}

func (l *Listener) listen(updateCh <-chan *dbus.SubStateUpdate, errCh <-chan error, scope UnitScope) {
	for {
		select {
		case <-l.ctx.Done():
			return

		case err, ok := <-errCh:
			if !ok {
				return
			}
			log.Printf("Systemd listener error (%s): %v", scope, err)

		case update, ok := <-updateCh:
			if !ok {
				return
			}

			// Filtrer : uniquement les services surveillés
			if !l.watched[update.UnitName] {
				continue
			}

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
	l.backend.sysConn.Unsubscribe()
	l.backend.userConn.Unsubscribe()
	l.cancel()
	log.Println("Stopped")
}
