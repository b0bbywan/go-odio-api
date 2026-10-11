package input

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/logger"
)

const nodePrefix = "event"

// startWatcher watches devRoot for hotplug. Non-fatal on failure: the devices
// present at startup are still handled.
func (b *InputBackend) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("[input] cannot create watcher, hotplug disabled: %v", err)
		return
	}
	if err := w.Add(b.devRoot); err != nil {
		logger.Warn("[input] cannot watch %s, hotplug disabled: %v", b.devRoot, err)
		if err := w.Close(); err != nil {
			logger.Warn("[input] closing watcher: %v", err)
		}
		return
	}
	b.watcher = w
	b.wg.Add(1)
	go b.watch()
}

func (b *InputBackend) watch() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case e, ok := <-b.watcher.Events:
			if !ok {
				return
			}
			node := filepath.Base(e.Name)
			if !strings.HasPrefix(node, nodePrefix) {
				continue
			}
			switch {
			case e.Has(fsnotify.Create):
				b.attach(node)
			case e.Has(fsnotify.Remove):
				b.detach(node)
			}
		case err, ok := <-b.watcher.Errors:
			if !ok {
				return
			}
			logger.Warn("[input] watcher error: %v", err)
		}
	}
}

func (b *InputBackend) scan() {
	entries, err := os.ReadDir(b.devRoot)
	if err != nil {
		logger.Warn("[input] cannot list %s: %v", b.devRoot, err)
		return
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), nodePrefix) {
			b.attach(e.Name())
		}
	}
}
