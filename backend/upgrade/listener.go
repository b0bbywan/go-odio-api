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

// startListener serves the unix socket the upgrade script streams progress to. Non-fatal on failure.
func (u *UpgradeBackend) startListener() {
	if u.progressSocket == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(u.progressSocket), 0o700); err != nil {
		logger.Warn("[upgrade] cannot create progress socket dir, progress disabled: %v", err)
		return
	}
	// remove a leftover socket so Listen doesn't fail with EADDRINUSE.
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

// accept serves one connection at a time (a single upgrade runs at a time).
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

// readProgress relays each newline-delimited JSON line as an upgrade.progress event.
func (u *UpgradeBackend) readProgress(conn net.Conn) {
	// closed here and from the ctx goroutine; Once avoids a double-close log.
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

// progressLine is the begin/progress/end contract; typed pointers reject absence and wrong type.
type progressLine struct {
	Event   *string `json:"event"`
	Total   *int    `json:"total"`
	Percent *int    `json:"percent"`
	Current *int    `json:"current"`
	Step    *string `json:"step"`
	Success *bool   `json:"success"`
}

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

// applyRunProgress advances the live run; a CLI run (claimed here as sourceStream) is also driven to running/finished, a unit run only has its live state refreshed.
func (u *UpgradeBackend) applyRunProgress(p progressLine) {
	switch *p.Event {
	case "begin":
		zero := 0
		if u.run.progress(sourceStream, &zero, nil) {
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
		}
	case "progress":
		if u.run.progress(sourceStream, p.Percent, p.Step) {
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "running"}})
		}
	case "end":
		if u.run.finish(sourceStream) {
			u.notify(events.Event{Type: events.TypeUpgradeInfo, Data: RunState{State: "finished", Success: p.Success}})
		}
	}
}
