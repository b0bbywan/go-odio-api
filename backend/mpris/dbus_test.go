package mpris

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/config"
)

// silentTracklistPlayer mimics Kodi: it answers the two mandatory interfaces
// but never replies to a GetAll on TrackList.
type silentTracklistPlayer struct {
	hang chan struct{}
}

func (p *silentTracklistPlayer) GetAll(iface string) (map[string]dbus.Variant, *dbus.Error) {
	switch iface {
	case MPRIS_INTERFACE:
		return map[string]dbus.Variant{"Identity": dbus.MakeVariant("Silent")}, nil
	case MPRIS_PLAYER_IFACE:
		return map[string]dbus.Variant{
			"PlaybackStatus": dbus.MakeVariant("Playing"),
			"CanPlay":        dbus.MakeVariant(true),
		}, nil
	}
	<-p.hang
	return nil, dbus.MakeFailedError(errors.New("unknown interface"))
}

func (p *silentTracklistPlayer) Get(iface, prop string) (dbus.Variant, *dbus.Error) {
	<-p.hang
	return dbus.Variant{}, dbus.MakeFailedError(errors.New("unknown interface"))
}

// exportSilentPlayer registers the fake player on the session bus under a
// unique MPRIS name. Skips the test when no session bus is reachable.
func exportSilentPlayer(t *testing.T) string {
	t.Helper()
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })

	hang := make(chan struct{})
	t.Cleanup(func() { close(hang) })
	if err := conn.Export(&silentTracklistPlayer{hang: hang}, MPRIS_PATH, DBUS_PROP_IFACE); err != nil {
		t.Fatalf("Export: %v", err)
	}

	name := fmt.Sprintf("%s.odio_test_%d_%d", MPRIS_PREFIX, os.Getpid(), time.Now().UnixNano())
	reply, err := conn.RequestName(name, dbus.NameFlagDoNotQueue)
	if err != nil || reply != dbus.RequestNameReplyPrimaryOwner {
		t.Fatalf("RequestName(%s) = %v, %v", name, reply, err)
	}
	return name
}

func startBackend(t *testing.T, timeout time.Duration) *MPRISBackend {
	t.Helper()
	b, err := New(context.Background(), &config.MPRISConfig{Enabled: true, Timeout: timeout})
	if err != nil {
		t.Skipf("no session bus: %v", err)
	}

	done := make(chan error, 1)
	go func() { done <- b.Start() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start blocked on the silent TrackList probe")
	}
	return b
}

func findPlayer(t *testing.T, b *MPRISBackend, name string) *Player {
	t.Helper()
	players, err := b.ListPlayers()
	if err != nil {
		t.Fatalf("ListPlayers: %v", err)
	}
	for i := range players {
		if players[i].BusName == name {
			return &players[i]
		}
	}
	return nil
}

// A player that never answers the optional TrackList probe must still be
// listed, without tracklist support, once the call deadline expires.
func TestSilentTracklistPlayerIsListed(t *testing.T) {
	name := exportSilentPlayer(t)
	b := startBackend(t, 100*time.Millisecond)
	defer b.Close()

	p := findPlayer(t, b, name)
	if p == nil {
		t.Fatalf("player %s not listed", name)
	}
	if p.TracklistSupported {
		t.Error("TracklistSupported = true, want false")
	}
	if p.Identity != "Silent" {
		t.Errorf("Identity = %q, want Silent", p.Identity)
	}
}

func TestCallTimeoutError(t *testing.T) {
	name := exportSilentPlayer(t)
	b := startBackend(t, 50*time.Millisecond)
	defer b.Close()

	start := time.Now()
	_, err := b.getProperty(name, MPRIS_TRACKLIST_IFACE, "Tracks")
	var timeoutErr *dbusTimeoutError
	if !errors.As(err, &timeoutErr) {
		t.Fatalf("getProperty error = %v, want dbusTimeoutError", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("call took %s, deadline not enforced", elapsed)
	}
}

// Close must wait for the listener's in-flight handler: a player appearing
// right before shutdown used to reach notify() after the events channel was
// closed and panic the process.
func TestCloseDuringInFlightReload(t *testing.T) {
	b := startBackend(t, 500*time.Millisecond)

	// Appears after Start so it is handled by the listener, not ListPlayers.
	exportSilentPlayer(t)
	time.Sleep(50 * time.Millisecond)

	closed := make(chan struct{})
	go func() {
		b.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(3 * time.Second):
		t.Fatal("Close did not return")
	}
	for {
		select {
		case _, ok := <-b.Events():
			if !ok {
				return
			}
		case <-time.After(time.Second):
			t.Fatal("events channel still open after Close")
		}
	}
}
