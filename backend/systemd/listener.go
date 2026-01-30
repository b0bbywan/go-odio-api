package systemd

import (
	"context"
	"log"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"
)

// Listener écoute les changements systemd via D-Bus
type Listener struct {
	backend *SystemdBackend
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewListener(backend *SystemdBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)
	return &Listener{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start démarre l'écoute des événements D-Bus
func (l *Listener) Start() error {
	// Map pour filtrage rapide
	watched := make(map[string]bool, len(l.backend.serviceNames))
	for _, name := range l.backend.serviceNames {
		watched[name] = true
	}

	// Fonction de comparaison : détecter les changements réels
	isChanged := func(u1, u2 *dbus.UnitStatus) bool {
		if u1 == nil || u2 == nil {
			return true
		}
		// Changement si ActiveState ou LoadState différent
		return u1.ActiveState != u2.ActiveState || u1.LoadState != u2.LoadState
	}

	// Subscribe system scope (sans filtre, on filtre nous-mêmes)
	sysStatusCh, sysErrCh := l.backend.sysConn.SubscribeUnitsCustomContext(
		l.ctx,
		250*time.Millisecond, // interval de polling
		100,                  // buffer size
		isChanged,
		nil, // pas de filtre ici, on filtre dans listen()
	)

	// Subscribe user scope
	userStatusCh, userErrCh := l.backend.userConn.SubscribeUnitsCustomContext(
		l.ctx,
		250*time.Millisecond,
		100,
		isChanged,
		nil, // pas de filtre ici, on filtre dans listen()
	)

	// Goroutines d'écoute
	go l.listen(sysStatusCh, sysErrCh, ScopeSystem, watched)
	go l.listen(userStatusCh, userErrCh, ScopeUser, watched)

	log.Println("Systemd listener started")
	return nil
}

func (l *Listener) listen(statusCh <-chan map[string]*dbus.UnitStatus, errCh <-chan error, scope UnitScope, watched map[string]bool) {
	for {
		select {
		case <-l.ctx.Done():
			return

		case err, ok := <-errCh:
			if !ok {
				return
			}
			log.Printf("Systemd listener error (%s): %v", scope, err)

		case statuses, ok := <-statusCh:
			if !ok {
				return
			}

			// Filtrer et rafraîchir uniquement les services surveillés
			for name := range statuses {
				// IMPORTANT: filtrer ici
				if !watched[name] {
					continue
				}

				log.Printf("Unit changed: %s/%s", scope, name)
				if _, err := l.backend.RefreshService(name, scope); err != nil {
					log.Printf("Failed to refresh service %s/%s: %v", scope, name, err)
				}
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	log.Println("Stopping systemd listener")
	// Unsubscribe pour fermer les channels
	l.backend.sysConn.Unsubscribe()
	l.backend.userConn.Unsubscribe()
	l.cancel()
	log.Println("Stoppped")
}
