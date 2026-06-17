package upgrade

import (
	"path/filepath"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/logger"
)

// startWatcher watches the result file's parent dir (not the file): atomic writes
// replace the inode, which a file watch would miss. Non-fatal on failure.
func (u *UpgradeBackend) startWatcher() {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		logger.Warn("[upgrade] cannot create watcher, live updates disabled: %v", err)
		return
	}
	if err := w.Add(filepath.Dir(u.resultFile)); err != nil {
		logger.Warn("[upgrade] cannot watch %s, live updates disabled: %v", u.resultFile, err)
		if err := w.Close(); err != nil {
			logger.Warn("[upgrade] closing watcher: %v", err)
		}
		return
	}
	u.watcher = w
	u.wg.Add(1)
	go u.watch()
}

func (u *UpgradeBackend) watch() {
	defer u.wg.Done()
	target := filepath.Clean(u.resultFile)
	const ops = fsnotify.Write | fsnotify.Create | fsnotify.Rename
	for {
		select {
		case <-u.ctx.Done():
			return
		case e, ok := <-u.watcher.Events:
			if !ok {
				return
			}
			if filepath.Clean(e.Name) == target && e.Op&ops != 0 {
				u.readResult()
			}
		case err, ok := <-u.watcher.Errors:
			if !ok {
				return
			}
			logger.Warn("[upgrade] watcher error: %v", err)
		}
	}
}
