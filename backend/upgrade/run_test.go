package upgrade

import "testing"

// A CLI run is the trunk: the first progress line claims it (announcing the running edge), later
// lines refresh without re-announcing, and the script's end finalizes it with its own verdict.
func TestCLIRunLifecycle(t *testing.T) {
	var r runTracker
	pct := 10
	if !r.observeProgress(&pct, nil) {
		t.Fatal("first progress announce = false, want true (idle→running edge)")
	}
	pct2 := 60
	step := "mpd"
	if r.observeProgress(&pct2, &step) {
		t.Fatal("second progress announce = true, want false (already running)")
	}
	if s := r.snapshot(); s == nil || s.State != "running" || s.Percent == nil || *s.Percent != 60 {
		t.Fatalf("snapshot = %+v, want running at 60%%", s)
	}
	lr := r.observeEnd(true, nil)
	if lr == nil || !lr.Success || lr.Step != "mpd" || lr.FinishedAt == "" {
		t.Fatalf("verdict = %+v, want success step=mpd finished_at set", lr)
	}
	if r.snapshot() != nil {
		t.Fatal("snapshot after end, want nil (idle)")
	}
}

// A CLI run's failure end folds its reported error into the verdict.
func TestCLIRunEndCarriesError(t *testing.T) {
	var r runTracker
	pct := 30
	step := "common"
	r.observeProgress(&pct, &step)
	lr := r.observeEnd(false, ptr("apt cache failed"))
	if lr == nil || lr.Success || lr.Step != "common" || lr.Error != "apt cache failed" {
		t.Fatalf("verdict = %+v, want failure step=common error set", lr)
	}
}

// A unit run goes idle→triggered (awaiting begin)→running; its end does NOT finalize — the
// verdict waits for the authoritative systemd job result.
func TestUnitRunLifecycle(t *testing.T) {
	var r runTracker
	if !r.trigger() {
		t.Fatal("trigger on idle = false, want true")
	}
	if r.trigger() {
		t.Fatal("second trigger = true, want false (a run is in flight)")
	}
	if s := r.snapshot(); s == nil || s.State != "running" || s.Percent != nil {
		t.Fatalf("triggered snapshot = %+v, want running with no percent (indeterminate)", s)
	}
	pct := 50
	step := "mpd"
	if r.observeProgress(&pct, &step) {
		t.Fatal("progress on a triggered unit run announced = true, want false (already announced at trigger)")
	}
	if lr := r.observeEnd(true, nil); lr != nil {
		t.Fatalf("unit end returned a verdict %+v, want nil (awaiting job result)", lr)
	}
	if s := r.snapshot(); s == nil || s.State != "running" {
		t.Fatalf("snapshot after unit end = %+v, want still running (settling)", s)
	}
	lr := r.observeUnitTerminal(true)
	if lr == nil || !lr.Success || lr.Step != "mpd" {
		t.Fatalf("verdict = %+v, want success from the job result, step=mpd", lr)
	}
	if r.snapshot() != nil {
		t.Fatal("snapshot after job result, want nil (idle)")
	}
}

// The systemd job result is authoritative for a unit run: a failed job overrides a script that
// reported success, and folds the script's reported error.
func TestUnitJobOverridesScriptVerdict(t *testing.T) {
	var r runTracker
	r.trigger()
	pct := 90
	step := "finalize"
	r.observeProgress(&pct, &step)
	r.observeEnd(true, ptr("late warning")) // script self-reports success, with a note
	lr := r.observeUnitTerminal(false)      // but the job failed
	if lr == nil || lr.Success {
		t.Fatalf("verdict = %+v, want failure (job authoritative over the script)", lr)
	}
	if lr.Step != "finalize" || lr.Error != "late warning" {
		t.Fatalf("verdict = %+v, want step=finalize error='late warning'", lr)
	}
}

// A unit run killed before it streams an end (status 143) still gets a verdict from the failed job.
func TestUnitFailsBeforeEnd(t *testing.T) {
	var r runTracker
	r.trigger()
	pct := 40
	step := "mpd"
	r.observeProgress(&pct, &step)
	lr := r.observeUnitTerminal(false)
	if lr == nil || lr.Success || lr.Step != "mpd" {
		t.Fatalf("verdict = %+v, want failure at step=mpd", lr)
	}
}

// THE REGRESSION: a unit terminal event must not touch a CLI run. A stale failed upgrade unit
// (its terminal state lingering from a prior run) used to kill any in-flight run via finishAny.
func TestUnitTerminalIgnoresCLIRun(t *testing.T) {
	var r runTracker
	pct := 25
	r.observeProgress(&pct, nil) // CLI run claims the trunk (unit=false)

	if lr := r.observeUnitTerminal(false); lr != nil {
		t.Fatalf("unit terminal on a CLI run returned %+v, want nil (ignored)", lr)
	}
	if s := r.snapshot(); s == nil || s.State != "running" {
		t.Fatalf("CLI run after a unit terminal = %+v, want still running (untouched)", s)
	}
}

// An end outside any run records nothing and leaves no residue for the next run.
func TestObserveEndOutsideRun(t *testing.T) {
	var r runTracker
	if lr := r.observeEnd(false, ptr("disk full")); lr != nil {
		t.Fatalf("end on idle = %+v, want nil", lr)
	}
	pct := 10
	r.observeProgress(&pct, nil)
	lr := r.observeEnd(true, nil)
	if lr == nil || !lr.Success || lr.Error != "" {
		t.Fatalf("verdict = %+v, want clean success with no leaked error", lr)
	}
}

// A failed unit trigger aborts the triggered run back to idle, so the next trigger can start fresh.
func TestAbortClearsTriggered(t *testing.T) {
	var r runTracker
	r.trigger()
	r.abort()
	if r.snapshot() != nil {
		t.Fatal("snapshot after abort, want nil (idle)")
	}
	if !r.trigger() {
		t.Fatal("trigger after abort = false, want true (run was cleared)")
	}
}

// resume restores a run at its persisted phase and ownership, and refuses once a run exists.
func TestResumeRestoresPhaseAndUnit(t *testing.T) {
	var r runTracker
	pct, step := 42, "mpd"
	if !r.resume(&RunState{Percent: &pct, Step: &step}, phaseRunning, true) {
		t.Fatal("resume on idle = false, want true")
	}
	s := r.snapshot()
	if s == nil || s.State != "running" || s.Percent == nil || *s.Percent != 42 {
		t.Fatalf("snapshot = %+v, want running at 42%%", s)
	}
	if r.resume(nil, phaseRunning, false) {
		t.Fatal("resume while a run exists = true, want false")
	}
	// Resumed as a unit run: the job result still finalizes it.
	if lr := r.observeUnitTerminal(true); lr == nil || !lr.Success {
		t.Fatalf("verdict after resumed unit terminal = %+v, want success", lr)
	}
}

// inflight reports the live run with its phase and ownership, for the shutdown snapshot.
func TestInflightReportsPhaseAndUnit(t *testing.T) {
	var r runTracker
	if st, ph, unit := r.inflight(); st != nil || ph != phaseIdle || unit {
		t.Fatalf("idle inflight = (%+v, %v, %v), want (nil, idle, false)", st, ph, unit)
	}
	r.trigger()
	if st, ph, unit := r.inflight(); st == nil || ph != phaseTriggered || !unit {
		t.Fatalf("triggered inflight = (%+v, %v, %v), want (running, triggered, true)", st, ph, unit)
	}
	pct := 10
	r.observeProgress(&pct, nil)
	st, ph, unit := r.inflight()
	if st == nil || ph != phaseRunning || !unit || st.Percent == nil || *st.Percent != 10 {
		t.Fatalf("running inflight = (%+v, %v, %v), want (running@10, running, true)", st, ph, unit)
	}
}
