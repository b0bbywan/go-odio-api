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
func TestRunTrackerOwnershipIsExclusive(t *testing.T) {
	var r runTracker
	r.start(sourceUnit) // unit owns the run
	pct := 42
	if r.progress(sourceStream, &pct, nil) {
		t.Fatal("stream progress on a unit run started = true, want false (no re-claim)")
	}
	if r.finish(sourceStream) {
		t.Fatal("stream finish on a unit run = true, want false (not the owner)")
	}
	if !r.finish(sourceUnit) {
		t.Fatal("unit finish on a unit run = false, want true (owner)")
	}
	if r.snapshot() != nil {
		t.Fatal("snapshot after finish, want nil (idle)")
	}
}
