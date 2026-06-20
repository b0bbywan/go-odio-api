package upgrade

import (
	"sync"
	"time"
)

// runSource is who owns the in-flight run, fixed on the idle→running edge.
type runSource int

const (
	sourceNone   runSource = iota
	sourceUnit             // triggered via our systemd unit; the bus finishes it
	sourceStream           // launched out of band (CLI); the progress stream finishes it
)

// activeRun is the whole live-run state: who owns it, the live snapshot, and the script-reported
// end error not yet folded into a verdict. Held as one nil-able value so idle (cur == nil) has a
// single source of truth and the three pieces can never desync.
type activeRun struct {
	source  runSource
	live    RunState
	pendErr *string
}

// runTracker owns the run lifecycle. The source set on the idle→running edge decides who may finish it.
type runTracker struct {
	mu  sync.Mutex
	cur *activeRun // nil when idle
}

// start moves idle→running for src, or returns false when a run is already in flight.
func (t *runTracker) start(src runSource) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur != nil {
		return false
	}
	t.cur = &activeRun{source: src, live: RunState{State: "running"}}
	return true
}

// progress refreshes percent/step; a non-owning source updates without re-claiming. Returns true on the idle→running edge.
func (t *runTracker) progress(src runSource, percent *int, step *string) (started bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	started = t.cur == nil
	if started {
		t.cur = &activeRun{source: src}
	}
	t.cur.live = RunState{State: "running", Percent: percent, Step: step}
	return started
}

// noteEnd stashes the script-reported error from an end line so the owner's finish (possibly the
// bus, for a unit run) can carry it. Ignored when idle: an end line outside a run, or one arriving
// after the bus already finished, must not leak its error into the next run.
func (t *runTracker) noteEnd(err *string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return
	}
	t.cur.pendErr = err
}

// finish moves running→idle iff src owns the run, returning the run's verdict; nil otherwise.
func (t *runTracker) finish(src runSource, success bool) *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil || t.cur.source != src {
		return nil
	}
	return t.finishLocked(success)
}

// finishAny finishes whatever run is in flight, ignoring ownership; nil when idle. The bus uses it
// on a failed unit: a run killed mid-step never sends its end line, so the unit's terminal failure
// is authoritative even over a stream-owned run.
func (t *runTracker) finishAny(success bool) *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return nil
	}
	return t.finishLocked(success)
}

// finishLocked builds the verdict from the live run and resets to idle; caller holds mu.
// The stashed error is only folded into a failure: a successful run carrying an error string
// would be contradictory (the script can report end{success:false} while the unit still exits 0).
func (t *runTracker) finishLocked(success bool) *LastRun {
	lr := &LastRun{Success: success, FinishedAt: time.Now().UTC().Format(time.RFC3339)}
	if t.cur.live.Step != nil {
		lr.Step = *t.cur.live.Step
	}
	if !success && t.cur.pendErr != nil {
		lr.Error = *t.cur.pendErr
	}
	t.cur = nil
	return lr
}

// resume restores a running state for src from a persisted snapshot (nil → bare running),
// or returns false when a run is already tracked.
func (t *runTracker) resume(src runSource, snap *RunState) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur != nil {
		return false
	}
	live := RunState{State: "running"}
	if snap != nil {
		live = *snap
		live.State = "running"
	}
	t.cur = &activeRun{source: src, live: live}
	return true
}

// inflight returns a copy of the live run and its source, or (nil, sourceNone) when idle.
func (t *runTracker) inflight() (*RunState, runSource) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return nil, sourceNone
	}
	live := t.cur.live
	return &live, t.cur.source
}

// snapshot is inflight without the source, for callers that only need the live state.
func (t *runTracker) snapshot() *RunState {
	st, _ := t.inflight()
	return st
}
