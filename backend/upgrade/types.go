package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

var ErrUnitNotConfigured = errors.New("upgrade: systemd unit not configured")

var ErrUpgradeInProgress = errors.New("upgrade: already in progress")

// RunState is the run lifecycle: the upgrade.info payload and the status "run"
// snapshot. The verdict is the state itself; idle is none or last succeeded.
type RunState struct {
	State      string  `json:"state"`                 // "idle" | "running" | "failed"
	Origin     string  `json:"origin,omitempty"`      // "systemd" | "socket"
	Percent    *int    `json:"percent,omitempty"`     // live, while running
	Step       *string `json:"step,omitempty"`        // live, while running
	StartedAt  string  `json:"started_at,omitempty"`  // RFC3339
	FinishedAt string  `json:"finished_at,omitempty"` // RFC3339
}

// Status is the detector result; UnmarshalJSON routes unknown fields verbatim into Extra.
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
	if s.CheckedAt != "" {
		if _, err := time.Parse(time.RFC3339, s.CheckedAt); err != nil {
			s.CheckedAt = "" // optional timestamp, malformed: drop it, keep the rest
		}
	}
	return nil
}

// StatusResponse is the GET /upgrade payload: Status, the run snapshot, and available triggers.
type StatusResponse struct {
	Status
	Run        RunState `json:"run"`
	CanCheck   bool     `json:"can_check"`
	CanUpgrade bool     `json:"can_upgrade"`
}

// systemdControl is the slice of systemd the upgrade backend depends on: enough
// to trigger the check/upgrade units and read one unit's state on resume.
// *systemd.SystemdBackend satisfies it; tests supply a fake.
type systemdControl interface {
	StartService(name string, scope systemd.UnitScope) error
	TriggerUserUnit(ctx context.Context, name string) error
	RefreshService(ctx context.Context, name string, scope systemd.UnitScope) (*systemd.Service, error)
}

// UpgradeBackend watches a detector result file and triggers systemd user units to upgrade.
type UpgradeBackend struct {
	ctx            context.Context
	resultFile     string
	checkUnit      string
	upgradeUnit    string
	progressSocket string

	systemd  systemdControl       // triggers and reads units (user scope); nil interface when disabled
	status   cache.Value[*Status] // last valid detector result; nil until first read
	lastRaw  []byte               // last accepted result file bytes, for change dedup
	watcher  *fsnotify.Watcher
	listener net.Listener // unix socket the upgrade script streams progress to
	run      *run         // run lifecycle and persisted snapshot
	wg       sync.WaitGroup
	events   chan events.Event

	stream events.Stream     // shared event bus; tracks the run unit's lifecycle
	sub    chan events.Event // our subscription to stream
}
