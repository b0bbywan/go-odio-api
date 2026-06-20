package upgrade

import (
	"sync"
	"time"
)

// runPhase is the explicit run lifecycle. The progress stream is the trunk (idle→running→done);
// the systemd unit decorates it with two states: triggered (before the first progress line) and
// settling (after the script's end, awaiting the authoritative job result).
type runPhase int

// These values are persisted as the in-flight phase across a (self-upgrade) restart: append new
// phases at the end, never renumber, or a resume reads back the wrong phase from an older state file.
const (
	phaseIdle      runPhase = iota
	phaseTriggered          // unit run: started, awaiting the first begin/progress
	phaseRunning            // begin/progress seen — the trunk, for both CLI and unit runs
	phaseSettling           // unit run: end seen, awaiting the systemd job result
)

// activeRun is the whole live run held as one nil-able value so idle (cur == nil) has a single
// source of truth. unit marks a run we triggered through the unit: only then does the systemd job
// result apply. pendErr is the script-reported error, folded into a failure verdict.
type activeRun struct {
	phase   runPhase
	unit    bool
	live    RunState
	pendErr *string
}

// runTracker owns the run lifecycle as an explicit state machine.
type runTracker struct {
	mu  sync.Mutex
	cur *activeRun
}

// trigger moves idle→triggered for a unit run, or returns false when a run is already in flight.
func (t *runTracker) trigger() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur != nil {
		return false
	}
	t.cur = &activeRun{phase: phaseTriggered, unit: true, live: RunState{State: "running"}}
	return true
}

// abort clears a triggered unit run whose unit trigger failed; a no-op once progress has begun.
func (t *runTracker) abort() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur != nil && t.cur.phase == phaseTriggered {
		t.cur = nil
	}
}

// observeProgress applies a begin/progress line. It claims an idle tracker as a CLI run, advances a
// triggered unit run to running, and refreshes a running one. announce is true only on the
// idle→running edge — a unit run already announced running when it was triggered.
func (t *runTracker) observeProgress(percent *int, step *string) (announce bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	switch {
	case t.cur == nil:
		t.cur = &activeRun{phase: phaseRunning, live: RunState{State: "running", Percent: percent, Step: step}}
		return true
	case t.cur.phase == phaseSettling:
		return false // end already seen; nothing more to stream
	default:
		t.cur.phase = phaseRunning
		t.cur.live = RunState{State: "running", Percent: percent, Step: step}
		return false
	}
}

// observeEnd applies the script's end line. A CLI run finalizes here with the script's verdict; a
// unit run moves to settling and waits for the job result (authoritative). An end outside a run is
// ignored — it must not leak its error into the next run.
func (t *runTracker) observeEnd(success bool, errStr *string) *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return nil
	}
	t.cur.pendErr = errStr
	if t.cur.unit {
		t.cur.phase = phaseSettling
		return nil
	}
	return t.finalizeLocked(success)
}

// failDisconnected finalizes a live CLI run whose progress connection dropped and never came back, so
// a crashed script does not strand the run on "running". A unit run (the bus finishes it) or an idle
// tracker is left untouched.
func (t *runTracker) failDisconnected() *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil || t.cur.unit {
		return nil
	}
	lost := "upgrade connection lost"
	t.cur.pendErr = &lost
	return t.finalizeLocked(false)
}

// observeUnitTerminal applies the systemd job result. It finalizes a unit run with the job's verdict
// (authoritative, covering a script killed before its end) and ignores a CLI run or an idle tracker.
func (t *runTracker) observeUnitTerminal(jobOK bool) *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil || !t.cur.unit {
		return nil
	}
	return t.finalizeLocked(jobOK)
}

// finalizeLocked builds the verdict from the live run and resets to idle; caller holds mu. The
// stashed error folds only into a failure: a successful run carrying an error would be contradictory.
func (t *runTracker) finalizeLocked(success bool) *LastRun {
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

// resume restores a run at its persisted phase and ownership from a snapshot (nil → bare running),
// or returns false when a run is already tracked.
func (t *runTracker) resume(snap *RunState, phase runPhase, unit bool) bool {
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
	t.cur = &activeRun{phase: phase, unit: unit, live: live}
	return true
}

// inflight returns a copy of the live run with its phase and ownership, or (nil, phaseIdle, false)
// when idle — the shutdown snapshot.
func (t *runTracker) inflight() (*RunState, runPhase, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cur == nil {
		return nil, phaseIdle, false
	}
	live := t.cur.live
	return &live, t.cur.phase, t.cur.unit
}

// snapshot is inflight reduced to the live wire state (nil when idle).
func (t *runTracker) snapshot() *RunState {
	st, _, _ := t.inflight()
	return st
}
