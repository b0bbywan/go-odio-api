package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

// fakeStream is a minimal events.Stream: the test pushes events onto ch, which
// the backend reads through its subscription.
type fakeStream struct{ ch chan events.Event }

func (f *fakeStream) SubscribeFunc(func(events.Event) bool) chan events.Event { return f.ch }
func (f *fakeStream) Unsubscribe(ch chan events.Event)                        { close(ch) }

// fakeUnits stands in for the systemd backend: RefreshService returns a canned ActiveState
// (or error), and the trigger methods record the unit they were called with and return a
// canned error — so the lifecycle can be driven without a real D-Bus connection.
type fakeUnits struct {
	state      string
	refreshErr error
	startErr   error
	triggerErr error
	started    []string // units passed to StartService
	triggered  []string // units passed to TriggerUserUnit
}

func (f *fakeUnits) StartService(name string, _ systemd.UnitScope) error {
	f.started = append(f.started, name)
	return f.startErr
}

func (f *fakeUnits) TriggerUserUnit(_ context.Context, name string) error {
	f.triggered = append(f.triggered, name)
	return f.triggerErr
}

func (f *fakeUnits) RefreshService(_ context.Context, name string, scope systemd.UnitScope) (*systemd.Service, error) {
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return &systemd.Service{Name: name, Scope: scope, ActiveState: f.state}, nil
}

const validResult = `{"current":"dev","latest":"2026.6.0b1","upgrade_available":true}`

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// atomicWrite mimics how a detector publishes the file: write a temp then
// rename over the target, which replaces the inode (the case the dir watch
// must catch).
func atomicWrite(t *testing.T, path, data string) {
	t.Helper()
	tmp := path + ".tmp"
	writeFile(t, tmp, data)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename %s: %v", tmp, err)
	}
}

func ptr[T any](v T) *T { return &v }

func recv(t *testing.T, ch <-chan events.Event, d time.Duration) (events.Event, bool) {
	t.Helper()
	select {
	case e := <-ch:
		return e, true
	case <-time.After(d):
		return events.Event{}, false
	}
}

func mustWrite(t *testing.T, conn net.Conn, line string) {
	t.Helper()
	if _, err := conn.Write([]byte(line + "\n")); err != nil {
		t.Fatalf("write %q: %v", line, err)
	}
}

// waitFinished drains events until a finished verdict appears, returning it.
func waitFinished(t *testing.T, ch <-chan events.Event, d time.Duration) RunState {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case e := <-ch:
			if rs, ok := e.Data.(RunState); ok && e.Type == events.TypeUpgradeInfo && rs.State == "finished" {
				return rs
			}
		case <-deadline:
			t.Fatal("no finished verdict before timeout")
			return RunState{}
		}
	}
}

// drain reads and discards events for d, so a wrongly-processed line has time to land.
func drain(t *testing.T, ch <-chan events.Event, d time.Duration) {
	t.Helper()
	deadline := time.After(d)
	for {
		select {
		case <-ch:
		case <-deadline:
			return
		}
	}
}

// newStarted returns a started backend pointed at a fresh result file path,
// and drains the event emitted by the initial read (when the file pre-exists).
func newStarted(t *testing.T, initial string) (*UpgradeBackend, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "upgrades.json")
	if initial != "" {
		writeFile(t, path, initial)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{Enabled: true, ResultFile: path}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u == nil {
		t.Fatal("New returned nil for an enabled config")
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if initial != "" {
		recv(t, u.Events(), time.Second) // drain initial-read event
	}
	return u, path
}

func TestNewDisabled(t *testing.T) {
	ctx := context.Background()
	cases := map[string]*config.UpgradeConfig{
		"nil":            nil,
		"disabled":       {Enabled: false, ResultFile: "/x"},
		"no result file": {Enabled: true, ResultFile: ""},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			u, err := New(ctx, cfg, nil)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if u != nil {
				t.Fatalf("expected nil backend, got %#v", u)
			}
		})
	}
}

func TestInitialReadExposesStatus(t *testing.T) {
	u, _ := newStarted(t, validResult)
	got := u.GetStatus()
	if got == nil {
		t.Fatal("GetStatus = nil, want a status")
	}
	if got.Current != "dev" || got.Latest != "2026.6.0b1" || !got.UpgradeAvailable {
		t.Fatalf("GetStatus = %+v, want current=dev latest=2026.6.0b1 upgrade_available=true", got)
	}
}

// TestStatusKeepsExtraFields verifies the detector's free fields (roles,
// manifest, …) survive the round trip verbatim under Extra.
func TestStatusKeepsExtraFields(t *testing.T) {
	const result = `{"current":"dev","latest":"2026.7.0","upgrade_available":false,` +
		`"roles":["common","odio_api"],"manifest":{"odios":"2026.7.0"}}`
	u, _ := newStarted(t, result)

	got := u.GetStatus()
	if got == nil {
		t.Fatal("GetStatus = nil")
	}
	if string(got.Extra["roles"]) != `["common","odio_api"]` {
		t.Errorf("Extra[roles] = %s, want verbatim array", got.Extra["roles"])
	}
	if string(got.Extra["manifest"]) != `{"odios":"2026.7.0"}` {
		t.Errorf("Extra[manifest] = %s, want verbatim object", got.Extra["manifest"])
	}
}

func TestWatchEmitsOnAtomicRewrite(t *testing.T) {
	u, path := newStarted(t, validResult)

	updated := `{"current":"dev","latest":"2026.7.0","upgrade_available":true}`
	atomicWrite(t, path, updated)

	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info event after rewrite, got none")
	}
	if e.Type != events.TypeUpgradeInfo {
		t.Fatalf("event type = %q, want %q", e.Type, events.TypeUpgradeInfo)
	}
	if got := u.GetStatus(); got == nil || got.Latest != "2026.7.0" {
		t.Fatalf("GetStatus = %+v, want latest=2026.7.0", got)
	}
}

func TestInvalidResultKeepsLastValid(t *testing.T) {
	u, path := newStarted(t, validResult)

	// Missing required "latest" field → must be rejected.
	atomicWrite(t, path, `{"current":"dev","upgrade_available":true}`)

	if _, ok := recv(t, u.Events(), 500*time.Millisecond); ok {
		t.Fatal("invalid result should not emit an event")
	}
	if got := u.GetStatus(); got == nil || got.Latest != "2026.6.0b1" {
		t.Fatalf("GetStatus = %+v, want last valid latest=2026.6.0b1", got)
	}
}

// TestStatusResponseShape asserts the GET /upgrade payload: contract fields
// flat at the top, free fields under "extra", and "run" added only while an
// upgrade is in flight.
func TestStatusResponseShape(t *testing.T) {
	const result = `{"current":"dev","latest":"2026.7.0","upgrade_available":true,` +
		`"checked_at":"2026-06-15T20:46:34Z","roles":["common"]}`
	u, _ := newStarted(t, result)

	marshal := func() map[string]json.RawMessage {
		t.Helper()
		b, err := json.Marshal(u.StatusResponse())
		if err != nil {
			t.Fatalf("marshal StatusResponse: %v", err)
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(b, &m); err != nil {
			t.Fatalf("response not an object: %v\n%s", err, b)
		}
		return m
	}

	// Idle: contract fields flat, roles under "extra", no "run".
	m := marshal()
	if string(m["latest"]) != `"2026.7.0"` {
		t.Errorf("latest = %s, want flat top-level", m["latest"])
	}
	if _, ok := m["run"]; ok {
		t.Errorf("idle response should have no run, got %s", m["run"])
	}
	var extra map[string]json.RawMessage
	if err := json.Unmarshal(m["extra"], &extra); err != nil {
		t.Fatalf("extra not an object: %v", err)
	}
	if string(extra["roles"]) != `["common"]` {
		t.Errorf("extra.roles = %s, want verbatim", extra["roles"])
	}

	// In flight: "run" appears alongside the flat fields.
	pct := 50
	u.run.progress(sourceStream, &pct, nil)
	var run RunState
	if err := json.Unmarshal(marshal()["run"], &run); err != nil {
		t.Fatalf("run missing/undecodable: %v", err)
	}
	if run.State != "running" || run.Percent == nil || *run.Percent != 50 {
		t.Errorf("run = %+v, want running 50%%", run)
	}
}

func TestProgressSocketRelaysLines(t *testing.T) {
	dir := t.TempDir()
	// A subdir that does not exist yet: startListener must create it (like the
	// default $XDG_RUNTIME_DIR/odio-api).
	sock := filepath.Join(dir, "odio-api", "upgrade.sock")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:        true,
		ResultFile:     filepath.Join(dir, "upgrades.json"),
		ProgressSocket: sock,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial progress socket: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("closing conn: %v", err)
		}
	}()

	// Missing the required current/step → must be rejected, no event.
	if _, err := conn.Write([]byte(`{"event":"progress","percent":10}` + "\n")); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	if _, ok := recv(t, u.Events(), 300*time.Millisecond); ok {
		t.Fatal("malformed progress line should not emit an event")
	}

	// Valid event with an extra ansible-flavoured field, which must pass through.
	progress := `{"event":"progress","percent":42,"current":1,"step":"mpd","changed":3}`
	if _, err := conn.Write([]byte(progress + "\n")); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	// First running line on an idle backend (no systemd) re-announces running.
	if e, ok := recv(t, u.Events(), 2*time.Second); !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("got (%q, %v), want a leading %s event", e.Type, ok, events.TypeUpgradeInfo)
	}
	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.progress event from the socket, got none")
	}
	if e.Type != events.TypeUpgradeProgress {
		t.Fatalf("event type = %q, want %q", e.Type, events.TypeUpgradeProgress)
	}
	if got := string(e.Data.(json.RawMessage)); got != progress {
		t.Fatalf("event data = %q, want verbatim %q", got, progress)
	}
}

// Without systemd, begin/end each emit an upgrade.info lifecycle event on top of the raw progress relay.
func TestProgressStreamDrivesLifecycleWithoutSystemd(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "upgrade.sock")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:        true,
		ResultFile:     filepath.Join(dir, "upgrades.json"),
		ProgressSocket: sock,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial progress socket: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("closing conn: %v", err)
		}
	}()

	// begin → upgrade.info{running} then the raw upgrade.progress relay.
	if _, err := conn.Write([]byte(`{"event":"begin","total":3}` + "\n")); err != nil {
		t.Fatalf("write begin: %v", err)
	}
	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("begin: got (%q, %v), want an %s event", e.Type, ok, events.TypeUpgradeInfo)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("begin info state = %q, want running", run.State)
	}
	if e, ok := recv(t, u.Events(), 2*time.Second); !ok || e.Type != events.TypeUpgradeProgress {
		t.Fatalf("begin: got (%q, %v), want an %s relay", e.Type, ok, events.TypeUpgradeProgress)
	}

	// end → upgrade.info{finished} carrying success, then the raw relay.
	if _, err := conn.Write([]byte(`{"event":"end","success":true}` + "\n")); err != nil {
		t.Fatalf("write end: %v", err)
	}
	e, ok = recv(t, u.Events(), 2*time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("end: got (%q, %v), want an %s event", e.Type, ok, events.TypeUpgradeInfo)
	}
	run, _ := e.Data.(RunState)
	if run.State != "finished" || run.Success == nil || !*run.Success {
		t.Fatalf("end info = %+v, want finished success=true", run)
	}
	if u.run.snapshot() != nil {
		t.Fatalf("run after end = %+v, want nil (cleared)", u.run.snapshot())
	}
}

// newSocketBackend starts a backend listening on a fresh progress socket and returns it.
func newSocketBackend(t *testing.T) (*UpgradeBackend, string) {
	t.Helper()
	dir := t.TempDir()
	sock := filepath.Join(dir, "upgrade.sock")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:        true,
		ResultFile:     filepath.Join(dir, "upgrades.json"),
		ProgressSocket: sock,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	return u, sock
}

// A trailing line after 'end' on the same connection must not re-open the finished run: the
// listener stops reading once a run ends, so a buffered late 'progress' can't resurrect it.
func TestTrailingLineAfterEndDoesNotResurrectRun(t *testing.T) {
	u, sock := newSocketBackend(t)
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	mustWrite(t, conn, `{"event":"begin","total":1}`)
	mustWrite(t, conn, `{"event":"end","success":true}`)
	mustWrite(t, conn, `{"event":"progress","percent":50,"current":1,"step":"late"}`)

	if v := waitFinished(t, u.Events(), 2*time.Second); v.Success == nil || !*v.Success {
		t.Fatalf("verdict = %+v, want success", v)
	}
	drain(t, u.Events(), 300*time.Millisecond)
	if u.run.snapshot() != nil {
		t.Fatalf("trailing line resurrected the run: %+v, want idle", u.run.snapshot())
	}
}

// A dropped connection is not the run's end: the run stays tracked with no verdict, because the
// script reconnects and resends its tail (notably end).
func TestStreamRunSurvivesDisconnectWithoutEnd(t *testing.T) {
	u, sock := newSocketBackend(t)
	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	mustWrite(t, conn, `{"event":"progress","percent":40,"current":1,"step":"mpd"}`)
	for i := 0; i < 2; i++ { // progress emits info{running} then the raw relay
		if _, ok := recv(t, u.Events(), 2*time.Second); !ok {
			t.Fatalf("setup: missing progress event %d", i)
		}
	}
	if err := conn.Close(); err != nil { // drop the connection without sending end
		t.Fatalf("close: %v", err)
	}

	if e, ok := recv(t, u.Events(), 300*time.Millisecond); ok {
		t.Fatalf("disconnect emitted %q, want nothing (run still pending)", e.Type)
	}
	if st := u.run.snapshot(); st == nil || st.State != "running" {
		t.Fatalf("run after disconnect = %+v, want still running", st)
	}
}

// A CLI reconnecting after an odio-api restart resumes mid-stream with a progress
// line (no begin); the idle→running edge must still re-announce running.
func TestProgressResumesRunningOnReconnect(t *testing.T) {
	u := &UpgradeBackend{events: make(chan events.Event, 4)}

	step := "mpd"
	pct, cur := 42, 1
	u.applyRunProgress(progressLine{Event: ptr("progress"), Percent: &pct, Current: &cur, Step: &step})

	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("resume: got (%q, %v), want an %s event", e.Type, ok, events.TypeUpgradeInfo)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("resume info state = %q, want running", run.State)
	}

	// A second progress line stays on the running edge: no further info event.
	u.applyRunProgress(progressLine{Event: ptr("progress"), Percent: &pct, Current: &cur, Step: &step})
	if e, ok := recv(t, u.Events(), 300*time.Millisecond); ok {
		t.Fatalf("second progress line emitted %q, want no info event", e.Type)
	}
}

func TestBusTerminalStateEmitsFinished(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrades.json")
	writeFile(t, path, validResult)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:     true,
		ResultFile:  path,
		UpgradeUnit: "odio-upgrade.service",
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stream := &fakeStream{ch: make(chan events.Event, 8)}
	u.UseEventStream(stream)
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	recv(t, u.Events(), time.Second) // drain the initial result-read event

	// Simulate an in-progress run, then its unit reaching a terminal success state.
	u.run.start(sourceUnit)
	stream.ch <- events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "inactive"},
	}

	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info finished event, got none")
	}
	prog, ok := e.Data.(RunState)
	if !ok {
		t.Fatalf("event data = %T, want RunState", e.Data)
	}
	if prog.State != "finished" || prog.Success == nil || !*prog.Success {
		t.Fatalf("got %+v, want state=finished success=true", prog)
	}
}

// The failure counterpart of TestBusTerminalStateEmitsFinished: a unit-owned run whose unit
// the bus reports as failed yields a failure verdict.
func TestBusFailedStateFinishesUnitRun(t *testing.T) {
	u := &UpgradeBackend{events: make(chan events.Event, 8), upgradeUnit: "odio-upgrade.service"}
	u.run.start(sourceUnit)

	u.onServiceEvent(events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "failed"},
	})

	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a finished event, got (%q, %v)", e.Type, ok)
	}
	if run, _ := e.Data.(RunState); run.State != "finished" || run.Success == nil || *run.Success {
		t.Fatalf("verdict = %+v, want finished success=false", e.Data)
	}
	if u.run.snapshot() != nil {
		t.Fatalf("run still tracked: %+v, want idle", u.run.snapshot())
	}
}

// finishAny on a failed unit ignores ownership, so a stale/repeated terminal event must not
// finish a newer, unrelated run. onServiceEvent only acts on the transition into a terminal state.
func TestStaleBusFailedDoesNotFinishNewerRun(t *testing.T) {
	u := &UpgradeBackend{events: make(chan events.Event, 8), upgradeUnit: "odio-upgrade.service"}
	failedEvt := events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "failed"},
	}

	// Run A (unit-owned) fails and is recorded.
	u.run.start(sourceUnit)
	u.onServiceEvent(failedEvt)
	if e, ok := recv(t, u.Events(), time.Second); !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("setup: want A's finished verdict, got (%q, %v)", e.Type, ok)
	}

	// A new CLI run B starts (out of band, stream-owned).
	pct := 60
	u.run.progress(sourceStream, &pct, ptr("late-run"))
	recv(t, u.Events(), time.Second) // drain B's running

	// A stale/duplicate failed event for the still-terminal unit must not touch B.
	u.onServiceEvent(failedEvt)
	if e, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatalf("stale failed event emitted %q, want run B untouched", e.Type)
	}
	if st, src := u.run.inflight(); st == nil || src != sourceStream {
		t.Fatalf("run B = (%+v, %v), want still stream-owned and running", st, src)
	}
}

// onServiceEvent acts only on a terminal event for its own user unit: a different unit, a
// system-scope event, or a still-activating state must leave the run untouched.
func TestBusEventFilteredByUnitScopeAndState(t *testing.T) {
	cases := map[string]systemd.Service{
		"other unit":       {Name: "other.service", Scope: systemd.ScopeUser, ActiveState: "failed"},
		"system scope":     {Name: "odio-upgrade.service", Scope: systemd.ScopeSystem, ActiveState: "failed"},
		"still activating": {Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "activating"},
	}
	for name, svc := range cases {
		t.Run(name, func(t *testing.T) {
			u := &UpgradeBackend{events: make(chan events.Event, 4), upgradeUnit: "odio-upgrade.service"}
			u.run.start(sourceUnit)

			u.onServiceEvent(events.Event{Type: events.TypeServiceUpdated, Data: svc})

			if e, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
				t.Fatalf("event emitted %q, want none (filtered out)", e.Type)
			}
			if u.run.snapshot() == nil {
				t.Fatal("run cleared, want still in flight")
			}
		})
	}
}

// With systemd present, an upgrade launched out of band (CLI, no StartUpgrade) is still
// driven by the progress stream, and a bus terminal event for the unit must not finish it.
func TestStreamDrivesCLIRunWithSystemdPresent(t *testing.T) {
	u := &UpgradeBackend{
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &systemd.SystemdBackend{}, // present, but this run never went through it
	}

	// begin from the CLI socket → running, even though systemd is present.
	u.applyRunProgress(progressLine{Event: ptr("begin"), Total: ptr(3)})
	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("begin: got (%q, %v), want %s", e.Type, ok, events.TypeUpgradeInfo)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("begin state = %q, want running", run.State)
	}

	// A bus terminal event for the unit must not finish a stream-owned run.
	u.onServiceEvent(events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "inactive"},
	})
	if e, ok := recv(t, u.Events(), 300*time.Millisecond); ok {
		t.Fatalf("bus event finished a CLI run: emitted %q", e.Type)
	}

	// end from the CLI → finished carrying the script verdict.
	u.applyRunProgress(progressLine{Event: ptr("end"), Success: ptr(true)})
	e, ok = recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("end: got (%q, %v), want %s", e.Type, ok, events.TypeUpgradeInfo)
	}
	run, _ := e.Data.(RunState)
	if run.State != "finished" || run.Success == nil || !*run.Success {
		t.Fatalf("end = %+v, want finished success=true", run)
	}
	if u.run.snapshot() != nil {
		t.Fatalf("run after end = %+v, want idle", u.run.snapshot())
	}
}

// Reported: an upgrade launched out of band (so the run is stream-owned) whose unit is
// SIGTERM'd mid-step — status=143, no "end" line ever reaches the socket — stays stuck on
// "running 40%". The unit's terminal `failed` state is authoritative: it must finish the
// run as a failure regardless of who owns it. Today onServiceEvent's finish(sourceUnit)
// returns nil for a stream-owned run, so nothing is emitted.
func TestUnitFailureFinishesStreamRun(t *testing.T) {
	u := &UpgradeBackend{
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &systemd.SystemdBackend{},
	}

	// Progress streams in before any StartUpgrade → the run is claimed by sourceStream.
	pct, cur, step := 40, 2, "upgrade"
	u.applyRunProgress(progressLine{Event: ptr("progress"), Percent: &pct, Current: &cur, Step: &step})
	if e, ok := recv(t, u.Events(), time.Second); !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("setup: got (%q, %v), want a leading running event", e.Type, ok)
	}

	// The unit is killed (status 143) → bus reports failed; the script sends no end line.
	u.onServiceEvent(events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "failed"},
	})

	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a finished info event after the unit failed, got (%q, %v)", e.Type, ok)
	}
	run, _ := e.Data.(RunState)
	if run.State != "finished" || run.Success == nil || *run.Success {
		t.Fatalf("verdict = %+v, want finished success=false", run)
	}
	if s := u.run.snapshot(); s != nil {
		t.Fatalf("run still tracked after unit failure: %+v, want idle", s)
	}
}

// A CLI run is snapshotted on graceful shutdown like a unit run, restored as a stream-owned hint,
// so resumeIfRunning can resume it and the script's resent end lands on a live run.
func TestInFlightCLIRunPersistsOnGracefulClose(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "upgrade-run.json")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8), stateFile: statePath}
	pct, step := 40, "upgrade"
	u.run.progress(sourceStream, &pct, &step) // CLI run claims sourceStream
	u.Close()

	u2 := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8), stateFile: statePath}
	u2.readState()
	if u2.resumeHint == nil || u2.resumeHint.Percent == nil || *u2.resumeHint.Percent != 40 {
		t.Fatalf("resumeHint = %+v, want a snapshot at 40%%", u2.resumeHint)
	}
	if u2.resumeHintSource != sourceStream {
		t.Fatalf("resumeHintSource = %v, want sourceStream", u2.resumeHintSource)
	}
}

// A CLI run snapshotted across a restart resumes blind (no unit to query) and re-announces running,
// so the script's resent end finishes a live run instead of being dropped on an idle tracker.
func TestResumeStreamRunFromSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8)}
	pct := 60
	u.resumeHint = &RunState{Percent: &pct, Step: ptr("finalize")}
	u.resumeHintSource = sourceStream

	u.resumeIfRunning()

	if e, ok := recv(t, u.Events(), time.Second); !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a running event, got (%q, %v)", e.Type, ok)
	}
	st, src := u.run.inflight()
	if st == nil || src != sourceStream || st.Percent == nil || *st.Percent != 60 {
		t.Fatalf("inflight = (%+v, %v), want stream-owned running at 60%%", st, src)
	}
}

// On restart, if the unit is still activating (the self-upgrade restarted us mid-playbook),
// resumeIfRunning re-announces running at the snapshot's percent and leaves the run live and
// unit-owned, so the bus finishes it like any other run.
func TestResumeRunningWhileUnitActivating(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{
		ctx:         ctx,
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &fakeUnits{state: "activating"},
	}
	pct := 40
	u.resumeHint = &RunState{Percent: &pct, Step: ptr("upgrade")}

	u.resumeIfRunning()

	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a running event, got (%q, %v)", e.Type, ok)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("resume state = %+v, want running", e.Data)
	}
	st, src := u.run.inflight()
	if st == nil || src != sourceUnit || st.Percent == nil || *st.Percent != 40 {
		t.Fatalf("inflight = (%+v, %v), want unit-owned running at 40%%", st, src)
	}
	if lr := u.StatusResponse().LastRun; lr != nil {
		t.Fatalf("last_run = %+v, want none while still running", lr)
	}
}

// A non-graceful kill (SIGKILL during a self-upgrade) leaves no snapshot, yet the unit is still
// activating on restart: resumeIfRunning must re-attach a bare running run anyway, so the badge
// shows running and the bus can still finish it. The old hint-gated return dropped this.
func TestResumeReattachesActivatingUnitWithoutSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{
		ctx:         ctx,
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &fakeUnits{state: "activating"},
	}
	// no resumeHint: nothing was persisted (the process was killed, not closed gracefully)

	u.resumeIfRunning()

	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a running event, got (%q, %v)", e.Type, ok)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("resume state = %+v, want running", e.Data)
	}
	if st, src := u.run.inflight(); st == nil || src != sourceUnit {
		t.Fatalf("inflight = (%+v, %v), want a unit-owned run re-attached", st, src)
	}
}

// A transient D-Bus error on startup means "can't tell yet", not "failed": resumeIfRunning must
// not fabricate a verdict, or a bus hiccup leaves the badge stuck on a false failure.
func TestResumeNoVerdictWhenUnitUnreadable(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{
		ctx:         ctx,
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &fakeUnits{refreshErr: errors.New("dbus down")},
	}
	pct := 70
	u.resumeHint = &RunState{Percent: &pct, Step: ptr("finalize")}

	u.resumeIfRunning()

	if e, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatalf("emitted %q, want no verdict when the unit is unreadable", e.Type)
	}
	if lr := u.StatusResponse().LastRun; lr != nil {
		t.Fatalf("last_run = %+v, want none", lr)
	}
}

// A terminal unit with no snapshot is some prior, already-recorded run — resumeIfRunning must not
// replay a verdict for it.
func TestResumeIgnoresTerminalUnitWithoutSnapshot(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{
		ctx:         ctx,
		events:      make(chan events.Event, 8),
		upgradeUnit: "odio-upgrade.service",
		systemd:     &fakeUnits{state: "failed"},
	}
	// no resumeHint

	u.resumeIfRunning()

	if e, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatalf("emitted %q, want nothing for a terminal unit without a snapshot", e.Type)
	}
	if u.run.snapshot() != nil {
		t.Fatalf("run = %+v, want idle (no replay)", u.run.snapshot())
	}
}

// On restart, a run snapshotted in flight whose unit reached a terminal state while we were down
// is resolved from that state: failed → failure, a clean terminal → success. The step rides along.
func TestResumeEmitsVerdictWhenUnitFinishedDuringDowntime(t *testing.T) {
	cases := map[string]struct {
		state   string
		success bool
	}{
		"unit failed":    {"failed", false},
		"unit succeeded": {"inactive", true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			t.Cleanup(cancel)

			u := &UpgradeBackend{
				ctx:         ctx,
				events:      make(chan events.Event, 8),
				upgradeUnit: "odio-upgrade.service",
				systemd:     &fakeUnits{state: tc.state},
			}
			pct := 70
			u.resumeHint = &RunState{Percent: &pct, Step: ptr("finalize")}

			u.resumeIfRunning()

			e, ok := recv(t, u.Events(), time.Second)
			if !ok || e.Type != events.TypeUpgradeInfo {
				t.Fatalf("want a finished verdict, got (%q, %v)", e.Type, ok)
			}
			run, _ := e.Data.(RunState)
			if run.State != "finished" || run.Success == nil || *run.Success != tc.success {
				t.Fatalf("verdict = %+v, want finished success=%v", e.Data, tc.success)
			}
			if lr := u.StatusResponse().LastRun; lr == nil || lr.Success != tc.success || lr.Step != "finalize" {
				t.Fatalf("last_run = %+v, want success=%v at step finalize", lr, tc.success)
			}
			if u.run.snapshot() != nil {
				t.Fatalf("run still tracked, want idle")
			}
		})
	}
}

// A finished run's verdict is exposed under last_run, persisted to the state file, and
// reloaded by a fresh backend on Start — so it survives a restart and a late page load.
func TestLastRunPersistsAndReloads(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state", "upgrade-run.json")

	u1 := &UpgradeBackend{events: make(chan events.Event, 8), stateFile: statePath}
	u1.applyRunProgress(progressLine{Event: ptr("begin"), Total: ptr(2)})
	u1.applyRunProgress(progressLine{Event: ptr("progress"), Percent: ptr(50), Current: ptr(1), Step: ptr("mpd")})
	u1.applyRunProgress(progressLine{Event: ptr("end"), Success: ptr(false), Error: ptr("disk full")})

	lr := u1.StatusResponse().LastRun
	if lr == nil || lr.Success || lr.Step != "mpd" || lr.Error != "disk full" {
		t.Fatalf("last_run after failed run = %+v, want failure at mpd: disk full", lr)
	}

	u2 := &UpgradeBackend{events: make(chan events.Event, 8), stateFile: statePath}
	u2.readState()
	got := u2.StatusResponse().LastRun
	if got == nil || got.Success || got.Step != "mpd" || got.Error != "disk full" {
		t.Fatalf("reloaded last_run = %+v, want the persisted failure", got)
	}
}

// A running unit upgrade is snapshotted on graceful shutdown and restored as a resume hint,
// so the next start can show the badge ring at the right percent (the self-upgrade case).
func TestInFlightUnitRunPersistsOnGracefulClose(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "upgrade-run.json")
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8), stateFile: statePath}
	u.run.start(sourceUnit)
	pct, step := 42, "mpd"
	u.run.progress(sourceStream, &pct, &step)
	u.Close()

	u2 := &UpgradeBackend{events: make(chan events.Event, 8), stateFile: statePath}
	u2.readState()
	if u2.resumeHint == nil || u2.resumeHint.Percent == nil || *u2.resumeHint.Percent != 42 {
		t.Fatalf("resumeHint = %+v, want a snapshot at 42%%", u2.resumeHint)
	}
}

func TestUnconfiguredTriggersReturnError(t *testing.T) {
	u, _ := newStarted(t, validResult)
	if err := u.CheckNow(); err != ErrUnitNotConfigured {
		t.Fatalf("CheckNow err = %v, want ErrUnitNotConfigured", err)
	}
	if err := u.StartUpgrade(); err != ErrUnitNotConfigured {
		t.Fatalf("StartUpgrade err = %v, want ErrUnitNotConfigured", err)
	}
}

// StartUpgrade triggers the upgrade unit, claims the run as unit-owned (so the bus, not the
// socket, finishes it), and announces running.
func TestStartUpgradeTriggersUnitAndAnnouncesRunning(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	fake := &fakeUnits{}
	u := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8), upgradeUnit: "odio-upgrade.service", systemd: fake}

	if err := u.StartUpgrade(); err != nil {
		t.Fatalf("StartUpgrade: %v", err)
	}
	if len(fake.triggered) != 1 || fake.triggered[0] != "odio-upgrade.service" {
		t.Fatalf("triggered = %v, want [odio-upgrade.service]", fake.triggered)
	}
	e, ok := recv(t, u.Events(), time.Second)
	if !ok || e.Type != events.TypeUpgradeInfo {
		t.Fatalf("want a running event, got (%q, %v)", e.Type, ok)
	}
	if run, _ := e.Data.(RunState); run.State != "running" {
		t.Fatalf("state = %+v, want running", e.Data)
	}
	if st, src := u.run.inflight(); st == nil || src != sourceUnit {
		t.Fatalf("inflight = (%+v, %v), want unit-owned", st, src)
	}
}

// When the trigger fails the claimed run is released (so a retry can start a fresh one) and no
// running event leaks.
func TestStartUpgradeClearsRunWhenTriggerFails(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	fake := &fakeUnits{triggerErr: errors.New("no bus")}
	u := &UpgradeBackend{ctx: ctx, events: make(chan events.Event, 8), upgradeUnit: "odio-upgrade.service", systemd: fake}

	if err := u.StartUpgrade(); err == nil {
		t.Fatal("StartUpgrade = nil, want the trigger error")
	}
	if u.run.snapshot() != nil {
		t.Fatalf("run still tracked after a failed trigger: %+v, want idle", u.run.snapshot())
	}
	if e, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatalf("failed trigger emitted %q, want no event", e.Type)
	}
}

// CheckNow starts the check unit in user scope and propagates its error unchanged.
func TestCheckNowStartsCheckUnit(t *testing.T) {
	fake := &fakeUnits{}
	u := &UpgradeBackend{events: make(chan events.Event, 4), checkUnit: "odio-check-upgrade.service", systemd: fake}

	if err := u.CheckNow(); err != nil {
		t.Fatalf("CheckNow: %v", err)
	}
	if len(fake.started) != 1 || fake.started[0] != "odio-check-upgrade.service" {
		t.Fatalf("started = %v, want [odio-check-upgrade.service]", fake.started)
	}

	fake.startErr = errors.New("masked")
	if err := u.CheckNow(); err == nil {
		t.Fatal("CheckNow = nil, want the unit start error")
	}
}

// A malformed checked_at is an optional cosmetic field: it is dropped, but the
// rest of the result must survive (the UI decodes checked_at as RFC3339).
func TestMalformedCheckedAtDropped(t *testing.T) {
	const result = `{"current":"dev","latest":"2026.7.0","upgrade_available":true,"checked_at":"2026-06-15 20:46:34"}`
	u, _ := newStarted(t, result)

	got := u.GetStatus()
	if got == nil {
		t.Fatal("GetStatus = nil, want a status")
	}
	if got.CheckedAt != "" {
		t.Errorf("CheckedAt = %q, want empty (malformed RFC3339 dropped)", got.CheckedAt)
	}
	if got.Latest != "2026.7.0" {
		t.Errorf("Latest = %q, want 2026.7.0 kept despite bad checked_at", got.Latest)
	}
}
