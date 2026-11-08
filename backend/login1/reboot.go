package login1

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/logger"
)

// RebootRequired returns the last read flag state, zero until Start.
func (l *Login1Backend) RebootRequired() RebootRequired {
	if r := l.reboot.Load(); r != nil {
		return *r
	}
	return RebootRequired{}
}

// startWatcher watches the parent dirs, not the files: the flag is created and
// removed, which a file watch cannot follow.
func (l *Login1Backend) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("[login1] cannot create watcher, reboot-required updates disabled: %v", err)
		return
	}
	for _, dir := range []string{filepath.Dir(l.rebootFile), filepath.Dir(l.rebootPkgs)} {
		if err := w.Add(dir); err != nil {
			logger.Warn("[login1] cannot watch %s, reboot-required updates disabled: %v", dir, err)
			if err := w.Close(); err != nil {
				logger.Warn("[login1] closing watcher: %v", err)
			}
			return
		}
	}
	l.watcher = w
	l.wg.Add(1)
	go l.watch()
}

func (l *Login1Backend) watch() {
	defer l.wg.Done()
	targets := []string{filepath.Clean(l.rebootFile), filepath.Clean(l.rebootPkgs)}
	const ops = fsnotify.Create | fsnotify.Write | fsnotify.Remove | fsnotify.Rename
	for {
		select {
		case <-l.ctx.Done():
			return
		case e, ok := <-l.watcher.Events:
			if !ok {
				return
			}
			if e.Op&ops != 0 && slices.Contains(targets, filepath.Clean(e.Name)) {
				l.readRebootRequired()
			}
		case err, ok := <-l.watcher.Errors:
			if !ok {
				return
			}
			logger.Warn("[login1] watcher error: %v", err)
		}
	}
}

// readRebootRequired ignores .pkgs without the flag: a leftover list alone does
// not mean a reboot is pending.
func (l *Login1Backend) readRebootRequired() {
	var next RebootRequired
	if _, err := os.Stat(l.rebootFile); err == nil {
		next.Required = true
		next.Packages = readPackages(l.rebootPkgs)
	} else if !errors.Is(err, fs.ErrNotExist) {
		logger.Warn("[login1] cannot stat %s: %v", l.rebootFile, err)
		return
	}

	if prev := l.reboot.Load(); prev != nil &&
		prev.Required == next.Required && slices.Equal(prev.Packages, next.Packages) {
		return
	}
	l.reboot.Store(&next)
	logger.Info("[login1] reboot required: %v %v", next.Required, next.Packages)
}

func readPackages(path string) []string {
	data, err := os.ReadFile(path)
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			logger.Warn("[login1] cannot read %s: %v", path, err)
		}
		return nil
	}
	var pkgs []string
	for line := range strings.Lines(string(data)) {
		if pkg := strings.TrimSpace(line); pkg != "" && !slices.Contains(pkgs, pkg) {
			pkgs = append(pkgs, pkg)
		}
	}
	return pkgs
}
