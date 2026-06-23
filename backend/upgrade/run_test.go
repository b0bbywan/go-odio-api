package upgrade

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/events"
)

func strp(s string) *string { return &s }
func intp(n int) *int       { return &n }
func boolp(b bool) *bool    { return &b }

// TestRunStatePersistsAcrossRestart is the save→load round trip: a terminal
// verdict written at shutdown must come back intact at the next boot. A failed
// verdict is used so the result is distinguishable from the idle default. The
// parent dir does not exist yet, so this also covers save() creating it — its
// absence silently dropped every snapshot, losing the run on restart.
func TestRunStatePersistsAcrossRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "odio-api", "upgrade-run.json")

	r := newRun(path)
	r.start(originSystemd)
	r.progress(70, "installing")
	r.finish(false)
	r.save()

	restored := newRun(path)
	restored.load()
	got := restored.snapshot()
	if got.State != stateFailed || got.Origin != originSystemd {
		t.Fatalf("restored = %+v, want failed/systemd", got)
	}
	if got.StartedAt == "" || got.FinishedAt == "" {
		t.Errorf("restored timestamps empty: %+v", got)
	}
}

// TestRunStateGracefulRestartKeepsPercent is the self-upgrade case: odio-api is
// restarted by its own upgrade unit while a systemd run is mid-flight. The
// in-flight percent must survive the save→load round trip (parent dir absent),
// so the resumed run shows real progress, not an indeterminate ring.
func TestRunStateGracefulRestartKeepsPercent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "odio-api", "upgrade-run.json")

	r := newRun(path)
	r.start(originSystemd)
	r.begin(7)
	r.progress(42, "mpd")
	r.save()

	restored := newRun(path)
	restored.load()
	if got := restored.snapshot(); got.State != stateRunning || got.Percent == nil || *got.Percent != 42 {
		t.Fatalf("restored = %+v, want running with percent 42", got)
	}
}

// TestRunStateRestoresRunningSocketRun covers the resume case: a socket run
// still in flight at shutdown reloads as running so resumeIfRunning can await
// the client.
func TestRunStateRestoresRunningSocketRun(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade-run.json")

	r := newRun(path)
	r.start(originSocket)
	r.progress(40, "mpd")
	r.save()

	restored := newRun(path)
	restored.load()
	if !restored.isRunning() || restored.origin() != originSocket {
		t.Fatalf("restored = %+v, want running/socket", restored.snapshot())
	}
}

func TestRunStateLoadMissingKeepsDefault(t *testing.T) {
	r := newRun(filepath.Join(t.TempDir(), "absent.json"))
	r.load()
	if got := r.snapshot(); got.State != stateIdle {
		t.Fatalf("missing-file load = %+v, want idle default", got)
	}
}

func TestRunStateLoadInvalidKeepsDefault(t *testing.T) {
	path := filepath.Join(t.TempDir(), "upgrade-run.json")
	writeFile(t, path, `{not json`)

	r := newRun(path)
	r.load()
	if got := r.snapshot(); got.State != stateIdle {
		t.Fatalf("invalid-file load = %+v, want idle default", got)
	}
}

// TestRunBeginAdoptsSocketOrigin: a run that never went through start (CLI
// upgrade outside the unit) takes the socket origin from its begin line.
func TestRunBeginAdoptsSocketOrigin(t *testing.T) {
	r := newRun("")
	r.begin(3)
	got := r.snapshot()
	if got.State != stateRunning || got.Origin != originSocket || got.Percent == nil || *got.Percent != 0 {
		t.Fatalf("begin = %+v, want running/socket/0%%", got)
	}
}

// A begin arriving on an already-started systemd run must not steal its origin.
func TestRunBeginKeepsExistingOrigin(t *testing.T) {
	r := newRun("")
	r.start(originSystemd)
	r.begin(3)
	if got := r.origin(); got != originSystemd {
		t.Fatalf("origin after begin = %q, want %q", got, originSystemd)
	}
}

func TestRunFinishIsIdempotent(t *testing.T) {
	r := newRun("")
	r.start(originSystemd)
	if !r.finish(true) {
		t.Fatal("first finish = false, want true")
	}
	if r.finish(false) {
		t.Fatal("second finish = true, want false (no run to close)")
	}
	if got := r.snapshot(); got.State != stateIdle {
		t.Errorf("verdict = %+v, want unchanged idle (success)", got)
	}
}

// TestApplyRunProgressSocketLifecycle drives the full begin→progress→end stream
// and asserts begin/end emit upgrade.info while progress does not, and that a
// repeated end is a no-op.
func TestApplyRunProgressSocketLifecycle(t *testing.T) {
	u := &UpgradeBackend{run: newRun(""), events: make(chan events.Event, 16)}

	u.applyRunProgress(progressLine{Event: strp("begin"), Total: intp(3)})
	e, ok := recv(t, u.Events(), time.Second)
	if !ok {
		t.Fatal("begin emitted no upgrade.info event")
	}
	if rs, _ := e.Data.(RunState); rs.State != stateRunning || rs.Origin != originSocket {
		t.Fatalf("begin event = %+v, want running/socket", e.Data)
	}

	u.applyRunProgress(progressLine{Event: strp("progress"), Percent: intp(50), Step: strp("mpd")})
	if _, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatal("progress should not emit an info event")
	}
	if s := u.run.snapshot(); s.Percent == nil || *s.Percent != 50 {
		t.Fatalf("after progress = %+v, want percent 50", s)
	}

	u.applyRunProgress(progressLine{Event: strp("end"), Success: boolp(false)})
	e, ok = recv(t, u.Events(), time.Second)
	if !ok {
		t.Fatal("end emitted no upgrade.info event")
	}
	if rs, _ := e.Data.(RunState); rs.State != stateFailed {
		t.Fatalf("end event = %+v, want failed", e.Data)
	}

	u.applyRunProgress(progressLine{Event: strp("end"), Success: boolp(false)})
	if _, ok := recv(t, u.Events(), 200*time.Millisecond); ok {
		t.Fatal("a second end should not emit (finish is a no-op)")
	}
}

// TestResumeIfRunningSocketAwaitsReconnect: a restored socket run owns its
// lifecycle, so resume re-announces it without consulting systemd.
func TestResumeIfRunningSocketAwaitsReconnect(t *testing.T) {
	u := &UpgradeBackend{
		run:     newRun(""),
		events:  make(chan events.Event, 16),
		systemd: &systemd.SystemdBackend{}, // never touched on the socket path
		ctx:     context.Background(),
	}
	u.run.start(originSocket)

	u.resumeIfRunning()
	e, ok := recv(t, u.Events(), time.Second)
	if !ok {
		t.Fatal("resume of a socket run emitted no event")
	}
	if rs, _ := e.Data.(RunState); rs.State != stateRunning || rs.Origin != originSocket {
		t.Fatalf("resume event = %+v, want running/socket", e.Data)
	}
}
