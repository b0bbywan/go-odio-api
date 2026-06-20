package upgrade

import "testing"

func TestRunTrackerStartRejectsSecond(t *testing.T) {
	var r runTracker
	if !r.start(sourceUnit) {
		t.Fatal("first start = false, want true")
	}
	if r.start(sourceStream) {
		t.Fatal("second start = true, want false (a run is already in flight)")
	}
}

func TestRunTrackerProgressClaimsOnIdleEdge(t *testing.T) {
	var r runTracker
	pct := 10
	if !r.progress(sourceStream, &pct, nil) {
		t.Fatal("first progress started = false, want true (idle→running edge)")
	}
	pct2 := 20
	if r.progress(sourceStream, &pct2, nil) {
		t.Fatal("second progress started = true, want false (already running)")
	}
	if s := r.snapshot(); s == nil || s.Percent == nil || *s.Percent != 20 {
		t.Fatalf("snapshot = %+v, want live percent 20", s)
	}
}

// resume restores a running state seeded from a persisted snapshot, and refuses once a run exists.
func TestRunTrackerResumeSeedsSnapshot(t *testing.T) {
	var r runTracker
	pct, step := 42, "mpd"
	if !r.resume(sourceUnit, &RunState{Percent: &pct, Step: &step}) {
		t.Fatal("resume on idle = false, want true")
	}
	s := r.snapshot()
	if s == nil || s.State != "running" || s.Percent == nil || *s.Percent != 42 {
		t.Fatalf("snapshot = %+v, want running at 42%%", s)
	}
	if r.resume(sourceStream, nil) {
		t.Fatal("resume while a run exists = true, want false")
	}
}

// inflight reports the live run and the owning source, for the shutdown snapshot.
func TestRunTrackerInflightReportsSource(t *testing.T) {
	var r runTracker
	if st, src := r.inflight(); st != nil || src != sourceNone {
		t.Fatalf("idle inflight = (%+v, %v), want (nil, none)", st, src)
	}
	r.start(sourceUnit)
	pct := 10
	r.progress(sourceStream, &pct, nil) // a non-owning update keeps the unit owner
	st, src := r.inflight()
	if st == nil || src != sourceUnit || st.Percent == nil || *st.Percent != 10 {
		t.Fatalf("inflight = (%+v, %v), want unit-owned at 10%%", st, src)
	}
}

// A non-owning source refreshes the live state but never re-claims, and cannot finish.
// The error it reported via noteEnd still folds into the owner's verdict, with the last step.
func TestRunTrackerOwnershipIsExclusive(t *testing.T) {
	var r runTracker
	r.start(sourceUnit) // unit owns the run
	step := "mpd"
	pct := 42
	if r.progress(sourceStream, &pct, &step) {
		t.Fatal("stream progress on a unit run started = true, want false (no re-claim)")
	}
	r.noteEnd(ptr("disk full"))
	if r.finish(sourceStream, true) != nil {
		t.Fatal("stream finish on a unit run != nil, want nil (not the owner)")
	}
	lr := r.finish(sourceUnit, false)
	if lr == nil {
		t.Fatal("unit finish on a unit run = nil, want a verdict (owner)")
	}
	if lr.Success || lr.Step != "mpd" || lr.Error != "disk full" || lr.FinishedAt == "" {
		t.Fatalf("verdict = %+v, want success=false step=mpd error='disk full' finished_at set", lr)
	}
	if r.snapshot() != nil {
		t.Fatal("snapshot after finish, want nil (idle)")
	}
}

// A successful finish must not carry a stashed error: the script can report end{success:false}
// while the unit still exits 0, and success=true with a non-empty error is contradictory.
func TestRunTrackerSuccessDropsStashedError(t *testing.T) {
	var r runTracker
	r.start(sourceUnit)
	r.noteEnd(ptr("disk full"))
	lr := r.finish(sourceUnit, true)
	if lr == nil || !lr.Success {
		t.Fatalf("verdict = %+v, want success", lr)
	}
	if lr.Error != "" {
		t.Fatalf("verdict carried error %q on success, want none", lr.Error)
	}
}

// An end line that arrives outside any run (e.g. after the bus already finished a unit run,
// or a duplicate) must not stash an error that the NEXT run would then carry as its verdict.
func TestRunTrackerNoteEndOutsideRunDoesNotLeak(t *testing.T) {
	var r runTracker
	r.noteEnd(ptr("disk full")) // no run in flight

	r.start(sourceUnit)
	lr := r.finish(sourceUnit, true)
	if lr == nil {
		t.Fatal("finish = nil, want a verdict")
	}
	if lr.Error != "" {
		t.Fatalf("verdict carried a stale error %q, want none", lr.Error)
	}
}
