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

	u := &UpgradeBackend{
		ctx:            ctx,
		resultFile:     cfg.ResultFile,
		checkUnit:      cfg.CheckUnit,
		upgradeUnit:    cfg.UpgradeUnit,
		progressSocket: cfg.ProgressSocket,
		run:            newRun(cfg.StateFile),
		events:         make(chan events.Event, 16),
	}
	// Assigning a nil *SystemdBackend through the interface would make u.systemd
	// non-nil; only set it for a real backend so the nil checks stay honest.
	if sysd != nil {
		// Register as internal before Start, when the listener snapshots the units.
		sysd.AddInternalUserUnits(cfg.CheckUnit, cfg.UpgradeUnit)
		u.systemd = sysd
	}
	logger.Info("[upgrade] configured: result=%s state=%q socket=%q check=%q upgrade=%q systemd=%t",
		cfg.ResultFile, cfg.StateFile, cfg.ProgressSocket, cfg.CheckUnit, cfg.UpgradeUnit, sysd != nil)
	return u, nil
}

// UseEventStream wires the shared bus; called by Backend.New once the broadcaster exists.
func (u *UpgradeBackend) UseEventStream(s events.Stream) { u.stream = s }

// Start loads the current result, then watches the result file, listens for run
// progress, and tracks the run unit over the bus.
func (u *UpgradeBackend) Start() error {
	u.run.load()   // restore a run persisted at the last shutdown before consumers read it
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
	u.run.save() // flush the final run snapshot for the next boot
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
	if !u.run.start(originSystemd) {
		logger.Warn("[upgrade] start requested but an upgrade is already running")
		return ErrUpgradeInProgress
	}
	logger.Info("[upgrade] triggering upgrade unit %s, emitting running", u.upgradeUnit)
	if err := u.systemd.TriggerUserUnit(u.ctx, u.upgradeUnit); err != nil {
		// The run started and failed to launch; record the verdict rather than
		// rewinding to idle, so the UI shows the failure.
		u.run.finish(false)
		logger.Warn("[upgrade] failed to trigger upgrade unit: %v", err)
		u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
		return err
	}
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
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

// onServiceEvent follows the upgrade unit, whoever started it (us, the admin
// app, a shell): activating claims the run for the unit, a terminal state
// closes it. A run the socket adopted outside the unit (a CLI upgrade) is
// closed by its "end" line instead; finish is idempotent, so the two terminal
// sources still emit finished exactly once.
func (u *UpgradeBackend) onServiceEvent(e events.Event) {
	svc, ok := e.Data.(systemd.Service)
	if !ok || svc.Name != u.upgradeUnit || svc.Scope != systemd.ScopeUser {
		return
	}
	switch svc.ActiveState {
	case "activating":
		if u.run.claim(originSystemd) {
			logger.Info("[upgrade] %s activating, emitting running", u.upgradeUnit)
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
		}
		return
	case "active", "inactive", "failed": // terminal for a oneshot
	default:
		return
	}
	success := svc.ActiveState != "failed"
	if u.run.isRunning() {
		if u.run.origin() == originSocket || !u.run.finish(success) {
			return
		}
	} else if success || !u.run.unitFailed() {
		return // nothing in flight, nothing new to say
	}
	logger.Info("[upgrade] %s reached %s, emitting finished (success=%v)", u.upgradeUnit, svc.ActiveState, success)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
}

// resumeIfRunning reconciles a restored or out-of-band run at startup by reading
// the unit: still activating means resume, as the unit's run; already terminal
// means a systemd run finished while we were down, so record its verdict. A
// socket run with no unit behind it waits for its CLI client to reconnect.
func (u *UpgradeBackend) resumeIfRunning() {
	if u.systemd == nil {
		return
	}
	socketRun := u.run.isRunning() && u.run.origin() == originSocket
	ctx, cancel := context.WithTimeout(u.ctx, 5*time.Second)
	svc, err := u.systemd.RefreshService(ctx, u.upgradeUnit, systemd.ScopeUser)
	cancel()
	switch {
	case err == nil && svc.ActiveState == "activating":
		// the unit's, whether restored as such, adopted from the socket, or new.
		u.run.claim(originSystemd)
		logger.Info("[upgrade] %s still running on startup, resuming", u.upgradeUnit)
		u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
	case socketRun:
		// a CLI run outside the unit: its client reconnects to end it.
		logger.Info("[upgrade] resuming socket run, awaiting client reconnect")
		u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
	case err != nil:
		logger.Warn("[upgrade] cannot read %s state on startup: %v", u.upgradeUnit, err)
	case u.run.isRunning():
		// restored running but the unit is terminal: it ended while we were down.
		if u.run.finish(svc.ActiveState != "failed") {
			logger.Info("[upgrade] %s ended while down (%s), recording verdict", u.upgradeUnit, svc.ActiveState)
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: u.run.snapshot()})
		}
	}
}
