package upgrade

import "sync"

// runSource is who owns the in-flight run, fixed on the idle→running edge.
type runSource int

const (
	sourceNone   runSource = iota
	sourceUnit             // triggered via our systemd unit; the bus finishes it
	sourceStream           // launched out of band (CLI); the progress stream finishes it
)

// runTracker owns the run lifecycle; the source set on the idle→running edge decides who may finish it.
type runTracker struct {
	mu     sync.Mutex
	source runSource
	state  *RunState // nil when idle
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

// finish moves running→idle iff src owns the run.
func (t *runTracker) finish(src runSource) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.state == nil || t.source != src {
		return false
	}
	t.source = sourceNone
	t.state = nil
	return true
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
