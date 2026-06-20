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

// RunState is the run lifecycle: the upgrade.info payload and the status "run" snapshot (nil when idle).
type RunState struct {
	State   string  `json:"state"`             // "running" | "finished"
	Percent *int    `json:"percent,omitempty"` // live, while running
	Step    *string `json:"step,omitempty"`    // live, while running
	Success *bool   `json:"success,omitempty"` // set once finished
	Error   *string `json:"error,omitempty"`   // set once finished, when reported
}

// LastRun is the persisted verdict of the most recent finished run, surfaced under
// "last_run" so a client connecting after the fact (or after a restart) still sees it.
type LastRun struct {
	Success    bool   `json:"success"`
	FinishedAt string `json:"finished_at"`     // RFC3339
	Step       string `json:"step,omitempty"`  // last step seen, best-effort
	Error      string `json:"error,omitempty"` // script-reported, best-effort
}

// persistedState is the on-disk shape: the last verdict, plus an in-flight snapshot
// written only on graceful shutdown so the badge ring resumes at the right percent.
type persistedState struct {
	LastRun  *LastRun  `json:"last_run,omitempty"`
	InFlight *RunState `json:"in_flight,omitempty"`
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

// StatusResponse is the GET /upgrade payload: Status, the live run, and available triggers.
type StatusResponse struct {
	Status
	Run        *RunState `json:"run,omitempty"`
	LastRun    *LastRun  `json:"last_run,omitempty"`
	CanCheck   bool      `json:"can_check"`
	CanUpgrade bool      `json:"can_upgrade"`
}

// systemdUnits is the slice of the systemd backend the upgrade backend drives, declared as an
// interface so tests can stand in for the unit lifecycle without a real D-Bus connection.
// *systemd.SystemdBackend satisfies it.
type systemdUnits interface {
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
	stateFile      string

	systemd  systemdUnits          // triggers units (user scope); nil when absent
	status   cache.Value[*Status]  // last valid detector result; nil until first read
	lastRun  cache.Value[*LastRun] // verdict of the most recent finished run; persisted
	lastRaw  []byte                // last accepted result file bytes, for change dedup
	watcher  *fsnotify.Watcher
	listener net.Listener // unix socket the upgrade script streams progress to
	run      runTracker   // owns the run lifecycle; source decides who finishes it
	wg       sync.WaitGroup
	events   chan events.Event

	resumeHint *RunState // in-flight snapshot restored from disk, consumed by resumeIfRunning

	stream    events.Stream     // shared event bus; tracks the run unit's lifecycle
	sub       chan events.Event // our subscription to stream
	unitState string            // last-seen ActiveState of the upgrade unit (onServiceEvent only); dedups terminal events
}
