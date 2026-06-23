package upgrade

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/events"
)

// fakeSystemd is a systemdControl that records calls and returns canned results,
// so the bus-driven paths run without a D-Bus connection.
type fakeSystemd struct {
	startErr     error
	triggerErr   error
	refreshSvc   *systemd.Service
	refreshErr   error
	startCalls   []string
	triggerCalls []string
}

func (f *fakeSystemd) StartService(name string, _ systemd.UnitScope) error {
	f.startCalls = append(f.startCalls, name)
	return f.startErr
}

func (f *fakeSystemd) TriggerUserUnit(_ context.Context, name string) error {
	f.triggerCalls = append(f.triggerCalls, name)
	return f.triggerErr
}

func (f *fakeSystemd) RefreshService(_ context.Context, _ string, _ systemd.UnitScope) (*systemd.Service, error) {
	return f.refreshSvc, f.refreshErr
}

func newBackend(t *testing.T, sysd systemdControl) *UpgradeBackend {
	t.Helper()
	return &UpgradeBackend{
		ctx:         context.Background(),
		checkUnit:   "odio-check.service",
		upgradeUnit: "odio-upgrade.service",
		run:         newRun(""),
		events:      make(chan events.Event, 16),
		systemd:     sysd,
	}
}

func recvRunState(t *testing.T, u *UpgradeBackend) RunState {
	t.Helper()
	e, ok := recv(t, u.Events(), time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info event, got none")
	}
	rs, ok := e.Data.(RunState)
	if !ok {
		t.Fatalf("event data = %T, want RunState", e.Data)
	}
	return rs
}

func TestStartUpgradeTriggersAndEmitsRunning(t *testing.T) {
	fake := &fakeSystemd{}
	u := newBackend(t, fake)

	if err := u.StartUpgrade(); err != nil {
		t.Fatalf("StartUpgrade = %v, want nil", err)
	}
	if len(fake.triggerCalls) != 1 || fake.triggerCalls[0] != "odio-upgrade.service" {
		t.Fatalf("triggerCalls = %v, want [odio-upgrade.service]", fake.triggerCalls)
	}
	if rs := recvRunState(t, u); rs.State != stateRunning || rs.Origin != originSystemd {
		t.Fatalf("event = %+v, want running/systemd", rs)
	}
}

// A trigger that fails records a failure verdict instead of rewinding to idle,
// so the UI shows the failure rather than a phantom-clean state.
func TestStartUpgradeTriggerFailureRecordsVerdict(t *testing.T) {
	wantErr := errors.New("dbus down")
	fake := &fakeSystemd{triggerErr: wantErr}
	u := newBackend(t, fake)

	if err := u.StartUpgrade(); !errors.Is(err, wantErr) {
		t.Fatalf("StartUpgrade = %v, want %v", err, wantErr)
	}
	rs := recvRunState(t, u)
	if rs.State != stateFailed {
		t.Fatalf("event = %+v, want failed", rs)
	}
	if u.run.isRunning() {
		t.Error("run still marked running after a failed trigger")
	}
}

func TestCheckNowTriggersCheckUnit(t *testing.T) {
	fake := &fakeSystemd{}
	u := newBackend(t, fake)

	if err := u.CheckNow(); err != nil {
		t.Fatalf("CheckNow = %v, want nil", err)
	}
	if len(fake.startCalls) != 1 || fake.startCalls[0] != "odio-check.service" {
		t.Fatalf("startCalls = %v, want [odio-check.service]", fake.startCalls)
	}
}

// resumeIfRunning on a unit still activating claims the run and re-announces it.
func TestResumeIfRunningActivatingResumes(t *testing.T) {
	fake := &fakeSystemd{refreshSvc: &systemd.Service{ActiveState: "activating"}}
	u := newBackend(t, fake)

	u.resumeIfRunning()
	if rs := recvRunState(t, u); rs.State != stateRunning || rs.Origin != originSystemd {
		t.Fatalf("event = %+v, want running/systemd", rs)
	}
}

// A run restored as running whose unit is already terminal ended while we were
// down: resume records the missed verdict.
func TestResumeIfRunningTerminalWhileDownRecordsVerdict(t *testing.T) {
	fake := &fakeSystemd{refreshSvc: &systemd.Service{ActiveState: "failed"}}
	u := newBackend(t, fake)
	u.run.start(originSystemd) // restored from a persisted snapshot

	u.resumeIfRunning()
	rs := recvRunState(t, u)
	if rs.State != stateFailed {
		t.Fatalf("event = %+v, want failed", rs)
	}
}

// A socket-driven run owns its lifecycle, so the unit reaching a terminal state
// must not close it.
func TestOnServiceEventIgnoresSocketRun(t *testing.T) {
	u := newBackend(t, &fakeSystemd{})
	u.run.start(originSocket)

	u.onServiceEvent(events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "failed"},
	})
	if _, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatal("a socket run must not be closed by the unit's terminal state")
	}
	if !u.run.isRunning() {
		t.Error("socket run should still be running")
	}
}
