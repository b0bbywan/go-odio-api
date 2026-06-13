package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"sync/atomic"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

const statusKey = "current"

// ErrUnitNotConfigured is returned when an action is requested but no systemd
// unit is configured (or the systemd backend is disabled).
var ErrUnitNotConfigured = errors.New("upgrade: systemd unit not configured")

// ErrUpgradeInProgress is returned when an upgrade is already running.
var ErrUpgradeInProgress = errors.New("upgrade: already in progress")

// Progress is the run lifecycle, emitted as upgrade.info data (distinct from
// the detector status payload). Success is set once State is "finished".
type Progress struct {
	State   string `json:"state"` // "running" | "finished"
	Success *bool  `json:"success,omitempty"`
}

// UpgradeBackend is an agnostic upgrade frontend: it reads and watches a result
// file written by an external detector, and triggers external systemd user
// units through the systemd backend. It does not know how detection or upgrade
// are implemented.
type UpgradeBackend struct {
	ctx            context.Context
	resultFile     string
	checkUnit      string
	upgradeUnit    string
	progressSocket string

	systemd  *systemd.SystemdBackend // triggers units (user scope); may be nil
	cache    *cache.Cache[json.RawMessage]
	watcher  *fsnotify.Watcher
	listener net.Listener // unix socket the upgrade script streams progress to
	running  atomic.Bool  // guards against concurrent upgrades
	wg       sync.WaitGroup
	events   chan events.Event
}
