package mpris

import (
	"errors"
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestTracksFromMetadata(t *testing.T) {
	ids := []dbus.ObjectPath{
		"/org/mpris/MediaPlayer2/track/1",
		"/org/mpris/MediaPlayer2/track/2",
		"/org/mpris/MediaPlayer2/track/3",
	}
	metas := []map[string]dbus.Variant{
		{
			"xesam:title":   dbus.MakeVariant("Song One"),
			"xesam:artist":  dbus.MakeVariant([]string{"Artist A"}),
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/1")),
			"custom:key":    dbus.MakeVariant("should be whitelisted out"),
		},
		{
			"xesam:title": dbus.MakeVariant("Song Two"),
		},
	}

	tracks := tracksFromMetadata(ids, metas)

	if len(tracks) != 3 {
		t.Fatalf("len(tracks) = %d, want 3", len(tracks))
	}
	for i, id := range ids {
		if tracks[i].TrackID != string(id) {
			t.Errorf("tracks[%d].TrackID = %q, want %q", i, tracks[i].TrackID, id)
		}
	}
	if tracks[0].Metadata["xesam:title"] != "Song One" {
		t.Errorf("tracks[0] title = %q, want %q", tracks[0].Metadata["xesam:title"], "Song One")
	}
	if tracks[1].Metadata["xesam:title"] != "Song Two" {
		t.Errorf("tracks[1] title = %q, want %q", tracks[1].Metadata["xesam:title"], "Song Two")
	}
	if _, ok := tracks[0].Metadata["custom:key"]; ok {
		t.Error("non-whitelisted metadata key should be dropped")
	}
	// metas shorter than ids: trailing track has no metadata
	if tracks[2].Metadata != nil {
		t.Errorf("tracks[2].Metadata = %v, want nil", tracks[2].Metadata)
	}
}

func TestTracksFromIDs(t *testing.T) {
	ids := []dbus.ObjectPath{
		"/org/mpris/MediaPlayer2/track/1",
		"/org/mpris/MediaPlayer2/track/2",
	}

	tracks := tracksFromIDs(ids)

	if len(tracks) != 2 {
		t.Fatalf("len(tracks) = %d, want 2", len(tracks))
	}
	for i, id := range ids {
		if tracks[i].TrackID != string(id) {
			t.Errorf("tracks[%d].TrackID = %q, want %q", i, tracks[i].TrackID, id)
		}
		if tracks[i].Metadata != nil {
			t.Errorf("tracks[%d].Metadata = %v, want nil", i, tracks[i].Metadata)
		}
	}

	if got := tracksFromIDs(nil); len(got) != 0 {
		t.Errorf("tracksFromIDs(nil) = %v, want empty", got)
	}
}

func TestTrackFromSignalMetadata(t *testing.T) {
	tests := []struct {
		name        string
		meta        map[string]dbus.Variant
		wantTrackID string
		wantTitle   string
	}{
		{
			name: "trackid as ObjectPath variant",
			meta: map[string]dbus.Variant{
				"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/org/mpris/MediaPlayer2/track/7")),
				"xesam:title":   dbus.MakeVariant("Signal Song"),
			},
			wantTrackID: "/org/mpris/MediaPlayer2/track/7",
			wantTitle:   "Signal Song",
		},
		{
			name: "trackid as plain string is not accepted as ID",
			meta: map[string]dbus.Variant{
				"mpris:trackid": dbus.MakeVariant("/org/mpris/MediaPlayer2/track/7"),
			},
			wantTrackID: "",
		},
		{
			name:        "missing trackid",
			meta:        map[string]dbus.Variant{"xesam:title": dbus.MakeVariant("No ID")},
			wantTrackID: "",
			wantTitle:   "No ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := trackFromSignalMetadata(tt.meta)
			if track.TrackID != tt.wantTrackID {
				t.Errorf("TrackID = %q, want %q", track.TrackID, tt.wantTrackID)
			}
			if tt.wantTitle != "" && track.Metadata["xesam:title"] != tt.wantTitle {
				t.Errorf("title = %q, want %q", track.Metadata["xesam:title"], tt.wantTitle)
			}
		})
	}
}

const testBus = "org.mpris.MediaPlayer2.test"

func newTracklistBackend(players ...Player) *MPRISBackend {
	b := &MPRISBackend{}
	b.players.Store(players)
	return b
}

func trackIDs(tracks []Track) []string {
	ids := make([]string, len(tracks))
	for i, tr := range tracks {
		ids[i] = tr.TrackID
	}
	return ids
}

func assertTrackIDs(t *testing.T, got []Track, want []string) {
	t.Helper()
	gotIDs := trackIDs(got)
	if len(gotIDs) != len(want) {
		t.Fatalf("track IDs = %v, want %v", gotIDs, want)
	}
	for i := range want {
		if gotIDs[i] != want[i] {
			t.Fatalf("track IDs = %v, want %v", gotIDs, want)
		}
	}
}

func TestReplaceTracklist(t *testing.T) {
	b := newTracklistBackend(Player{BusName: testBus})

	tracks := []Track{{TrackID: "/track/1"}, {TrackID: "/track/2"}}
	if err := b.ReplaceTracklist(testBus, tracks); err != nil {
		t.Fatalf("ReplaceTracklist failed: %v", err)
	}

	p, err := b.GetPlayerFromCache(testBus)
	if err != nil {
		t.Fatalf("GetPlayerFromCache: %v", err)
	}
	if !p.TracklistSupported {
		t.Error("ReplaceTracklist should mark the player as tracklist-supported")
	}
	assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})

	if err := b.ReplaceTracklist("org.mpris.MediaPlayer2.unknown", tracks); err == nil {
		t.Error("ReplaceTracklist on unknown player should error")
	}
}

func TestAddTrackToCache(t *testing.T) {
	initial := []Track{{TrackID: "/track/1"}, {TrackID: "/track/2"}}
	newTrack := Track{TrackID: "/track/new"}

	tests := []struct {
		name       string
		busName    string
		trackID    string // defaults to /track/new
		afterTrack string
		wantIDs    []string
		wantErr    bool
		noop       bool // duplicate signal: nothing stored, supported flag untouched
	}{
		{
			name:       "prepend via NoTrack sentinel",
			busName:    testBus,
			afterTrack: MPRIS_NO_TRACK,
			wantIDs:    []string{"/track/new", "/track/1", "/track/2"},
		},
		{
			name:       "duplicate TrackAdded for a cached ID is a no-op",
			busName:    testBus,
			trackID:    "/track/1",
			afterTrack: "/track/2",
			wantIDs:    []string{"/track/1", "/track/2"},
			noop:       true,
		},
		{
			name:       "insert after existing track",
			busName:    testBus,
			afterTrack: "/track/1",
			wantIDs:    []string{"/track/1", "/track/new", "/track/2"},
		},
		{
			name:       "unknown afterTrack appends",
			busName:    testBus,
			afterTrack: "/track/ghost",
			wantIDs:    []string{"/track/1", "/track/2", "/track/new"},
		},
		{
			name:       "unknown player errors",
			busName:    "org.mpris.MediaPlayer2.unknown",
			afterTrack: MPRIS_NO_TRACK,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := newTracklistBackend(Player{
				BusName:   testBus,
				Tracklist: append([]Track{}, initial...),
			})

			track := newTrack
			if tt.trackID != "" {
				track.TrackID = tt.trackID
			}
			err := b.AddTrackToCache(tt.busName, track, tt.afterTrack)
			if (err != nil) != tt.wantErr {
				t.Fatalf("AddTrackToCache error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}

			p, _ := b.GetPlayerFromCache(testBus)
			if p.TracklistSupported == tt.noop {
				t.Errorf("TracklistSupported = %v: an insert should set it, a no-op should store nothing", p.TracklistSupported)
			}
			assertTrackIDs(t, p.Tracklist, tt.wantIDs)
		})
	}
}

func TestRemoveTrackFromCache(t *testing.T) {
	b := newTracklistBackend(Player{
		BusName:   testBus,
		Tracklist: []Track{{TrackID: "/track/1"}, {TrackID: "/track/2"}},
	})

	if err := b.RemoveTrackFromCache(testBus, "/track/1"); err != nil {
		t.Fatalf("RemoveTrackFromCache failed: %v", err)
	}
	p, _ := b.GetPlayerFromCache(testBus)
	assertTrackIDs(t, p.Tracklist, []string{"/track/2"})

	// Unknown ID is a no-op
	if err := b.RemoveTrackFromCache(testBus, "/track/ghost"); err != nil {
		t.Fatalf("RemoveTrackFromCache with unknown ID failed: %v", err)
	}
	p, _ = b.GetPlayerFromCache(testBus)
	assertTrackIDs(t, p.Tracklist, []string{"/track/2"})
}

func TestUpdateTrackMetadataInCache(t *testing.T) {
	newBackend := func() *MPRISBackend {
		return newTracklistBackend(Player{
			BusName: testBus,
			Tracklist: []Track{
				{TrackID: "/track/1", Metadata: map[string]string{"xesam:title": "Old"}},
				{TrackID: "/track/2"},
			},
		})
	}

	t.Run("metadata updated, ID kept when new one is empty", func(t *testing.T) {
		b := newBackend()
		err := b.UpdateTrackMetadataInCache(testBus, "/track/1", Track{
			Metadata: map[string]string{"xesam:title": "New"},
		})
		if err != nil {
			t.Fatalf("UpdateTrackMetadataInCache failed: %v", err)
		}
		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
		if p.Tracklist[0].Metadata["xesam:title"] != "New" {
			t.Errorf("title = %q, want %q", p.Tracklist[0].Metadata["xesam:title"], "New")
		}
	})

	t.Run("trackid rename", func(t *testing.T) {
		b := newBackend()
		err := b.UpdateTrackMetadataInCache(testBus, "/track/1", Track{
			TrackID:  "/track/renamed",
			Metadata: map[string]string{"xesam:title": "New"},
		})
		if err != nil {
			t.Fatalf("UpdateTrackMetadataInCache failed: %v", err)
		}
		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/renamed", "/track/2"})
	})

	t.Run("unknown old ID is a no-op", func(t *testing.T) {
		b := newBackend()
		if err := b.UpdateTrackMetadataInCache(testBus, "/track/ghost", Track{TrackID: "/x"}); err != nil {
			t.Fatalf("UpdateTrackMetadataInCache failed: %v", err)
		}
		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
	})
}

func TestResolveTrackRef(t *testing.T) {
	tracks := []Track{
		{TrackID: "/org/mpris/MediaPlayer2/Track/7"},
		{TrackID: "/org/videolan/vlc/playlist/17"},
	}

	tests := []struct {
		name   string
		ref    string
		want   string
		wantOk bool
	}{
		{name: "full object path", ref: "/org/mpris/MediaPlayer2/Track/7", want: "/org/mpris/MediaPlayer2/Track/7", wantOk: true},
		{name: "last segment", ref: "17", want: "/org/videolan/vlc/playlist/17", wantOk: true},
		{name: "unknown ref", ref: "99", wantOk: false},
		{name: "partial path does not match", ref: "playlist/17", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := resolveTrackRef(tracks, tt.ref)
			if ok != tt.wantOk {
				t.Fatalf("resolveTrackRef(%q) ok = %v, want %v", tt.ref, ok, tt.wantOk)
			}
			if got != tt.want {
				t.Errorf("resolveTrackRef(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestUpdateCanEditTracks(t *testing.T) {
	b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true})

	if err := b.UpdateCanEditTracks(testBus, dbus.MakeVariant(true)); err != nil {
		t.Fatalf("UpdateCanEditTracks failed: %v", err)
	}
	p, _ := b.GetPlayerFromCache(testBus)
	if !p.CanEditTracks {
		t.Error("CanEditTracks should be true")
	}

	// Non-bool variant is ignored
	if err := b.UpdateCanEditTracks(testBus, dbus.MakeVariant("nope")); err != nil {
		t.Fatalf("UpdateCanEditTracks with non-bool failed: %v", err)
	}
	p, _ = b.GetPlayerFromCache(testBus)
	if !p.CanEditTracks {
		t.Error("non-bool variant should not change CanEditTracks")
	}
}

func TestGetTracklist(t *testing.T) {
	t.Run("unsupported player", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus})
		_, err := b.GetTracklist(testBus)
		var unsupported *TracklistUnsupportedError
		if !errors.As(err, &unsupported) {
			t.Fatalf("GetTracklist error = %v, want TracklistUnsupportedError", err)
		}
	})

	t.Run("supported with nil slice returns empty non-nil tracks", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true})
		resp, err := b.GetTracklist(testBus)
		if err != nil {
			t.Fatalf("GetTracklist failed: %v", err)
		}
		if resp.Tracks == nil {
			t.Error("Tracks should be non-nil so JSON is [] instead of null")
		}
		if len(resp.Tracks) != 0 {
			t.Errorf("len(Tracks) = %d, want 0", len(resp.Tracks))
		}
	})

	t.Run("full response", func(t *testing.T) {
		b := newTracklistBackend(Player{
			BusName:            testBus,
			TracklistSupported: true,
			CanEditTracks:      true,
			Tracklist:          []Track{{TrackID: "/track/1"}},
		})
		resp, err := b.GetTracklist(testBus)
		if err != nil {
			t.Fatalf("GetTracklist failed: %v", err)
		}
		if !resp.CanEditTracks {
			t.Error("CanEditTracks should be true")
		}
		assertTrackIDs(t, resp.Tracks, []string{"/track/1"})
	})
}

// Action guard tests run with a nil D-Bus conn: any guard passing through
// to callMethod would panic.

func TestGoToValidation(t *testing.T) {
	t.Run("unsupported player", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus})
		var unsupported *TracklistUnsupportedError
		if err := b.GoTo(testBus, "/track/1"); !errors.As(err, &unsupported) {
			t.Errorf("GoTo error = %v, want TracklistUnsupportedError", err)
		}
	})

	t.Run("unknown track ref", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true})
		var validation *ValidationError
		if err := b.GoTo(testBus, "99"); !errors.As(err, &validation) {
			t.Errorf("GoTo error = %v, want ValidationError", err)
		}
	})
}

func TestAddTrackValidation(t *testing.T) {
	t.Run("unsupported player", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus})
		var unsupported *TracklistUnsupportedError
		if err := b.AddTrack(testBus, "file:///a.mp3", "", false); !errors.As(err, &unsupported) {
			t.Errorf("AddTrack error = %v, want TracklistUnsupportedError", err)
		}
	})

	t.Run("cannot edit tracks", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true})
		var capability *CapabilityError
		if err := b.AddTrack(testBus, "file:///a.mp3", "", false); !errors.As(err, &capability) {
			t.Errorf("AddTrack error = %v, want CapabilityError", err)
		}
	})

	t.Run("empty uri", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true, CanEditTracks: true})
		var validation *ValidationError
		if err := b.AddTrack(testBus, "", "", false); !errors.As(err, &validation) {
			t.Errorf("AddTrack error = %v, want ValidationError", err)
		}
	})

	t.Run("bare path uri (no scheme)", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true, CanEditTracks: true})
		var validation *ValidationError
		if err := b.AddTrack(testBus, "B' Side/track.mp3", "", false); !errors.As(err, &validation) {
			t.Errorf("AddTrack error = %v, want ValidationError", err)
		}
	})

	t.Run("scheme not in SupportedUriSchemes", func(t *testing.T) {
		b := newTracklistBackend(Player{
			BusName:             testBus,
			TracklistSupported:  true,
			CanEditTracks:       true,
			SupportedUriSchemes: []string{"file"},
		})
		var validation *ValidationError
		if err := b.AddTrack(testBus, "http://example.com/a.mp3", "", false); !errors.As(err, &validation) {
			t.Errorf("AddTrack error = %v, want ValidationError", err)
		}
	})

	t.Run("unknown afterTrack", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true, CanEditTracks: true})
		var validation *ValidationError
		if err := b.AddTrack(testBus, "file:///a.mp3", "99", false); !errors.As(err, &validation) {
			t.Errorf("AddTrack error = %v, want ValidationError", err)
		}
	})
}

func TestRemoveTrackValidation(t *testing.T) {
	t.Run("unsupported player", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus})
		var unsupported *TracklistUnsupportedError
		if err := b.RemoveTrack(testBus, "/track/1"); !errors.As(err, &unsupported) {
			t.Errorf("RemoveTrack error = %v, want TracklistUnsupportedError", err)
		}
	})

	t.Run("cannot edit tracks", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true})
		var capability *CapabilityError
		if err := b.RemoveTrack(testBus, "/track/1"); !errors.As(err, &capability) {
			t.Errorf("RemoveTrack error = %v, want CapabilityError", err)
		}
	})

	t.Run("unknown track ref", func(t *testing.T) {
		b := newTracklistBackend(Player{BusName: testBus, TracklistSupported: true, CanEditTracks: true})
		var validation *ValidationError
		if err := b.RemoveTrack(testBus, "99"); !errors.As(err, &validation) {
			t.Errorf("RemoveTrack error = %v, want ValidationError", err)
		}
	})
}

func TestListenerTracklistSignals(t *testing.T) {
	const uniqueName = ":1.42"

	newListener := func() (*Listener, *MPRISBackend) {
		b := newTracklistBackend(Player{
			BusName:            testBus,
			uniqueName:         uniqueName,
			TracklistSupported: true,
			Tracklist:          []Track{{TrackID: "/track/1"}, {TrackID: "/track/2"}},
		})
		return &Listener{backend: b}, b
	}

	signal := func(name string, body ...interface{}) *dbus.Signal {
		return &dbus.Signal{Sender: uniqueName, Path: MPRIS_PATH, Name: name, Body: body}
	}

	t.Run("TrackAdded prepends via NoTrack", func(t *testing.T) {
		l, b := newListener()
		meta := map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/track/new")),
		}
		l.handleSignal(signal(MPRIS_SIGNAL_TRACK_ADDED, meta, dbus.ObjectPath(MPRIS_NO_TRACK)))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/new", "/track/1", "/track/2"})
	})

	t.Run("TrackRemoved", func(t *testing.T) {
		l, b := newListener()
		l.handleSignal(signal(MPRIS_SIGNAL_TRACK_REMOVED, dbus.ObjectPath("/track/1")))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/2"})
	})

	t.Run("TrackMetadataChanged", func(t *testing.T) {
		l, b := newListener()
		meta := map[string]dbus.Variant{
			"mpris:trackid": dbus.MakeVariant(dbus.ObjectPath("/track/1")),
			"xesam:title":   dbus.MakeVariant("Fresh"),
		}
		l.handleSignal(signal(MPRIS_SIGNAL_TRACK_METADATA_CHANGED, dbus.ObjectPath("/track/1"), meta))

		p, _ := b.GetPlayerFromCache(testBus)
		if p.Tracklist[0].Metadata["xesam:title"] != "Fresh" {
			t.Errorf("title = %q, want %q", p.Tracklist[0].Metadata["xesam:title"], "Fresh")
		}
	})

	t.Run("TrackListReplaced with empty list clears", func(t *testing.T) {
		l, b := newListener()
		l.handleSignal(signal(MPRIS_SIGNAL_TRACKLIST_REPLACED, []dbus.ObjectPath{}, dbus.ObjectPath(MPRIS_NO_TRACK)))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{})
	})

	t.Run("unknown sender is dropped", func(t *testing.T) {
		l, b := newListener()
		sig := signal(MPRIS_SIGNAL_TRACK_REMOVED, dbus.ObjectPath("/track/1"))
		sig.Sender = ":9.99"
		l.handleSignal(sig)

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
	})

	t.Run("malformed bodies are ignored", func(t *testing.T) {
		l, b := newListener()
		l.handleSignal(signal(MPRIS_SIGNAL_TRACK_ADDED, "not-a-map"))
		l.handleSignal(signal(MPRIS_SIGNAL_TRACK_REMOVED))
		l.handleSignal(signal(MPRIS_SIGNAL_TRACKLIST_REPLACED, "not-ids", dbus.ObjectPath("/x")))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
	})

	t.Run("PropertiesChanged on TrackList iface updates CanEditTracks", func(t *testing.T) {
		l, b := newListener()
		// Tracks matching the cache must be skipped (dedup for players that
		// emit both TrackListReplaced and the property change): with a nil
		// conn, going through the refresh would panic on the metadata fetch.
		l.handleSignal(signal(DBUS_PROP_CHANGED_SIGNAL, MPRIS_TRACKLIST_IFACE, map[string]dbus.Variant{
			"CanEditTracks": dbus.MakeVariant(true),
			"Tracks":        dbus.MakeVariant([]dbus.ObjectPath{"/track/1", "/track/2"}),
		}, []string{}))

		p, _ := b.GetPlayerFromCache(testBus)
		if !p.CanEditTracks {
			t.Error("CanEditTracks should be true")
		}
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
	})

	t.Run("PropertiesChanged with changed Tracks refreshes the list (VLC style)", func(t *testing.T) {
		l, b := newListener()
		l.handleSignal(signal(DBUS_PROP_CHANGED_SIGNAL, MPRIS_TRACKLIST_IFACE, map[string]dbus.Variant{
			"Tracks": dbus.MakeVariant([]dbus.ObjectPath{}),
		}, []string{}))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{})
	})

	t.Run("unrelated invalidated property does not refetch", func(t *testing.T) {
		l, b := newListener()
		// With a nil conn, a refetch would panic — not reaching D-Bus is the assertion.
		l.handleSignal(signal(DBUS_PROP_CHANGED_SIGNAL, MPRIS_TRACKLIST_IFACE,
			map[string]dbus.Variant{}, []string{"CanEditTracks"}))

		p, _ := b.GetPlayerFromCache(testBus)
		assertTrackIDs(t, p.Tracklist, []string{"/track/1", "/track/2"})
	})
}

func TestTracklistUnsupportedError(t *testing.T) {
	err := &TracklistUnsupportedError{BusName: "org.mpris.MediaPlayer2.spotify"}
	expected := "tracklist not supported: org.mpris.MediaPlayer2.spotify"
	if err.Error() != expected {
		t.Errorf("TracklistUnsupportedError.Error() = %q, want %q", err.Error(), expected)
	}
}
