package upgrade

import (
	"context"
	"encoding/json"
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

func TestUnconfiguredTriggersReturnError(t *testing.T) {
	u, _ := newStarted(t, validResult)
	if err := u.CheckNow(); err != ErrUnitNotConfigured {
		t.Fatalf("CheckNow err = %v, want ErrUnitNotConfigured", err)
	}
	if err := u.StartUpgrade(); err != ErrUnitNotConfigured {
		t.Fatalf("StartUpgrade err = %v, want ErrUnitNotConfigured", err)
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
