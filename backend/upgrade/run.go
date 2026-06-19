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

// runTracker owns the run lifecycle; the source set on the idle→running edge decides who may finish it.
type runTracker struct {
	mu      sync.Mutex
	source  runSource
	state   *RunState // nil when idle
	lastErr *string   // error the script reported on its end line, folded into the next finish
}

// start moves idle→running for src, or returns false when a run is already in flight.
func (t *runTracker) start(src runSource) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state != nil {
		return false
	}
	t.source = src
	t.state = &RunState{State: "running"}
	return true
}

// progress refreshes percent/step; a non-owning source updates without re-claiming. Returns true on the idle→running edge.
func (t *runTracker) progress(src runSource, percent *int, step *string) (started bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	started = t.state == nil
	if started {
		t.source = src
	}
	t.state = &RunState{State: "running", Percent: percent, Step: step}
	return started
}

// noteEnd stashes the script-reported error from an end line, regardless of who owns the
// run, so a unit run finished by the bus can still carry it.
func (t *runTracker) noteEnd(err *string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.lastErr = err
}

// finish moves running→idle iff src owns the run, returning the run's verdict; nil otherwise.
func (t *runTracker) finish(src runSource, success bool) *LastRun {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil || t.source != src {
		return nil
	}
	lr := &LastRun{Success: success, FinishedAt: time.Now().UTC().Format(time.RFC3339)}
	if t.state.Step != nil {
		lr.Step = *t.state.Step
	}
	if t.lastErr != nil {
		lr.Error = *t.lastErr
	}
	t.source = sourceNone
	t.state = nil
	t.lastErr = nil
	return lr
}

func (t *runTracker) snapshot() *RunState {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil {
		return nil
	}
	cp := *t.state
	return &cp
}
