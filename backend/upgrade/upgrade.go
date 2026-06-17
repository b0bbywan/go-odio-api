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

	// Register as internal before Start, when the listener snapshots the units.
	if sysd != nil {
		sysd.AddInternalUserUnits(cfg.CheckUnit, cfg.UpgradeUnit)
	}

	return &UpgradeBackend{
		ctx:            ctx,
		resultFile:     cfg.ResultFile,
		checkUnit:      cfg.CheckUnit,
		upgradeUnit:    cfg.UpgradeUnit,
		progressSocket: cfg.ProgressSocket,
		systemd:        sysd,
		events:         make(chan events.Event, 16),
	}, nil
}

// UseEventStream wires the shared bus; called by Backend.New once the broadcaster exists.
func (u *UpgradeBackend) UseEventStream(s events.Stream) { u.stream = s }

// Start loads the current result, then watches the result file, listens for run
// progress, and tracks the run unit over the bus.
func (u *UpgradeBackend) Start() error {
	u.readResult() // best-effort; a missing file is not an error
	u.startWatcher()
	u.startListener()
	u.subscribeEvents()
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
		Run:        u.runState.Load(),
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
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
	return nil
}

// subscribeEvents tracks the run unit's service.updated events and resumes a run
// already in flight. No-op without a unit or bus (e.g. tests).
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
// state. The CAS guard fires finished exactly once per tracked run.
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
	u.runState.Reset()
	if !u.running.CompareAndSwap(true, false) {
		return
	}
	success := svc.ActiveState != "failed"
	logger.Info("[upgrade] %s reached %s, emitting finished (success=%v)", u.upgradeUnit, svc.ActiveState, success)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "finished", Success: &success}})
}

// resumeIfRunning re-attaches to an upgrade triggered before a restart: if the
// unit is still activating, re-announce running and let completion arrive over
// the bus like any live run.
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
		u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
	}
}
