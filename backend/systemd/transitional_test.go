package systemd

import (
	"slices"
	"testing"
)

func TestTransitionalUnits(t *testing.T) {
	services := []Service{
		{Name: "odio-screen.service", Scope: ScopeUser, ActiveState: "activating"},
		{Name: "mpd.service", Scope: ScopeUser, ActiveState: "active"},
		{Name: "spotifyd.service", Scope: ScopeUser, ActiveState: "failed"},
		{Name: "qbzd.service", Scope: ScopeUser, ActiveState: "deactivating"},
		{Name: "bluetooth.service", Scope: ScopeSystem, ActiveState: "activating"},
		{Name: "unwatched.service", Scope: ScopeUser, ActiveState: "reloading"},
	}
	watched := map[string]bool{
		"odio-screen.service": true,
		"mpd.service":         true,
		"spotifyd.service":    true,
		"qbzd.service":        true,
		"bluetooth.service":   true,
	}

	got := transitionalUnits(services, watched)
	want := []string{"odio-screen.service", "qbzd.service"}
	if !slices.Equal(got, want) {
		t.Errorf("transitionalUnits = %v, want %v", got, want)
	}
}

func TestTrackTransitionalLeavesDBusModeToSignals(t *testing.T) {
	l := &Listener{
		supportsUTMP: true,
		userWatched:  map[string]bool{"odio-screen.service": true},
	}

	l.trackTransitional([]Service{{Name: "odio-screen.service", Scope: ScopeUser, ActiveState: "activating"}})

	if _, tracked := l.watching["odio-screen.service"]; tracked {
		t.Error("odio-screen.service tracked in D-Bus mode, want signals to cover it")
	}
}

func TestTrackMarksARunningWatch(t *testing.T) {
	// A restart: the stop event started the watch, the start event must not be lost.
	l := &Listener{watching: map[string]bool{"odio-screen.service": false}}

	l.track("odio-screen.service")

	if marked := l.watching["odio-screen.service"]; !marked {
		t.Error("running watch not marked, the start event would be dropped")
	}
}

func TestSettleEndsAQuietWatch(t *testing.T) {
	l := &Listener{watching: map[string]bool{"odio-screen.service": false}}

	if !l.settle("odio-screen.service") {
		t.Fatal("settle = false, want true with no event since")
	}
	if _, tracked := l.watching["odio-screen.service"]; tracked {
		t.Error("settled watch still tracked")
	}
}

func TestSettleReadsAgainAfterAnEvent(t *testing.T) {
	l := &Listener{watching: map[string]bool{"odio-screen.service": true}}

	if l.settle("odio-screen.service") {
		t.Fatal("settle = true, want false: an event came in meanwhile")
	}
	if marked, tracked := l.watching["odio-screen.service"]; !tracked || marked {
		t.Errorf("watching = %v, want still tracked with the mark cleared", l.watching)
	}
	if !l.settle("odio-screen.service") {
		t.Error("second settle = false, want true once the event was read")
	}
}
