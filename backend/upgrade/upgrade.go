package upgrade

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"

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
		ctx:         ctx,
		resultFile:  cfg.ResultFile,
		checkUnit:   cfg.CheckUnit,
		upgradeUnit: cfg.UpgradeUnit,
		systemd:     sysd,
		cache:       cache.New[json.RawMessage](0), // TTL=0 = no expiration
		events:      make(chan events.Event, 16),
	}, nil
}

// Start reads the current result file and starts watching it for changes.
func (u *UpgradeBackend) Start() error {
	u.readResult() // best-effort; a missing file is not an error
	u.startWatcher()
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
	u.wg.Wait()
	close(u.events)
	u.watcher = nil
}

// Events returns the read-only event channel for this backend.
func (u *UpgradeBackend) Events() <-chan events.Event { return u.events }

// GetStatus returns the last valid result file content, or nil if none.
func (u *UpgradeBackend) GetStatus() json.RawMessage {
	status, _ := u.cache.Get(statusKey)
	return status
}

func (u *UpgradeBackend) notify(e events.Event) {
	select {
	case u.events <- e:
	default:
		logger.Warn("[upgrade] event channel full, dropping %s event", e.Type)
	}
}

// readResult reads the result file, validates the required fields, and on a
// new value caches it and emits an event. The remaining fields are passed
// through verbatim: the backend stays agnostic of the version format.
func (u *UpgradeBackend) readResult() {
	data, err := os.ReadFile(u.resultFile)
	if err != nil {
		logger.Debug("[upgrade] result file unavailable: %v", err)
		return
	}

	// Required contract; typed pointers reject both absence and wrong type.
	var required struct {
		Current          *string `json:"current"`
		Latest           *string `json:"latest"`
		UpgradeAvailable *bool   `json:"upgrade_available"`
	}
	if err := json.Unmarshal(data, &required); err != nil {
		logger.Warn("[upgrade] result file is not valid JSON, ignoring: %v", err)
		return
	}
	if required.Current == nil || required.Latest == nil || required.UpgradeAvailable == nil {
		logger.Warn("[upgrade] result file missing required fields, ignoring")
		return
	}

	raw := json.RawMessage(data)
	if prev, ok := u.cache.Get(statusKey); ok && bytes.Equal(prev, raw) {
		return // unchanged; avoid spurious events from repeated fsnotify writes
	}
	u.cache.Set(statusKey, raw)
	logger.Info("[upgrade] result updated from %s, emitting %s", u.resultFile, events.TypeUpgradeInfo)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: raw})
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

// StartUpgrade triggers the configured upgrade unit and tracks it in the
// background: the request returns immediately while a goroutine waits for the
// oneshot to finish and emits the run lifecycle. Whether the unit needs
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
	// Emit the start synchronously so it is tied to the accepted trigger,
	// before the background wait begins.
	logger.Info("[upgrade] starting upgrade unit %s, emitting running", u.upgradeUnit)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: Progress{State: "running"}})
	u.wg.Add(1)
	go u.runUpgrade()
	return nil
}

// runUpgrade waits for the upgrade oneshot to complete and emits the verdict.
// A ctx cancellation (shutdown) exits quietly without a verdict.
func (u *UpgradeBackend) runUpgrade() {
	defer u.wg.Done()
	defer u.running.Store(false)

	result, err := u.systemd.StartUserServiceWait(u.ctx, u.upgradeUnit)
	if errors.Is(err, context.Canceled) {
		logger.Info("[upgrade] upgrade wait cancelled (shutdown)")
		return
	}
	if err != nil {
		logger.Warn("[upgrade] upgrade run failed to start: %v", err)
	}
	success := err == nil && result == "done"
	logger.Info("[upgrade] upgrade finished (result=%q success=%v), emitting finished", result, success)
	u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: Progress{State: "finished", Success: &success}})
}
