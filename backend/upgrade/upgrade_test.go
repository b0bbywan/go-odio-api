package upgrade

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

// fakeStream is a minimal events.Stream: the test pushes events onto ch, which
// the backend reads through its subscription.
type fakeStream struct{ ch chan events.Event }

func (f *fakeStream) SubscribeFunc(func(events.Event) bool) chan events.Event { return f.ch }
func (f *fakeStream) Unsubscribe(ch chan events.Event)                        { close(ch) }

const validResult = `{"current":"dev","latest":"2026.6.0b1","upgrade_available":true}`

func writeFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// atomicWrite mimics how a detector publishes the file: write a temp then
// rename over the target, which replaces the inode (the case the dir watch
// must catch).
func atomicWrite(t *testing.T, path, data string) {
	t.Helper()
	tmp := path + ".tmp"
	writeFile(t, tmp, data)
	if err := os.Rename(tmp, path); err != nil {
		t.Fatalf("rename %s: %v", tmp, err)
	}
}

func recv(t *testing.T, ch <-chan events.Event, d time.Duration) (events.Event, bool) {
	t.Helper()
	select {
	case e := <-ch:
		return e, true
	case <-time.After(d):
		return events.Event{}, false
	}
}

// newStarted returns a started backend pointed at a fresh result file path,
// and drains the event emitted by the initial read (when the file pre-exists).
func newStarted(t *testing.T, initial string) (*UpgradeBackend, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "upgrades.json")
	if initial != "" {
		writeFile(t, path, initial)
	}
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{Enabled: true, ResultFile: path}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if u == nil {
		t.Fatal("New returned nil for an enabled config")
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if initial != "" {
		recv(t, u.Events(), time.Second) // drain initial-read event
	}
	return u, path
}

func TestNewDisabled(t *testing.T) {
	ctx := context.Background()
	cases := map[string]*config.UpgradeConfig{
		"nil":            nil,
		"disabled":       {Enabled: false, ResultFile: "/x"},
		"no result file": {Enabled: true, ResultFile: ""},
	}
	for name, cfg := range cases {
		t.Run(name, func(t *testing.T) {
			u, err := New(ctx, cfg, nil)
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if u != nil {
				t.Fatalf("expected nil backend, got %#v", u)
			}
		})
	}
}

func TestInitialReadExposesStatus(t *testing.T) {
	u, _ := newStarted(t, validResult)
	if got := string(u.GetStatus()); got != validResult {
		t.Fatalf("GetStatus = %q, want %q", got, validResult)
	}
}

func TestWatchEmitsOnAtomicRewrite(t *testing.T) {
	u, path := newStarted(t, validResult)

	updated := `{"current":"dev","latest":"2026.7.0","upgrade_available":true}`
	atomicWrite(t, path, updated)

	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info event after rewrite, got none")
	}
	if e.Type != events.TypeUpgradeInfo {
		t.Fatalf("event type = %q, want %q", e.Type, events.TypeUpgradeInfo)
	}
	if got := string(u.GetStatus()); got != updated {
		t.Fatalf("GetStatus = %q, want %q", got, updated)
	}
}

func TestInvalidResultKeepsLastValid(t *testing.T) {
	u, path := newStarted(t, validResult)

	// Missing required "latest" field → must be rejected.
	atomicWrite(t, path, `{"current":"dev","upgrade_available":true}`)

	if _, ok := recv(t, u.Events(), 500*time.Millisecond); ok {
		t.Fatal("invalid result should not emit an event")
	}
	if got := string(u.GetStatus()); got != validResult {
		t.Fatalf("GetStatus = %q, want last valid %q", got, validResult)
	}
}

func TestProgressSocketRelaysLines(t *testing.T) {
	dir := t.TempDir()
	// A subdir that does not exist yet: startListener must create it (like the
	// default $XDG_RUNTIME_DIR/odio-api).
	sock := filepath.Join(dir, "odio-api", "upgrade.sock")

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:        true,
		ResultFile:     filepath.Join(dir, "upgrades.json"),
		ProgressSocket: sock,
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	conn, err := net.Dial("unix", sock)
	if err != nil {
		t.Fatalf("dial progress socket: %v", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			t.Logf("closing conn: %v", err)
		}
	}()

	// Missing the required current/step → must be rejected, no event.
	if _, err := conn.Write([]byte(`{"event":"progress","percent":10}` + "\n")); err != nil {
		t.Fatalf("write malformed: %v", err)
	}
	if _, ok := recv(t, u.Events(), 300*time.Millisecond); ok {
		t.Fatal("malformed progress line should not emit an event")
	}

	// Valid event with an extra ansible-flavoured field, which must pass through.
	progress := `{"event":"progress","percent":42,"current":1,"step":"mpd","changed":3}`
	if _, err := conn.Write([]byte(progress + "\n")); err != nil {
		t.Fatalf("write progress: %v", err)
	}
	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info event from the socket, got none")
	}
	if e.Type != events.TypeUpgradeInfo {
		t.Fatalf("event type = %q, want %q", e.Type, events.TypeUpgradeInfo)
	}
	if got := string(e.Data.(json.RawMessage)); got != progress {
		t.Fatalf("event data = %q, want verbatim %q", got, progress)
	}
}

func TestBusTerminalStateEmitsFinished(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "upgrades.json")
	writeFile(t, path, validResult)

	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	u, err := New(ctx, &config.UpgradeConfig{
		Enabled:     true,
		ResultFile:  path,
		UpgradeUnit: "odio-upgrade.service",
	}, nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	stream := &fakeStream{ch: make(chan events.Event, 8)}
	u.UseEventStream(stream)
	t.Cleanup(u.Close)
	if err := u.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	recv(t, u.Events(), time.Second) // drain the initial result-read event

	// Simulate an in-progress run, then its unit reaching a terminal success state.
	u.running.Store(true)
	stream.ch <- events.Event{
		Type: events.TypeServiceUpdated,
		Data: systemd.Service{Name: "odio-upgrade.service", Scope: systemd.ScopeUser, ActiveState: "inactive"},
	}

	e, ok := recv(t, u.Events(), 2*time.Second)
	if !ok {
		t.Fatal("expected an upgrade.info finished event, got none")
	}
	prog, ok := e.Data.(Progress)
	if !ok {
		t.Fatalf("event data = %T, want Progress", e.Data)
	}
	if prog.State != "finished" || prog.Success == nil || !*prog.Success {
		t.Fatalf("got %+v, want state=finished success=true", prog)
	}
}

func TestUnconfiguredTriggersReturnError(t *testing.T) {
	u, _ := newStarted(t, validResult)
	if err := u.CheckNow(); err != ErrUnitNotConfigured {
		t.Fatalf("CheckNow err = %v, want ErrUnitNotConfigured", err)
	}
	if err := u.StartUpgrade(); err != ErrUnitNotConfigured {
		t.Fatalf("StartUpgrade err = %v, want ErrUnitNotConfigured", err)
	}
}
