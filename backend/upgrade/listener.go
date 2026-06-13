package upgrade

import (
	"bufio"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"sync"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// startListener listens on a unix socket the upgrade script connects to during a
// run. Progress is ephemeral, so it streams over a socket rather than a file: on
// SD-card systems that avoids write wear. A listen failure is non-fatal.
func (u *UpgradeBackend) startListener() {
	if u.progressSocket == "" {
		return
	}
	// The runtime subdir (e.g. $XDG_RUNTIME_DIR/odio-api) may not exist yet.
	if err := os.MkdirAll(filepath.Dir(u.progressSocket), 0o700); err != nil {
		logger.Warn("[upgrade] cannot create progress socket dir, progress disabled: %v", err)
		return
	}
	// A leftover socket from an unclean shutdown makes Listen fail with EADDRINUSE.
	if err := os.Remove(u.progressSocket); err != nil && !os.IsNotExist(err) {
		logger.Warn("[upgrade] removing stale progress socket: %v", err)
	}
	l, err := net.Listen("unix", u.progressSocket)
	if err != nil {
		logger.Warn("[upgrade] cannot listen on %s, progress disabled: %v", u.progressSocket, err)
		return
	}
	u.listener = l
	u.wg.Add(1)
	go u.accept()
}

// accept serves one connection at a time: a single upgrade runs at a time, and
// the script holds the connection open for the whole run.
func (u *UpgradeBackend) accept() {
	defer u.wg.Done()
	for {
		conn, err := u.listener.Accept()
		if err != nil {
			return // listener closed
		}
		u.readProgress(conn)
	}
}

// readProgress relays each newline-delimited JSON object the script writes as an
// upgrade.info event, mirroring readResult's pass-through: the backend stays
// agnostic of the progress payload. ctx cancellation closes the connection so
// the read unblocks on shutdown.
func (u *UpgradeBackend) readProgress(conn net.Conn) {
	// conn is closed both here and from the ctx goroutine; Once keeps it to a
	// single checked close so shutdown does not log a spurious double-close.
	var once sync.Once
	closeConn := func() {
		once.Do(func() {
			if err := conn.Close(); err != nil {
				logger.Warn("[upgrade] closing progress connection: %v", err)
			}
		})
	}
	defer closeConn()
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-u.ctx.Done():
			closeConn()
		case <-done:
		}
	}()

	scanner := bufio.NewScanner(conn)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		p, ok := parseProgress(line)
		if !ok {
			logger.Warn("[upgrade] progress line does not match the event contract, ignoring")
			continue
		}
		u.applyRunProgress(p)
		raw := make(json.RawMessage, len(line)) // scanner reuses its buffer; copy before async send
		copy(raw, line)
		u.notify(events.Event{Type: events.TypeUpgradeProgress, Data: raw})
	}
}

// progressLine is the begin/progress/end contract odio-api relies on; the
// ansible-flavoured roles/changed fields are not required and pass through
// verbatim. Typed pointers reject both absence and wrong type.
type progressLine struct {
	Event   *string `json:"event"`
	Total   *int    `json:"total"`
	Percent *int    `json:"percent"`
	Current *int    `json:"current"`
	Step    *string `json:"step"`
	Success *bool   `json:"success"`
}

// parseProgress parses and validates a streamed progress line.
func parseProgress(line []byte) (progressLine, bool) {
	var p progressLine
	if err := json.Unmarshal(line, &p); err != nil || p.Event == nil {
		return p, false
	}
	switch *p.Event {
	case "begin":
		return p, p.Total != nil
	case "progress":
		return p, p.Percent != nil && p.Current != nil && p.Step != nil
	case "end":
		return p, p.Success != nil
	default:
		return p, false
	}
}

// applyRunProgress advances the live run state from a streamed progress line so
// the status endpoint reflects it. "begin" resets to 0%, "progress" carries the
// percent/step; "end" leaves the run flagged until the unit reaches its terminal
// state (which clears it).
func (u *UpgradeBackend) applyRunProgress(p progressLine) {
	switch *p.Event {
	case "begin":
		zero := 0
		u.runState.Store(&RunState{State: "running", Percent: &zero})
	case "progress":
		u.runState.Store(&RunState{State: "running", Percent: p.Percent, Step: p.Step})
	}
}
