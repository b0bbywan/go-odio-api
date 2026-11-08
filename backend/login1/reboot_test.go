package login1

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/events"
)

func newRebootBackend(t *testing.T, dir string) *Login1Backend {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	flag := filepath.Join(dir, "reboot-required")
	return &Login1Backend{
		ctx:        ctx,
		rebootFile: flag,
		rebootPkgs: flag + ".pkgs",
		eventsC:    make(chan events.Event, 4),
	}
}

func startReboot(t *testing.T, l *Login1Backend) {
	t.Helper()
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(l.Close)
}

func touch(t *testing.T, path, data string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// appendLine mimics the dpkg hooks: `echo "$pkg" >> reboot-required.pkgs`.
func appendLine(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatalf("append %s: %v", path, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("close %s: %v", path, err)
	}
}

func eventually(t *testing.T, l *Login1Backend, want RebootRequired) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := l.RebootRequired()
		if got.Required == want.Required && slices.Equal(got.Packages, want.Packages) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("RebootRequired = %+v, want %+v", got, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestRebootRequired_DisabledWithoutFile(t *testing.T) {
	l := &Login1Backend{ctx: context.Background()}
	startReboot(t, l)

	if l.watcher != nil {
		t.Error("watcher started without a reboot-required file")
	}
	if got := l.RebootRequired(); got.Required || got.Packages != nil {
		t.Errorf("RebootRequired = %+v, want zero", got)
	}
}

func TestRebootRequired_InitialRead(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootFile, "")
	touch(t, l.rebootPkgs, "linux-image-6.12.57+deb13-amd64\ndbus\n")
	startReboot(t, l)

	got := l.RebootRequired()
	want := []string{"linux-image-6.12.57+deb13-amd64", "dbus"}
	if !got.Required || !slices.Equal(got.Packages, want) {
		t.Errorf("RebootRequired = %+v, want required with %v", got, want)
	}
}

func TestRebootRequired_FlagWithoutPkgs(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootFile, "")
	startReboot(t, l)

	if got := l.RebootRequired(); !got.Required || len(got.Packages) != 0 {
		t.Errorf("RebootRequired = %+v, want required with no packages", got)
	}
}

// A leftover .pkgs without the flag must not raise the warning.
func TestRebootRequired_PkgsWithoutFlag(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootPkgs, "dbus\n")
	startReboot(t, l)

	if got := l.RebootRequired(); got.Required || got.Packages != nil {
		t.Errorf("RebootRequired = %+v, want not required", got)
	}
}

func TestRebootRequired_BlankLinesIgnored(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootFile, "")
	touch(t, l.rebootPkgs, "\n dbus \n\n")
	startReboot(t, l)

	if got := l.RebootRequired(); !slices.Equal(got.Packages, []string{"dbus"}) {
		t.Errorf("Packages = %q, want [dbus]", got.Packages)
	}
}

// Hooks that append without checking (dnf actions, pacman) list a package once
// per upgrade.
func TestRebootRequired_DuplicatePackagesListedOnce(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootFile, "")
	touch(t, l.rebootPkgs, "kernel-core\nglibc\nkernel-core\n")
	startReboot(t, l)

	if got := l.RebootRequired(); !slices.Equal(got.Packages, []string{"kernel-core", "glibc"}) {
		t.Errorf("Packages = %q, want [kernel-core glibc]", got.Packages)
	}
}

func TestRebootRequired_WatchesHookSequence(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	startReboot(t, l)

	touch(t, l.rebootFile, "")
	eventually(t, l, RebootRequired{Required: true})

	appendLine(t, l.rebootPkgs, "linux-image-6.12.57+deb13-amd64")
	eventually(t, l, RebootRequired{Required: true, Packages: []string{"linux-image-6.12.57+deb13-amd64"}})

	appendLine(t, l.rebootPkgs, "dbus")
	eventually(t, l, RebootRequired{Required: true, Packages: []string{"linux-image-6.12.57+deb13-amd64", "dbus"}})
}

func TestRebootRequired_WatchesRemoval(t *testing.T) {
	dir := t.TempDir()
	l := newRebootBackend(t, dir)
	touch(t, l.rebootFile, "")
	touch(t, l.rebootPkgs, "dbus\n")
	startReboot(t, l)

	if err := os.Remove(l.rebootFile); err != nil {
		t.Fatal(err)
	}
	eventually(t, l, RebootRequired{})
}

func TestRebootRequired_MissingDirIsNonFatal(t *testing.T) {
	l := newRebootBackend(t, filepath.Join(t.TempDir(), "missing"))
	startReboot(t, l)

	if l.watcher != nil {
		t.Error("watcher should not be kept when its dir cannot be watched")
	}
	if got := l.RebootRequired(); got.Required {
		t.Errorf("RebootRequired = %+v, want not required", got)
	}
}

func TestRebootRequired_CloseStopsWatcher(t *testing.T) {
	l := newRebootBackend(t, t.TempDir())
	if err := l.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	done := make(chan struct{})
	go func() {
		l.Close()
		l.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}

func recvEvent(t *testing.T, l *Login1Backend, d time.Duration) (events.Event, bool) {
	t.Helper()
	select {
	case e := <-l.Events():
		return e, true
	case <-time.After(d):
		return events.Event{}, false
	}
}

func TestStatus_JSON(t *testing.T) {
	tests := []struct {
		name string
		flag bool
		pkgs string
		want string
	}{
		{"no reboot required", false, "", `{"reboot":true,"power_off":false,"reboot_required":false,"reboot_required_by":[]}`},
		{"flag without pkgs", true, "", `{"reboot":true,"power_off":false,"reboot_required":true,"reboot_required_by":[]}`},
		{"flag with pkgs", true, "linux-image-6.12.57+deb13-amd64\ndbus\n",
			`{"reboot":true,"power_off":false,"reboot_required":true,"reboot_required_by":["linux-image-6.12.57+deb13-amd64","dbus"]}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := newRebootBackend(t, t.TempDir())
			l.CanReboot = true
			if tt.flag {
				touch(t, l.rebootFile, "")
				touch(t, l.rebootPkgs, tt.pkgs)
			}
			startReboot(t, l)

			got, err := json.Marshal(l.Status())
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tt.want {
				t.Errorf("Status JSON = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestRebootRequired_NoEventWhenNothingChanged(t *testing.T) {
	l := newRebootBackend(t, t.TempDir())
	startReboot(t, l)

	if e, ok := recvEvent(t, l, 100*time.Millisecond); ok {
		t.Errorf("unexpected event at start: %+v", e)
	}
}

func TestRebootRequired_EmitsPowerUpdated(t *testing.T) {
	l := newRebootBackend(t, t.TempDir())
	l.CanPoweroff = true
	startReboot(t, l)

	touch(t, l.rebootFile, "")
	e, ok := recvEvent(t, l, 2*time.Second)
	if !ok {
		t.Fatal("no event after the flag was created")
	}
	if e.Type != events.TypePowerUpdated {
		t.Fatalf("event type = %s, want %s", e.Type, events.TypePowerUpdated)
	}
	got, isStatus := e.Data.(PowerStatus)
	if !isStatus {
		t.Fatalf("event data = %T, want PowerStatus", e.Data)
	}
	if !got.RebootRequired || !got.PowerOff {
		t.Errorf("event data = %+v, want reboot_required and power_off", got)
	}

	if err := os.Remove(l.rebootFile); err != nil {
		t.Fatal(err)
	}
	e, ok = recvEvent(t, l, 2*time.Second)
	if !ok {
		t.Fatal("no event after the flag was removed")
	}
	if got := e.Data.(PowerStatus); got.RebootRequired {
		t.Errorf("event data = %+v, want reboot not required", got)
	}
}
