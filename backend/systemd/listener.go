package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
    actionStarted = "STARTED"
    actionStopped = "STOPPED"
)

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

	logger.Info("[systemd] %s listener started (D-Bus signal-based)", scope)
	return nil
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

	logger.Debug("[systemd] unit changed: %s/%s -> %s", scope, unitName, subState)
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
					logger.Error("[systemd] failed to refresh service %s/%s: %v", scope, unitName, err)
				}
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	logger.Info("[systemd] stopping listener")
	l.cancel()
	logger.Debug("[systemd] listener stopped")
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

	logger.Info("[systemd] user listener started (fsnotify), monitoring %s", unitsDir)

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
			l.dispatchHeadless(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error("[systemd] fsnotify watcher error: %v", err)
		}
	}
}

func (l *Listener) dispatchHeadless(event fsnotify.Event) {
	// Filtre sur invocation:*.service
	basename := filepath.Base(event.Name)
	if len(basename) <= 11 || basename[:11] != "invocation:" {
		return
	}

	serviceName := basename[11:]

	// Filtrer uniquement les services surveillés
	if !l.userWatched[serviceName] {
		return
	}
	var action string

	switch {
		case event.Has(fsnotify.Create):
			action = actionStarted
		case event.Has(fsnotify.Remove):
			action = actionStopped
		default:
			return

	}

	logger.Info("Service %s: %s/%s", action, ScopeUser, serviceName)
	if _, err := l.backend.RefreshService(serviceName, ScopeUser); err != nil {
		logger.Error("Failed to refresh service %s/%s: %v", ScopeUser, serviceName, err)
	}
	return
}
