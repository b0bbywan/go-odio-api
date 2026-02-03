package systemd

import (
	"context"
	"log"
	"strings"
	"sync"

	"github.com/godbus/dbus/v5"
)

// Listener écoute les changements systemd via signaux D-Bus natifs (godbus)
type Listener struct {
	backend     *SystemdBackend
	ctx         context.Context
	cancel      context.CancelFunc
	sysWatched  map[string]bool
	userWatched map[string]bool

	// Déduplication : dernier état connu par service/scope
	lastState   map[string]string
	lastStateMu sync.RWMutex
}

func NewListener(backend *SystemdBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	// Map pour filtrage rapide
	sysWatched := make(map[string]bool, len(backend.config.SystemServices))
	for _, name := range backend.config.SystemServices {
		sysWatched[name] = true
	}

	userWatched := make(map[string]bool, len(backend.config.UserServices))
	for _, name := range backend.config.UserServices {
		userWatched[name] = true
	}


	return &Listener{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
		sysWatched: sysWatched,
		userWatched: userWatched,
		lastState: make(map[string]string),
	}
}

// Start démarre l'écoute des signaux D-Bus directement via godbus
func (l *Listener) Start() error {
	// Connexions D-Bus brutes pour les signaux
	sysConn, err := dbus.ConnectSystemBus()
	if err != nil {
		return err
	}

	userConn, err := dbus.ConnectSessionBus()
	if err != nil {
		sysConn.Close()
		return err
	}

	// S'abonner aux signaux de systemd (path filtre sur systemd1)
	matchRule := "type='signal',sender='org.freedesktop.systemd1'"

	if err := sysConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		sysConn.Close()
		userConn.Close()
		return err
	}

	if err := userConn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		sysConn.Close()
		userConn.Close()
		return err
	}

	// Channels pour les signaux
	sysCh := make(chan *dbus.Signal, 10)
	userCh := make(chan *dbus.Signal, 10)

	sysConn.Signal(sysCh)
	userConn.Signal(userCh)

	// Goroutines d'écoute
	go l.listen(sysCh, sysConn, ScopeSystem, l.sysWatched)
	go l.listen(userCh, userConn, ScopeUser, l.userWatched)

	log.Println("Systemd listener started (signal-based)")
	return nil
}

// unitNameFromPath extrait le nom de l'unité depuis le path D-Bus
// Ex: /org/freedesktop/systemd1/unit/spotifyd_2eservice -> spotifyd.service
func unitNameFromPath(path dbus.ObjectPath) string {
	s := string(path)
	const prefix = "/org/freedesktop/systemd1/unit/"
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	encoded := s[len(prefix):]
	// Décoder les caractères échappés (ex: _2e -> .)
	return decodeUnitName(encoded)
}

// decodeUnitName décode le nom d'unité échappé par systemd
func decodeUnitName(encoded string) string {
	var result strings.Builder
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '_' && i+2 < len(encoded) {
			// Séquence d'échappement _XX (hex)
			hex := encoded[i+1 : i+3]
			var b byte
			if _, err := parseHexByte(hex, &b); err == nil {
				result.WriteByte(b)
				i += 2
				continue
			}
		}
		result.WriteByte(encoded[i])
	}
	return result.String()
}

func parseHexByte(s string, b *byte) (bool, error) {
	if len(s) != 2 {
		return false, nil
	}
	val := 0
	for _, c := range s {
		val <<= 4
		switch {
		case c >= '0' && c <= '9':
			val |= int(c - '0')
		case c >= 'a' && c <= 'f':
			val |= int(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			val |= int(c - 'A' + 10)
		default:
			return false, nil
		}
	}
	*b = byte(val)
	return true, nil
}

// stateKey génère une clé unique pour le couple service/scope
func stateKey(name string, scope UnitScope) string {
	return string(scope) + "/" + name
}

func (l *Listener) listen(
	ch <-chan *dbus.Signal,
	conn *dbus.Conn,
	scope UnitScope,
	watched map[string]bool,
) {
	defer conn.Close()

	for {
		select {
		case <-l.ctx.Done():
			return

		case sig, ok := <-ch:
			if !ok {
				return
			}

			// Extraire le nom de l'unité depuis le path
			unitName := unitNameFromPath(sig.Path)
			if unitName == "" {
				continue
			}

			// Filtrer : uniquement les services surveillés
			if !watched[unitName] {
				continue
			}

			// Extraire SubState depuis les propriétés changées (signaux PropertiesChanged)
			if len(sig.Body) < 2 {
				continue
			}
			changed, ok := sig.Body[1].(map[string]dbus.Variant)
			if !ok {
				continue
			}

			subStateVar, hasSubState := changed["SubState"]
			if !hasSubState {
				continue
			}
			subState, ok := subStateVar.Value().(string)
			if !ok {
				continue
			}

			// Déduplication : ignorer si même état que précédemment
			key := stateKey(unitName, scope)
			l.lastStateMu.RLock()
			lastState := l.lastState[key]
			l.lastStateMu.RUnlock()

			if lastState == subState {
				continue
			}

			// Mettre à jour le dernier état connu
			l.lastStateMu.Lock()
			l.lastState[key] = subState
			l.lastStateMu.Unlock()

			log.Printf("Unit changed: %s/%s -> %s", scope, unitName, subState)
			if _, err := l.backend.RefreshService(unitName, scope); err != nil {
				log.Printf("Failed to refresh service %s/%s: %v", scope, unitName, err)
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	log.Println("Stopping systemd listener")
	l.cancel()
	log.Println("Stopped")
}
