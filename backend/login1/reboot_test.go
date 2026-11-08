package login1

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"
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
