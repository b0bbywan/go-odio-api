package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// StartFSNotifier starts listening for systemd events via fsnotify
func (l *Listener) StartFSNotifier() error {
	if len(l.userWatched) == 0 {
		return nil
	}

	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}

	// Use the XDG_RUNTIME_DIR directory from the config
	unitsDir := filepath.Join(l.backend.config.XDGRuntimeDir, "systemd/units")

	// Verify that the directory exists
	if _, err := os.Stat(unitsDir); os.IsNotExist(err) {
		if closeErr := watcher.Close(); closeErr != nil {
			logger.Info("[systemd] Failed to close watcher: %v", closeErr)
		}
		return fmt.Errorf("units directory does not exist: %s", unitsDir)
	}

	if err := watcher.Add(unitsDir); err != nil {
		if closeErr := watcher.Close(); closeErr != nil {
			logger.Info("[systemd] Failed to close watcher: %v", closeErr)
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
			logger.Warn("[systemd] Failed to close watcher: %v", err)
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
	switch {
	case event.Has(fsnotify.Create):
		logger.Debug("[systemd] %s starting", filepath.Base(event.Name))
	case event.Has(fsnotify.Remove):
		logger.Debug("[systemd] %s stopping", filepath.Base(event.Name))
	default:
		logger.Debug(
			"[systemd] %s other event. chmod: %v, write: %v, rename: %v",
			filepath.Base(event.Name),
			event.Has(fsnotify.Chmod),
			event.Has(fsnotify.Write),
			event.Has(fsnotify.Rename),
		)
	}

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

	if _, loaded := l.watcherMap.LoadOrStore(serviceName, true); !loaded {
		go l.waitForStableState(serviceName)
	}
}

func (l *Listener) waitForStableState(service string) {
	ctx, cancel := context.WithTimeout(l.backend.ctx, 65*time.Second)
	defer func() {
		l.watcherMap.Delete(service)
		if ctx.Err() == context.DeadlineExceeded {
			logger.Warn("[systemd] %s failed to start in less than 60s, cache might be out ouf sync", service)
		}
		cancel()
	}()

	logger.Debug("[systemd] waitForStableState %s", service)
	waitTime := 500 * time.Millisecond
	maxWait := 8 * time.Second
	factor := 1.5

	// fires immediately
	timer := time.NewTimer(0)
	defer timer.Stop()

	for {
		select {
		case <-ctx.Done():
			logger.Debug("[systemd] ending stable state wait")
			return
		case <-timer.C:
			unit, err := l.backend.RefreshService(ctx, service, ScopeUser)
			if err != nil {
				timer.Reset(waitTime)
				continue
			}
			switch unit.ActiveState {
			case "active", "inactive", "failed":
				logger.Debug("[systemd] %s/%s reached stable state: %s", ScopeUser, service, unit.ActiveState)
				l.backend.notify(events.Event{Type: events.TypeServiceUpdated, Data: *unit})
				return
			}
			logger.Debug("[systemd] %s/%s still in transitional state: %s", ScopeUser, service, unit.ActiveState)
			timer.Reset(waitTime)

			waitTime = time.Duration(float64(waitTime) * factor)
			if waitTime > maxWait {
				waitTime = maxWait
			}
		}
	}
}
