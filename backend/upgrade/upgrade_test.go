package upgrade

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

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

func TestUnconfiguredTriggersReturnError(t *testing.T) {
	u, _ := newStarted(t, validResult)
	if err := u.CheckNow(); err != ErrUnitNotConfigured {
		t.Fatalf("CheckNow err = %v, want ErrUnitNotConfigured", err)
	}
	if err := u.StartUpgrade(); err != ErrUnitNotConfigured {
		t.Fatalf("StartUpgrade err = %v, want ErrUnitNotConfigured", err)
	}
}
