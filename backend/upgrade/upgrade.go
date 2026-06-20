package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New returns nil when the backend is disabled or has no result file configured.
func New(ctx context.Context, cfg *config.UpgradeConfig, sysd *systemd.SystemdBackend) (*UpgradeBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}
	if cfg.ResultFile == "" {
		logger.Warn("[upgrade] enabled but no resultFile configured, disabling backend")
		return nil, nil
	}

	b := &UpgradeBackend{
		ctx:            ctx,
		resultFile:     cfg.ResultFile,
		checkUnit:      cfg.CheckUnit,
		upgradeUnit:    cfg.UpgradeUnit,
		progressSocket: cfg.ProgressSocket,
		stateFile:      cfg.StateFile,
		events:         make(chan events.Event, 16),
	}
	// Assign only when present: a typed-nil *SystemdBackend stored in the interface field would
	// read as non-nil and defeat every `u.systemd == nil` guard.
	if sysd != nil {
		// Register as internal before Start, when the listener snapshots the units.
		sysd.AddInternalUserUnits(cfg.CheckUnit, cfg.UpgradeUnit)
		b.systemd = sysd
	}
	return b, nil
}

// UseEventStream wires the shared bus; called by Backend.New once the broadcaster exists.
func (u *UpgradeBackend) UseEventStream(s events.Stream) { u.stream = s }

// Start loads the current result, then watches the result file, listens for run
// progress, and tracks the run unit over the bus.
func (u *UpgradeBackend) Start() error {
	u.readResult() // best-effort; a missing file is not an error
	u.readState()  // restore the last run verdict across restarts
	u.startWatcher()
	u.startListener()
	u.subscribeEvents()
	u.resumeIfRunning()
	logger.Info("[upgrade] backend started successfully")
	return nil
}

// Close stops the watcher, progress listener and bus subscription, then waits for
// their goroutines. Consumers also exit on the cancelled ctx.
func (u *UpgradeBackend) Close() {
	if u.watcher != nil {
		if err := u.watcher.Close(); err != nil {
			logger.Warn("[upgrade] closing watcher: %v", err)
		}
	}
	if u.listener != nil {
		if err := u.listener.Close(); err != nil {
			logger.Warn("[upgrade] closing progress listener: %v", err)
		}
	}
	if u.sub != nil && u.stream != nil {
		u.stream.Unsubscribe(u.sub)
	}
	u.wg.Wait()
	// Snapshot only after the goroutines stop: a run finishing concurrently would otherwise race
	// recordRun's verdict write and could clobber it with a stale in-flight snapshot.
	u.persistInFlight()
	u.watcher = nil
	u.listener = nil
}

// Events returns the read-only event channel for this backend.
func (u *UpgradeBackend) Events() <-chan events.Event { return u.events }

// GetStatus returns the last valid detector result, or nil if none.
func (u *UpgradeBackend) GetStatus() *Status {
	return u.status.Load()
}

// StatusResponse builds the GET /upgrade payload (always non-nil).
func (u *UpgradeBackend) StatusResponse() StatusResponse {
	resp := StatusResponse{
		Run:        u.run.snapshot(),
		LastRun:    u.lastRun.Load(),
		CanCheck:   u.CanCheck(),
		CanUpgrade: u.CanUpgrade(),
	}
	if status := u.GetStatus(); status != nil {
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

// readResult loads the result file into a Status and, when it changed, caches it
// and emits an event. Unknown fields stay in Status.Extra, keeping the backend
// agnostic of the version format.
func (u *UpgradeBackend) readResult() {
	data, err := os.ReadFile(u.resultFile)
	if err != nil {
		logger.Debug("[upgrade] result file unavailable: %v", err)
		return
	}
	if bytes.Equal(data, u.lastRaw) {
		return // unchanged
	}

	var status Status
	if err := json.NewDecoder(bytes.NewReader(data)).Decode(&status); err != nil {
		// An empty or truncated file is a mid-write read; ignore and wait for the
		// next event rather than treating it as corruption.
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			logger.Debug("[upgrade] result file empty or partial, ignoring: %v", err)
		} else {
			logger.Warn("[upgrade] result file invalid, ignoring: %v", err)
		}
		return
	}

	u.lastRaw = append(u.lastRaw[:0], data...)
	u.status.Store(&status)
	logger.Info("[upgrade] result updated from %s, emitting %s", u.resultFile, events.TypeUpgradeInfo)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: &status})
}

// readState restores the persisted last-run verdict and any in-flight snapshot; a missing
// or invalid file is not an error. The snapshot is held as a hint for resumeIfRunning.
func (u *UpgradeBackend) readState() {
	if u.stateFile == "" {
		return
	}
	data, err := os.ReadFile(u.stateFile)
	if err != nil {
		logger.Debug("[upgrade] run state file unavailable: %v", err)
		return
	}
	var ps persistedState
	if err := json.Unmarshal(data, &ps); err != nil {
		logger.Warn("[upgrade] run state file invalid, ignoring: %v", err)
		return
	}
	if ps.LastRun != nil {
		u.lastRun.Store(ps.LastRun)
	}
	u.resumeHint = ps.InFlight
}

// persist writes the state file best-effort; failures are logged, never fatal.
func (u *UpgradeBackend) persist(ps persistedState) {
	if u.stateFile == "" {
		return
	}
	if err := writeJSONAtomic(u.stateFile, ps); err != nil {
		logger.Warn("[upgrade] cannot persist run state to %s: %v", u.stateFile, err)
	}
}

// persistInFlight snapshots a running unit upgrade on graceful shutdown so the next start can
// resume the badge ring at the right percent (the self-upgrade case). Only unit runs are
// persisted: the systemd unit is a queryable source of truth across a restart, so resume can
// tell "still running" from "finished while we were down". A CLI run has no such anchor — it
// resumes on the script's next progress line, and one that ends during downtime is missed.
func (u *UpgradeBackend) persistInFlight() {
	st, src := u.run.inflight()
	if st == nil || src != sourceUnit {
		return
	}
	logger.Info("[upgrade] persisting in-flight run on shutdown")
	u.persist(persistedState{LastRun: u.lastRun.Load(), InFlight: st})
}

// recordRun caches the run verdict, persists it best-effort, and emits the finished event.
func (u *UpgradeBackend) recordRun(lr *LastRun) {
	u.lastRun.Store(lr)
	u.persist(persistedState{LastRun: lr})
	success := lr.Success
	fin := RunState{State: "finished", Success: &success}
	if lr.Step != "" {
		fin.Step = &lr.Step
	}
	if lr.Error != "" {
		fin.Error = &lr.Error
	}
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: fin})
}

// CanCheck reports whether the check trigger is available: its unit is
// configured and a systemd backend is present to run it.
func (u *UpgradeBackend) CanCheck() bool { return u.checkUnit != "" && u.systemd != nil }

// CanUpgrade reports whether the upgrade trigger is available.
func (u *UpgradeBackend) CanUpgrade() bool { return u.upgradeUnit != "" && u.systemd != nil }

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

// StartUpgrade triggers the upgrade unit without blocking; the run verdict
// arrives asynchronously via its service.updated events (see onServiceEvent).
func (u *UpgradeBackend) StartUpgrade() error {
	if u.upgradeUnit == "" || u.systemd == nil {
		logger.Warn("[upgrade] start requested but no upgrade unit available")
		return ErrUnitNotConfigured
	}
	if !u.run.start(sourceUnit) {
		logger.Warn("[upgrade] start requested but an upgrade is already running")
		return ErrUpgradeInProgress
	}
	logger.Info("[upgrade] triggering upgrade unit %s, emitting running", u.upgradeUnit)
	if err := u.systemd.TriggerUserUnit(u.ctx, u.upgradeUnit); err != nil {
		u.run.finish(sourceUnit, false) // never started; just clear the run
		logger.Warn("[upgrade] failed to trigger upgrade unit: %v", err)
		return err
	}
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
	return nil
}

// subscribeEvents tracks the run unit's service.updated events. No-op without a unit or bus
// (e.g. tests). resumeIfRunning runs separately in Start, as it also covers unit-less runs.
func (u *UpgradeBackend) subscribeEvents() {
	if u.upgradeUnit == "" || u.stream == nil {
		return
	}
	u.sub = u.stream.SubscribeFunc(func(e events.Event) bool {
		return e.Type == events.TypeServiceUpdated
	})
	u.wg.Add(1)
	go u.consumeEvents()
}

// consumeEvents drains the subscription until shutdown.
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

// onServiceEvent emits the run verdict once the upgrade unit reaches a terminal
// state, but only for a unit-owned run (finish rejects a stream-owned one).
func (u *UpgradeBackend) onServiceEvent(e events.Event) {
	svc, ok := e.Data.(systemd.Service)
	if !ok || svc.Name != u.upgradeUnit || svc.Scope != systemd.ScopeUser {
		return
	}
	prev := u.unitState
	u.unitState = svc.ActiveState
	switch svc.ActiveState {
	case "active", "inactive", "failed": // terminal for a oneshot
	default:
		return // still activating
	}
	// Only act on the transition INTO a terminal state: a stale or repeated terminal event (the
	// unit was already terminal) must not finish a newer, unrelated run — e.g. a failed unit's
	// duplicate event killing a CLI run started after it.
	if prev == "active" || prev == "inactive" || prev == "failed" {
		return
	}
	// A failed unit is authoritative: a run killed mid-step (status 143) sends no end line, so
	// finish it whoever owns it. A success only finishes a unit-owned run; a stream run reports
	// its own success through the end line on the socket.
	var lr *LastRun
	if svc.ActiveState == "failed" {
		lr = u.run.finishAny(false)
	} else {
		lr = u.run.finish(sourceUnit, true)
	}
	if lr == nil {
		return
	}
	logger.Info("[upgrade] %s reached %s, emitting finished (success=%v)", u.upgradeUnit, svc.ActiveState, lr.Success)
	u.recordRun(lr)
}

// resumeIfRunning re-attaches to a unit upgrade that crossed our restart, using the systemd unit
// as the source of truth — never a fabricated verdict from the absence of one:
//   - still activating: a run is genuinely in progress (the self-upgrade restarted us mid-playbook,
//     possibly killed without a snapshot), so resume running and let the bus finish it;
//   - terminal AND we held a snapshot: it finished while we were down, emit that verdict;
//   - unreadable (transient D-Bus error on startup): say nothing — "can't tell yet" is not "failed".
//
// The hint seeds the resumed percent but is not required to re-attach an activating unit, so a
// non-graceful kill (no snapshot) is still recovered. A terminal unit without a hint is some prior,
// already-recorded run — left alone.
func (u *UpgradeBackend) resumeIfRunning() {
	hint := u.resumeHint
	u.resumeHint = nil
	if u.systemd == nil || u.upgradeUnit == "" {
		return // no queryable unit; a CLI run resumes on its next progress line
	}

	ctx, cancel := context.WithTimeout(u.ctx, 5*time.Second)
	svc, err := u.systemd.RefreshService(ctx, u.upgradeUnit, systemd.ScopeUser)
	cancel()
	if err != nil {
		logger.Warn("[upgrade] cannot read %s state on startup, not resuming: %v", u.upgradeUnit, err)
		return
	}

	if svc.ActiveState == "activating" {
		if u.run.resume(sourceUnit, hint) {
			logger.Info("[upgrade] %s still running on startup, resuming", u.upgradeUnit)
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
		}
		return
	}

	if hint != nil && u.run.resume(sourceUnit, hint) {
		success := svc.ActiveState != "failed"
		logger.Info("[upgrade] %s finished during downtime (%s), emitting verdict", u.upgradeUnit, svc.ActiveState)
		if lr := u.run.finish(sourceUnit, success); lr != nil {
			u.recordRun(lr)
		}
	}
}
