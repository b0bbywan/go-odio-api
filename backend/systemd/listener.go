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
	sysWatched := make(map[string]bool, len(l.backend.config.SystemServices))
	for _, name := range l.backend.config.SystemServices {
		sysWatched[name] = true
	}

	userWatched := make(map[string]bool, len(l.backend.config.SystemServices))
	for _, name := range l.backend.config.SystemServices {
		userWatched[name] = true
	}

	// Fonction de comparaison : détecter les changements réels
	isChanged := func(u1, u2 *dbus.UnitStatus) bool {
		if u1 == nil || u2 == nil {
			return true
		}
		// Changement si ActiveState ou LoadState différent
		return u1.ActiveState != u2.ActiveState || u1.LoadState != u2.LoadState
	}

	// Fonction de filtrage : ne surveiller que les services configurés
	// Cela évite de poll TOUS les units systemd à chaque intervalle
	filterUnit := func(name string) bool {
		return watched[name]
	}

	// Subscribe system scope
	sysStatusCh, sysErrCh := l.backend.sysConn.SubscribeUnitsCustomContext(
		l.ctx,
		time.Second, // interval de polling (1s suffit pour les changements d'état)
		10,          // buffer size réduit (moins d'events attendus)
		isChanged,
		filterUnit,
	)

	// Subscribe user scope
	userStatusCh, userErrCh := l.backend.userConn.SubscribeUnitsCustomContext(
		l.ctx,
		time.Second,
		10,
		isChanged,
		filterUnit,
	)

	// Goroutines d'écoute
	go l.listen(sysStatusCh, sysErrCh, ScopeSystem, sysWatched)
	go l.listen(userStatusCh, userErrCh, ScopeUser, userWatched)

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
