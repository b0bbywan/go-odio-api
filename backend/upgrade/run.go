package upgrade

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	stateIdle    = "idle"
	stateRunning = "running"
	stateFailed  = "failed"

	originSystemd = "systemd"
	originSocket  = "socket"
)

// run is the single source of truth for the upgrade run lifecycle, replacing the
// former running/runState pair. The state is always set: idle means none-or-last-
// succeeded, so a fresh install starts from idle rather than an absent run the
// clients would have to special-case.
//
// Progress is kept in memory and only flushed to disk on Close. The state file
// is a shutdown dump for load() at next boot, not a live journal: persisting
// every progress tick would hammer the SD card on a Pi for no gain. A SIGKILL
// or power loss therefore loses the in-memory percent; resumeIfRunning then
// reconciles against systemd for a systemd-origin run, and a socket-origin run
// simply waits for its CLI client to reconnect.
type run struct {
	mu   sync.Mutex
	path string
	st   RunState
}

func newRun(path string) *run {
	return &run{path: path, st: defaultState()}
}

func defaultState() RunState {
	return RunState{State: stateIdle}
}

// load restores the snapshot persisted at the last clean shutdown, leaving the
// idle default in place on a missing or invalid file.
func (r *run) load() {
	if r.path == "" {
		logger.Warn("[upgrade] no state file configured, starting from idle (a run cannot survive a restart)")
		return
	}
	data, err := os.ReadFile(r.path)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("[upgrade] no run state file at %s, starting from idle", r.path)
		} else {
			logger.Warn("[upgrade] cannot read run state %s, starting from idle: %v", r.path, err)
		}
		return
	}
	var st RunState
	if err := json.Unmarshal(data, &st); err != nil || st.State == "" {
		logger.Warn("[upgrade] run state %s invalid (%q), keeping idle default: %v", r.path, string(data), err)
		return
	}
	r.mu.Lock()
	r.st = st
	r.mu.Unlock()
	logger.Info("[upgrade] restored run state from %s: state=%s origin=%s percent=%s", r.path, st.State, st.Origin, pctLabel(st.Percent))
}

// save writes the current snapshot atomically, creating the parent dir if it is
// missing. Called once from Close; a failure here means the next boot cannot
// restore the run, so every exit path is logged.
func (r *run) save() {
	if r.path == "" {
		logger.Warn("[upgrade] no state file configured, run state not persisted")
		return
	}
	r.mu.Lock()
	st := r.st
	data, err := json.Marshal(st)
	r.mu.Unlock()
	if err != nil {
		logger.Warn("[upgrade] cannot marshal run state: %v", err)
		return
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0o700); err != nil {
		logger.Warn("[upgrade] cannot create state dir %s, run state not persisted: %v", filepath.Dir(r.path), err)
		return
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		logger.Warn("[upgrade] cannot write run state %s: %v", tmp, err)
		return
	}
	if err := os.Rename(tmp, r.path); err != nil {
		logger.Warn("[upgrade] cannot commit run state %s: %v", r.path, err)
		return
	}
	logger.Info("[upgrade] saved run state to %s: state=%s origin=%s percent=%s", r.path, st.State, st.Origin, pctLabel(st.Percent))
}

func pctLabel(p *int) string {
	if p == nil {
		return "n/a"
	}
	return strconv.Itoa(*p)
}

// start reserves the run for origin. It returns false when one is already in
// flight: this is the concurrency guard the atomic.Bool used to provide.
func (r *run) start(origin string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State == stateRunning {
		logger.Debug("[upgrade] run start rejected, already running (origin=%s)", r.st.Origin)
		return false
	}
	r.st = RunState{State: stateRunning, Origin: origin, StartedAt: nowRFC3339()}
	logger.Info("[upgrade] run started (origin=%s)", origin)
	return true
}

// begin handles the socket "begin" line. A CLI-driven run that never went
// through start adopts the socket origin here, which is what lets an upgrade
// launched outside the systemd unit show up in the UI.
func (r *run) begin(total int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State != stateRunning {
		r.st = RunState{State: stateRunning, Origin: originSocket, StartedAt: nowRFC3339()}
		logger.Info("[upgrade] run adopted from socket begin (total=%d)", total)
	} else {
		logger.Debug("[upgrade] socket begin on an already-running %s run (total=%d)", r.st.Origin, total)
	}
	zero := 0
	r.st.Percent = &zero
	r.st.Step = nil
}

// claim makes the run origin's: a new one when none is in flight, or the one a
// socket begin adopted, taken over by the unit that turns out to run it. It
// reports whether anything changed.
func (r *run) claim(origin string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State == stateRunning {
		if r.st.Origin == origin {
			return false
		}
		logger.Info("[upgrade] run taken over: origin %s -> %s", r.st.Origin, origin)
		r.st.Origin = origin
		return true
	}
	r.st = RunState{State: stateRunning, Origin: origin, StartedAt: nowRFC3339()}
	logger.Info("[upgrade] run claimed (origin=%s)", origin)
	return true
}

func (r *run) progress(pct int, step string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State != stateRunning {
		logger.Debug("[upgrade] dropping progress %d%% (%s): no run in flight", pct, step)
		return
	}
	r.st.Percent = &pct
	r.st.Step = &step
	logger.Debug("[upgrade] run progress %d%% (%s)", pct, step)
}

// finish records the verdict and returns false when there was no run to close.
// The two terminal sources, the socket "end" line and the unit reaching a
// terminal state, can race; the bool lets the caller emit finished exactly once.
func (r *run) finish(success bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.finishLocked(success)
}

func (r *run) finishLocked(success bool) bool {
	if r.st.State != stateRunning {
		return false
	}
	state := stateIdle
	if !success {
		state = stateFailed
	}
	r.st = RunState{
		State:      state,
		Origin:     r.st.Origin,
		StartedAt:  r.st.StartedAt,
		FinishedAt: nowRFC3339(),
	}
	logger.Info("[upgrade] run finished: state=%s origin=%s", state, r.st.Origin)
	return true
}

// finishSocket is the socket "end" line: it closes a run the socket owns. A
// systemd run is left to its unit, which may run more than one playbook (a
// disable, then the upgrade) and has the last word on how it went.
func (r *run) finishSocket(success bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State == stateRunning && r.st.Origin == originSystemd {
		logger.Debug("[upgrade] socket end (success=%v) on a systemd run, waiting for the unit", success)
		return false
	}
	return r.finishLocked(success)
}

// unitFailed records a failure for a unit that failed without a run in flight:
// started elsewhere and gone before it was seen activating. It reports false
// when there is a run to close instead, or the verdict is failed already.
func (r *run) unitFailed() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.st.State != stateIdle {
		return false
	}
	now := nowRFC3339()
	r.st = RunState{State: stateFailed, Origin: originSystemd, StartedAt: now, FinishedAt: now}
	logger.Info("[upgrade] unit failed with no run seen, recording failed")
	return true
}

func (r *run) snapshot() RunState {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st
}

func (r *run) isRunning() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st.State == stateRunning
}

func (r *run) origin() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.st.Origin
}

func nowRFC3339() string { return time.Now().UTC().Format(time.RFC3339) }
