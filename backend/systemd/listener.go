package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// Listener écoute les changements systemd via signaux D-Bus natifs (godbus)
type Listener struct {
	backend     *SystemdBackend
	ctx         context.Context
	cancel      context.CancelFunc
	sysWatched  map[string]bool
	userWatched map[string]bool
	headless    bool

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
		headless:  backend.config.Headless,
		lastState: make(map[string]string),
	}
}

// Start démarre l'écoute des signaux D-Bus directement via godbus
func (l *Listener) Start() error {
	// Connexions D-Bus brutes pour les signaux
	if err := l.startScope(ScopeSystem, l.sysWatched); err != nil {
		return err
	}

	// En mode headless, utiliser fsnotify au lieu de D-Bus pour les services user
	if l.headless {
		if err := l.StartHeadless(); err != nil {
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
	var conn *dbus.Conn
	var err error
	if scope == ScopeSystem {
		if conn, err = dbus.ConnectSystemBus(); err != nil {
			return err
		}
	} else if scope == ScopeUser {
		if conn, err = dbus.ConnectSessionBus(); err != nil {
			return err
		}
	}

	// S'abonner aux signaux de systemd (path filtre sur systemd1)
	matchRule := "type='signal',sender='org.freedesktop.systemd1'"

	if err := conn.BusObject().Call("org.freedesktop.DBus.AddMatch", 0, matchRule).Err; err != nil {
		conn.Close()
		return err
	}
	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go l.listen(ch, conn, scope, watched)

	logger.Info("%s Systemd listener started (signal-based)", scope)
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

func (l *Listener) checkUnit(sig *dbus.Signal, scope UnitScope) (string, bool) {
// Extraire le nom de l'unité depuis le path
	var unitName string
	var ok bool

	unitName = unitNameFromPath(sig.Path)
	if unitName == "" {
		return unitName, ok
	}

	// Filtrer : uniquement les services surveillés
	if !l.Watched(unitName, scope) {
		return unitName, ok
	}

	// Extraire SubState depuis les propriétés changées (signaux PropertiesChanged)
	if len(sig.Body) < 2 {
		return unitName, ok
	}
	changed, ok := sig.Body[1].(map[string]dbus.Variant)
	if !ok {
		return unitName, ok
	}

	subStateVar, hasSubState := changed["SubState"]
	if !hasSubState {
		return unitName, ok
	}
	subState, ok := subStateVar.Value().(string)
	if !ok {
		return unitName, ok
	}

	// Déduplication : ignorer si même état que précédemment
	key := stateKey(unitName, scope)
	l.lastStateMu.RLock()
	lastState := l.lastState[key]
	l.lastStateMu.RUnlock()

	if lastState == subState {
		return unitName, ok
	}

	// Mettre à jour le dernier état connu
	l.lastStateMu.Lock()
	l.lastState[key] = subState
	l.lastStateMu.Unlock()

	logger.Debug("Unit changed: %s/%s -> %s", scope, unitName, subState)
	return unitName, true
}

func (l *Listener) Watched(unitName string, scope UnitScope) bool{
	switch scope {
	case ScopeSystem:
		if !l.sysWatched[unitName] {
			return false
		}
		return true
	case ScopeUser:
		if !l.userWatched[unitName] {
			return false
		}
		return true
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
	defer conn.Close()

	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			if unitName, ok := l.checkUnit(sig, scope); ok {
				if _, err := l.backend.RefreshService(unitName, scope); err != nil {
					logger.Error("Failed to refresh service %s/%s: %v", scope, unitName, err)
				}
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	logger.Info("Stopping systemd listener")
	l.cancel()
	logger.Info("Stopped")
}

// StartHeadless démarre l'écoute des événements systemd via fsnotify
func (l *Listener) StartHeadless() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Obtenir le UID de l'utilisateur courant
	uid := os.Getuid()
	unitsDir := fmt.Sprintf("/run/user/%d/systemd/units", uid)

	// Vérifier que le répertoire existe
	if _, err := os.Stat(unitsDir); os.IsNotExist(err) {
		watcher.Close()
		return fmt.Errorf("units directory does not exist: %s", unitsDir)
	}

	if err := watcher.Add(unitsDir); err != nil {
		watcher.Close()
		return err
	}

	logger.Info("Monitoring %s for systemd user events (fsnotify)", unitsDir)

	go l.listenHeadless(watcher)

	return nil
}

func (l *Listener) listenHeadless(watcher *fsnotify.Watcher) {
	defer watcher.Close()

	for {
		select {
		case <-l.ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}

			// Filtre sur invocation:*.service
			basename := filepath.Base(event.Name)
			if len(basename) <= 11 || basename[:11] != "invocation:" {
				continue
			}

			serviceName := basename[11:]

			// Filtrer uniquement les services surveillés
			if !l.userWatched[serviceName] {
				continue
			}

			switch {
			case event.Op&fsnotify.Create == fsnotify.Create:
				logger.Info("Service STARTED: %s/%s", ScopeUser, serviceName)
				if _, err := l.backend.RefreshService(serviceName, ScopeUser); err != nil {
					logger.Error("Failed to refresh service %s/%s: %v", ScopeUser, serviceName, err)
				}

			case event.Op&fsnotify.Remove == fsnotify.Remove:
				logger.Info("Service STOPPED: %s/%s", ScopeUser, serviceName)
				if _, err := l.backend.RefreshService(serviceName, ScopeUser); err != nil {
					logger.Error("Failed to refresh service %s/%s: %v", ScopeUser, serviceName, err)
				}
			}

		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error("Systemd fsnotify listener error: %v", err)
		}
	}
}
