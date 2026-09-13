package systemd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"

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

	l.track(serviceName)
}

// track starts a watcher for service, or marks the running one: an event it
// has not seen may postdate the state it is about to settle on (a restart).
func (l *Listener) track(service string) {
	l.watchMu.Lock()
	defer l.watchMu.Unlock()
	if l.watching == nil {
		l.watching = make(map[string]bool)
	}
	if _, running := l.watching[service]; running {
		l.watching[service] = true
		return
	}
	l.watching[service] = false
	go l.waitForStableState(service)
}

// settle ends the watch on service, unless an event came in meanwhile: then it
// clears the mark and reports false, and the watcher reads the state again.
func (l *Listener) settle(service string) bool {
	l.watchMu.Lock()
	defer l.watchMu.Unlock()
	if l.watching[service] {
		l.watching[service] = false
		return false
	}
	delete(l.watching, service)
	return true
}

func (l *Listener) untrack(service string) {
	l.watchMu.Lock()
	defer l.watchMu.Unlock()
	delete(l.watching, service)
}

func (l *Listener) waitForStableState(service string) {
	timeout := l.backend.config.Timeout
	ctx, cancel := context.WithTimeout(l.backend.ctx, timeout)
	settled := false
	defer func() {
		// Once settled the entry may already belong to a newer watcher.
		if !settled {
			l.untrack(service)
		}
		if ctx.Err() == context.DeadlineExceeded {
			logger.Warn("[systemd] %s failed to start in less than %s, cache might be out of sync", service, timeout)
			refreshCtx, refreshCancel := context.WithTimeout(l.backend.ctx, 5*time.Second)
			if unit, err := l.backend.RefreshService(refreshCtx, service, ScopeUser); err == nil {
				l.backend.notifyService(*unit)
			}
			refreshCancel()
		}
		cancel()
	}()

	logger.Debug("[systemd] waitForStableState %s", service)
	waitTime := 1 * time.Second
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
			if isStableState(unit.ActiveState) {
				if !l.settle(service) {
					logger.Debug("[systemd] %s/%s changed while settling on %s, reading again", ScopeUser, service, unit.ActiveState)
					timer.Reset(0)
					continue
				}
				settled = true
				logger.Debug("[systemd] %s/%s reached stable state: %s", ScopeUser, service, unit.ActiveState)
				l.backend.notifyService(*unit)
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

func isStableState(state string) bool {
	switch state {
	case "active", "inactive", "failed":
		return true
	}
	return false
}

// trackTransitional follows the watched user units the startup snapshot caught
// mid-transition: their invocation link predates the watcher, so no event comes.
func (l *Listener) trackTransitional(services []Service) {
	if l.supportsUTMP {
		return // D-Bus signals report the end of the transition
	}
	for _, name := range transitionalUnits(services, l.userWatched) {
		l.track(name)
	}
}

// transitionalUnits lists the watched user units not yet in a stable state.
func transitionalUnits(services []Service, watched map[string]bool) []string {
	var names []string
	for _, svc := range services {
		if svc.Scope == ScopeUser && watched[svc.Name] && !isStableState(svc.ActiveState) {
			names = append(names, svc.Name)
		}
	}
	return names
}
