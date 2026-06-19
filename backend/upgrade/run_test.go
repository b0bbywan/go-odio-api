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
