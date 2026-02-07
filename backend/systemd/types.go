package systemd

import (
	"context"
	"sync"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
)

// Listener listens for systemd changes via native D-Bus signals (godbus)
type Listener struct {
	backend      *SystemdBackend
	ctx          context.Context
	cancel       context.CancelFunc
	sysWatched   map[string]bool
	userWatched  map[string]bool
	supportsUTMP bool

	// Deduplication: last known state per service/scope
	lastState   map[string]string
	lastStateMu sync.RWMutex
	watcherMap  sync.Map
}

type UnitScope string

const (
	ScopeSystem UnitScope = "system"
	ScopeUser   UnitScope = "user"
	cacheKey    string    = "services"
)

type SystemdBackend struct {
	sysConn  *dbus.Conn
	userConn *dbus.Conn
	ctx      context.Context
	config   *config.SystemdConfig // Comes from the config

	// permanent cache (no expiration)
	cache *cache.Cache[[]Service]

	// listener for systemd changes
	listener *Listener
}

type Service struct {
	Name        string    `json:"name"`
	Scope       UnitScope `json:"scope"`
	ActiveState string    `json:"active_state,omitempty"`
	Running     bool      `json:"running"`
	Enabled     bool      `json:"enabled"`
	Exists      bool      `json:"exists"`
	Description string    `json:"description,omitempty"`
}
