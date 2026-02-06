package systemd

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/logger"
)

// StartHeadless starts listening for systemd events via fsnotify
func (l *Listener) StartFSNotifier() error {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Use the XDG_RUNTIME_DIR directory from the config
	unitsDir := filepath.Join(l.backend.config.XDGRuntimeDir, "systemd/units")

	// Verify that the directory exists
	if _, err := os.Stat(unitsDir); os.IsNotExist(err) {
		if closeErr := watcher.Close(); closeErr != nil {
			logger.Info("Failed to close watcher: %v", closeErr)
		}
		return fmt.Errorf("units directory does not exist: %s", unitsDir)
	}

	if err := watcher.Add(unitsDir); err != nil {
		if closeErr := watcher.Close(); closeErr != nil {
			logger.Info("Failed to close watcher: %v", closeErr)
		}
		return err
	}

	logger.Info("[systemd] user listener started (fsnotify), monitoring %s", unitsDir)

	go l.listenFSNotify(watcher)

	return nil
}

func (l *Listener) listenFSNotify(watcher *fsnotify.Watcher) {
	defer func() {
		if err := watcher.Close(); err != nil {
			logger.Info("Failed to close watcher: %v", err)
		}
	}()

	for {
		select {
		case <-l.ctx.Done():
			return

		case event, ok := <-watcher.Events:
			if !ok {
				return
			}
			l.dispatchFSNotify(event)
		case err, ok := <-watcher.Errors:
			if !ok {
				return
			}
			logger.Error("[systemd] fsnotify watcher error: %v", err)
		}
	}
}

func (l *Listener) dispatchFSNotify(event fsnotify.Event) {
	// Filter on invocation:*.service
	basename := filepath.Base(event.Name)
	if len(basename) <= 11 || basename[:11] != "invocation:" {
		return
	}

	serviceName := basename[11:]

	// Filter only monitored services
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

	logger.Info("[systemd] Service %s: %s/%s", action, ScopeUser, serviceName)
	if _, err := l.backend.RefreshService(serviceName, ScopeUser); err != nil {
		logger.Error("[systemd] Failed to refresh service %s/%s: %v", ScopeUser, serviceName, err)
	}
}
