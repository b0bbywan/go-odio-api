package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New creates the upgrade backend, or returns nil when disabled or no result
// file is configured. Triggers are delegated to sysd, with which the units are
// registered as internal; detection works even when sysd is nil.
func New(ctx context.Context, cfg *config.UpgradeConfig, sysd *systemd.SystemdBackend) (*UpgradeBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	if cfg.ResultFile == "" {
		logger.Warn("[upgrade] enabled but no resultFile configured, disabling backend")
		return nil, nil
	}

	// Register the units as internal so they are triggerable and hidden from
	// /services. Done before the systemd backend starts (its listener snapshots
	// the unit list then), so no manual systemd.user config is needed.
	if sysd != nil {
		sysd.AddInternalUserUnits(cfg.CheckUnit, cfg.UpgradeUnit)
	} else if cfg.CheckUnit != "" || cfg.UpgradeUnit != "" {
		logger.Warn("[upgrade] systemd backend disabled; triggers unavailable")
	}

	return &UpgradeBackend{
		ctx:            ctx,
		resultFile:     cfg.ResultFile,
		checkUnit:      cfg.CheckUnit,
		upgradeUnit:    cfg.UpgradeUnit,
		progressSocket: cfg.ProgressSocket,
		systemd:        sysd,
		cache:          cache.New[*Status](0), // TTL=0 = no expiration
		events:         make(chan events.Event, 16),
	}, nil
}

// UseEventStream wires the shared event bus, which the backend subscribes to in
// Start to track its run unit's lifecycle. Wired by Backend.New once the
// broadcaster exists.
func (u *UpgradeBackend) UseEventStream(s events.Stream) { u.stream = s }

// Start reads the current result file, watches it for changes, listens for run
// progress streamed by the upgrade script, and subscribes to the event bus to
// track its run unit.
func (u *UpgradeBackend) Start() error {
	u.readResult() // best-effort; a missing file is not an error
	u.startWatcher()
	u.startListener()
	u.subscribeEvents()
	logger.Info("[upgrade] backend started successfully")
	return nil
}

// Close stops the watcher, waits for the watch goroutine to exit, then closes
// the event channel. Waiting avoids a send-on-closed-channel race.
func (u *UpgradeBackend) Close() {
	if u.watcher != nil {
		if err := u.watcher.Close(); err != nil {
			logger.Warn("[upgrade] closing watcher: %v", err)
		}
	}
	// Closing the listener unblocks Accept; an active connection unblocks via the
	// already-cancelled ctx (see readProgress).
	if u.listener != nil {
		if err := u.listener.Close(); err != nil {
			logger.Warn("[upgrade] closing progress listener: %v", err)
		}
	}
	// Unsubscribe closes u.sub, unblocking consumeEvents (which also exits on the
	// already-cancelled ctx).
	if u.sub != nil && u.stream != nil {
		u.stream.Unsubscribe(u.sub)
	}
	u.wg.Wait()
	close(u.events)
	u.watcher = nil
	u.listener = nil
}

// Events returns the read-only event channel for this backend.
func (u *UpgradeBackend) Events() <-chan events.Event { return u.events }

// GetStatus returns the last valid detector result, or nil if none.
func (u *UpgradeBackend) GetStatus() *Status {
	status, _ := u.cache.Get(statusKey)
	return status
}

// StatusResponse is the GET /upgrade payload: the detector result plus live run
// progress while an upgrade is in flight, so the endpoint reflects an ongoing
// run. Returns nil when nothing is known yet.
func (u *UpgradeBackend) StatusResponse() any {
	status := u.GetStatus()
	run := u.runState.Load()
	if status == nil && run == nil {
		return nil
	}
	resp := StatusResponse{Run: run}
	if status != nil {
		resp.Status = *status
	}
	return resp
}

func (u *UpgradeBackend) notify(e events.Event) {
	select {
	case u.events <- e:
	default:
		logger.Warn("[upgrade] event channel full, dropping %s event", e.Type)
	}
}

// readResult reads the result file into a Status, validates it, and on a new
// value caches it and emits an event. Unknown fields ride along verbatim in
// Status.Extra: the backend stays agnostic of the version format.
func (u *UpgradeBackend) readResult() {
	data, err := os.ReadFile(u.resultFile)
	if err != nil {
		logger.Debug("[upgrade] result file unavailable: %v", err)
		return
	}
	if bytes.Equal(data, u.lastRaw) {
		return // unchanged; avoid spurious events from repeated fsnotify writes
	}

	var status Status
	if err := json.Unmarshal(data, &status); err != nil {
		logger.Warn("[upgrade] result file invalid, ignoring: %v", err)
		return
	}

	u.lastRaw = append(u.lastRaw[:0], data...)
	u.cache.Set(statusKey, &status)
	logger.Info("[upgrade] result updated from %s, emitting %s", u.resultFile, events.TypeUpgradeInfo)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: &status})
}

// CheckNow triggers the configured detection unit and waits for it: it is a
// short oneshot, so on return the result file is up to date.
func (u *UpgradeBackend) CheckNow() error {
	if u.checkUnit == "" || u.systemd == nil {
		logger.Warn("[upgrade] check requested but no check unit available")
		return ErrUnitNotConfigured
	}
	logger.Info("[upgrade] triggering check unit %s", u.checkUnit)
	return u.systemd.StartService(u.checkUnit, systemd.ScopeUser)
}

// StartUpgrade triggers the configured upgrade unit without blocking. The run
// verdict (finished/success) is reported asynchronously from the unit's
// service.updated events on the bus (see onServiceEvent). Whether the unit needs
// privileges is the unit's concern, not the API's.
func (u *UpgradeBackend) StartUpgrade() error {
	if u.upgradeUnit == "" || u.systemd == nil {
		logger.Warn("[upgrade] start requested but no upgrade unit available")
		return ErrUnitNotConfigured
	}
	if !u.running.CompareAndSwap(false, true) {
		logger.Warn("[upgrade] start requested but an upgrade is already running")
		return ErrUpgradeInProgress
	}
	logger.Info("[upgrade] triggering upgrade unit %s, emitting running", u.upgradeUnit)
	if err := u.systemd.TriggerUserUnit(u.ctx, u.upgradeUnit); err != nil {
		u.running.Store(false)
		logger.Warn("[upgrade] failed to trigger upgrade unit: %v", err)
		return err
	}
	u.runState.Store(&RunState{State: "running"})
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: Progress{State: "running"}})
	return nil
}

// subscribeEvents subscribes to the bus for the run unit's service.updated
// events, then resumes tracking if an upgrade was already running before a
// restart. No-op when there is no unit or no bus (e.g. tests).
func (u *UpgradeBackend) subscribeEvents() {
	if u.upgradeUnit == "" || u.stream == nil {
		return
	}
	u.sub = u.stream.SubscribeFunc(func(e events.Event) bool {
		return e.Type == events.TypeServiceUpdated
	})
	u.wg.Add(1)
	go u.consumeEvents()
	u.resumeIfRunning()
}

// consumeEvents drains the subscription until shutdown. wg-tracked so Close waits
// for it before closing the event channel.
func (u *UpgradeBackend) consumeEvents() {
	defer u.wg.Done()
	for {
		select {
		case <-u.ctx.Done():
			return
		case e, ok := <-u.sub:
			if !ok {
				return
			}
			u.onServiceEvent(e)
		}
	}
}

// onServiceEvent emits the run verdict when the upgrade unit reaches a terminal
// state. The running guard limits this to a tracked run and fires finished
// exactly once (CAS true→false).
func (u *UpgradeBackend) onServiceEvent(e events.Event) {
	svc, ok := e.Data.(systemd.Service)
	if !ok || svc.Name != u.upgradeUnit || svc.Scope != systemd.ScopeUser {
		return
	}
	switch svc.ActiveState {
	case "active", "inactive", "failed": // terminal for a oneshot
	default:
		return // still activating
	}
	u.runState.Store(nil) // run no longer in flight; status endpoint reverts to detection
	if !u.running.CompareAndSwap(true, false) {
		return
	}
	success := svc.ActiveState != "failed"
	logger.Info("[upgrade] %s reached %s, emitting finished (success=%v)", u.upgradeUnit, svc.ActiveState, success)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: Progress{State: "finished", Success: &success}})
}

// resumeIfRunning re-attaches to an upgrade triggered before an odio-api restart:
// if the unit is still activating, re-announce running so reconnecting clients
// see it; completion then arrives through the bus like any live run. The startup
// snapshot is a single read, not a poll.
func (u *UpgradeBackend) resumeIfRunning() {
	if u.systemd == nil {
		return
	}
	ctx, cancel := context.WithTimeout(u.ctx, 5*time.Second)
	svc, err := u.systemd.RefreshService(ctx, u.upgradeUnit, systemd.ScopeUser)
	cancel()
	if err != nil {
		logger.Warn("[upgrade] cannot read %s state on startup: %v", u.upgradeUnit, err)
		return
	}
	if svc.ActiveState == "activating" && u.running.CompareAndSwap(false, true) {
		logger.Info("[upgrade] %s still running on startup, resuming", u.upgradeUnit)
		u.runState.Store(&RunState{State: "running"})
		u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: Progress{State: "running"}})
	}
}
