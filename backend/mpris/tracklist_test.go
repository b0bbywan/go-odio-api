package mpris

import (
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

func TestTracklistUnsupportedError(t *testing.T) {
	err := &TracklistUnsupportedError{BusName: "org.mpris.MediaPlayer2.spotify"}
	expected := "tracklist not supported: org.mpris.MediaPlayer2.spotify"
	if err.Error() != expected {
		t.Errorf("TracklistUnsupportedError.Error() = %q, want %q", err.Error(), expected)
	}
}
