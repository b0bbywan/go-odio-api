package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// RunState is the live progress of an in-flight upgrade. It rides under the
// status endpoint's "run" key so the endpoint reflects an ongoing run, and is
// nil whenever no upgrade is running. Percent/Step are present only once the
// script streams them.
type RunState struct {
	State   string  `json:"state"` // always "running" while in flight
	Percent *int    `json:"percent,omitempty"`
	Step    *string `json:"step,omitempty"`
}

// Status is the detector result. The typed fields are the contract odio relies
// on; UnmarshalJSON routes everything else the detector writes (roles, manifest,
// future fields) verbatim into Extra, kept apart under "extra".
type Status struct {
	Current          string                     `json:"current"`
	Latest           string                     `json:"latest"`
	UpgradeAvailable bool                       `json:"upgrade_available"`
	CheckedAt        string                     `json:"checked_at,omitempty"`
	Extra            map[string]json.RawMessage `json:"extra,omitempty"`
}

func (s *Status) UnmarshalJSON(b []byte) error {
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	for _, req := range []string{"current", "latest", "upgrade_available"} {
		if _, ok := m[req]; !ok {
			return fmt.Errorf("missing required field %q", req)
		}
	}
	known := map[string]any{
		"current":           &s.Current,
		"latest":            &s.Latest,
		"upgrade_available": &s.UpgradeAvailable,
		"checked_at":        &s.CheckedAt,
	}
	for key, v := range m {
		if dst, isKnown := known[key]; isKnown {
			if err := json.Unmarshal(v, dst); err != nil {
				return err
			}
			continue
		}
		if s.Extra == nil {
			s.Extra = make(map[string]json.RawMessage)
		}
		s.Extra[key] = v
	}
	return nil
}

// StatusResponse is the GET /upgrade payload: the detector Status fields plus
// the live run state, nil unless an upgrade is in flight.
type StatusResponse struct {
	Status
	Run *RunState `json:"run,omitempty"`
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
	cache    *cache.Cache[*Status]
	lastRaw  []byte // last accepted result file bytes, for change dedup
	watcher  *fsnotify.Watcher
	listener net.Listener             // unix socket the upgrade script streams progress to
	running  atomic.Bool              // guards against concurrent upgrades
	runState atomic.Pointer[RunState] // live run progress for the status endpoint; nil when idle
	wg       sync.WaitGroup
	events   chan events.Event

	stream events.Stream     // shared event bus; tracks the run unit's lifecycle
	sub    chan events.Event // our subscription to stream
}
