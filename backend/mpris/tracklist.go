package mpris

import (
	"net/url"
	"path"
	"slices"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// resolveTrackRef resolves an API-supplied track reference — a full object
// path or just its last segment — against the cached tracklist. Track IDs are
// player-chosen object paths with no common prefix, so the cache is the only
// way to rebuild the full path from a bare ID.
func resolveTrackRef(tracks []Track, ref string) (string, bool) {
	for i := range tracks {
		if tracks[i].TrackID == ref || path.Base(tracks[i].TrackID) == ref {
			return tracks[i].TrackID, true
		}
	}
	return "", false
}

// mutateTracklist applies fn to the cached player and broadcasts the resulting
// tracklist snapshot. fn returns false for no-op mutations (nothing stored,
// nothing broadcast). fn must respect updatePlayers' copy-on-write contract:
// replace the Tracklist slice, never mutate it in place.
func (m *MPRISBackend) mutateTracklist(busName string, fn func(p *Player) bool) error {
	var snapshot Player
	found, changed := false, false
	ok := m.updatePlayers(func(players []Player) []Player {
		for i := range players {
			if players[i].BusName != busName {
				continue
			}
			found = true
			if !fn(&players[i]) {
				return nil
			}
			changed = true
			snapshot = players[i]
			return players
		}
		return nil
	})
	if !ok || !found {
		return &PlayerNotFoundError{BusName: busName}
	}

	if changed {
		m.notify(events.Event{Type: events.TypePlayerTracklist, Data: tracklistEnvelope(snapshot)})
	}
	return nil
}

func tracklistEnvelope(p Player) map[string]any {
	tracks := p.Tracklist
	if tracks == nil {
		tracks = []Track{}
	}
	return map[string]any{
		"bus_name":        p.BusName,
		"can_edit_tracks": p.CanEditTracks,
		"tracks":          tracks,
		"emitted_at":      time.Now().UnixMilli(),
	}
}

// tracklistMatches reports whether the player's cached tracklist has exactly
// the given IDs in the same order.
func (m *MPRISBackend) tracklistMatches(busName string, ids []dbus.ObjectPath) bool {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil || len(player.Tracklist) != len(ids) {
		return false
	}
	for i := range ids {
		if player.Tracklist[i].TrackID != string(ids[i]) {
			return false
		}
	}
	return true
}

// ReplaceTracklist wholesale-replaces a player's tracklist (TrackListReplaced).
func (m *MPRISBackend) ReplaceTracklist(busName string, tracks []Track) error {
	return m.mutateTracklist(busName, func(p *Player) bool {
		p.TracklistSupported = true
		p.Tracklist = tracks
		return true
	})
}

// AddTrackToCache inserts a track after afterTrack (TrackAdded signal).
// Track IDs are unique within a tracklist per spec, so a TrackAdded for an
// already-cached ID is a duplicate signal and is ignored.
func (m *MPRISBackend) AddTrackToCache(busName string, track Track, afterTrack string) error {
	return m.mutateTracklist(busName, func(p *Player) bool {
		// A player emitting TrackList signals supports the interface even if
		// the initial GetAll failed (lazy init).
		p.TracklistSupported = true
		for i := range p.Tracklist {
			if p.Tracklist[i].TrackID == track.TrackID {
				logger.Debug("[mpris] ignoring duplicate TrackAdded %s for %s", track.TrackID, busName)
				return false
			}
		}
		p.Tracklist = insertTrack(p.Tracklist, track, afterTrack)
		return true
	})
}

// insertTrack returns a new slice with track inserted after afterTrack:
// the NoTrack sentinel prepends, an unknown ID appends.
func insertTrack(tracks []Track, track Track, afterTrack string) []Track {
	out := make([]Track, 0, len(tracks)+1)
	if afterTrack == MPRIS_NO_TRACK {
		return append(append(out, track), tracks...)
	}

	inserted := false
	for _, t := range tracks {
		out = append(out, t)
		if !inserted && t.TrackID == afterTrack {
			out = append(out, track)
			inserted = true
		}
	}
	if !inserted {
		logger.Debug("[mpris] afterTrack %s not in cached tracklist, appending", afterTrack)
		out = append(out, track)
	}
	return out
}

// RemoveTrackFromCache drops a track by ID (TrackRemoved signal); unknown ID is a no-op.
func (m *MPRISBackend) RemoveTrackFromCache(busName, trackID string) error {
	return m.mutateTracklist(busName, func(p *Player) bool {
		for i := range p.Tracklist {
			if p.Tracklist[i].TrackID == trackID {
				out := make([]Track, 0, len(p.Tracklist)-1)
				out = append(out, p.Tracklist[:i]...)
				out = append(out, p.Tracklist[i+1:]...)
				p.Tracklist = out
				return true
			}
		}
		return false
	})
}

// UpdateTrackMetadataInCache replaces the track matching oldTrackID
// (TrackMetadataChanged signal). The spec allows trackid renames, so the
// stored ID comes from the new metadata when present.
func (m *MPRISBackend) UpdateTrackMetadataInCache(busName, oldTrackID string, track Track) error {
	return m.mutateTracklist(busName, func(p *Player) bool {
		for i := range p.Tracklist {
			if p.Tracklist[i].TrackID != oldTrackID {
				continue
			}
			if track.TrackID == "" {
				track.TrackID = oldTrackID
			}
			out := make([]Track, len(p.Tracklist))
			copy(out, p.Tracklist)
			out[i] = track
			p.Tracklist = out
			return true
		}
		return false
	})
}

// UpdateCanEditTracks handles CanEditTracks arriving via PropertiesChanged.
func (m *MPRISBackend) UpdateCanEditTracks(busName string, variant dbus.Variant) error {
	val, ok := extract[bool](variant)
	if !ok {
		return nil
	}
	return m.mutateTracklist(busName, func(p *Player) bool {
		if p.CanEditTracks == val {
			return false
		}
		p.CanEditTracks = val
		return true
	})
}

// tracklistPlayer fetches the cached player, rejecting players without
// TrackList support.
func (m *MPRISBackend) tracklistPlayer(busName string) (*Player, error) {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return nil, err
	}
	if !player.TracklistSupported {
		return nil, &TracklistUnsupportedError{BusName: busName}
	}
	return player, nil
}

// editableTracklistPlayer is tracklistPlayer plus the CanEditTracks gate
// required by edit operations.
func (m *MPRISBackend) editableTracklistPlayer(busName string) (*Player, error) {
	player, err := m.tracklistPlayer(busName)
	if err != nil {
		return nil, err
	}
	if !player.CanEditTracks {
		return nil, &CapabilityError{Required: "CanEditTracks"}
	}
	return player, nil
}

// GetTracklist serves the cached tracklist; distinguishes "unsupported" (error,
// →404) from "empty" (non-nil slice so JSON is [], not null).
func (m *MPRISBackend) GetTracklist(busName string) (*TracklistResponse, error) {
	player, err := m.tracklistPlayer(busName)
	if err != nil {
		return nil, err
	}

	tracks := player.Tracklist
	if tracks == nil {
		tracks = []Track{}
	}
	return &TracklistResponse{CanEditTracks: player.CanEditTracks, Tracks: tracks}, nil
}

// GoTo skips to the referenced track. Not gated on CanEditTracks: the spec
// doesn't class GoTo as an edit operation.
func (m *MPRISBackend) GoTo(busName, trackRef string) error {
	player, err := m.tracklistPlayer(busName)
	if err != nil {
		return err
	}
	trackID, ok := resolveTrackRef(player.Tracklist, trackRef)
	if !ok {
		return &ValidationError{Field: "track_id", Message: "unknown track: " + trackRef}
	}

	logger.Debug("[mpris] going to track %s for %s", trackID, busName)
	return m.callMethod(busName, MPRIS_METHOD_GO_TO, dbus.ObjectPath(trackID))
}

// AddTrack asks the player to add uri after afterTrack; empty afterTrack
// appends after the last cached track (NoTrack sentinel when the list is empty).
func (m *MPRISBackend) AddTrack(busName, uri, afterTrack string, setAsCurrent bool) error {
	player, err := m.editableTracklistPlayer(busName)
	if err != nil {
		return err
	}
	// The spec requires an absolute URI whose scheme the player declared in
	// SupportedUriSchemes; a bare path would be silently dropped player-side.
	u, err := url.Parse(uri)
	if err != nil || u.Scheme == "" {
		return &ValidationError{Field: "uri", Message: "must be an absolute URI (e.g. file:///path or http://...)"}
	}
	if len(player.SupportedUriSchemes) > 0 && !slices.Contains(player.SupportedUriSchemes, u.Scheme) {
		return &ValidationError{Field: "uri", Message: "scheme " + u.Scheme + " not supported by player"}
	}

	switch afterTrack {
	case "":
		if n := len(player.Tracklist); n > 0 {
			afterTrack = player.Tracklist[n-1].TrackID
		} else {
			afterTrack = MPRIS_NO_TRACK
		}
	case MPRIS_NO_TRACK, "NoTrack": // explicit prepend
		afterTrack = MPRIS_NO_TRACK
	default:
		resolved, ok := resolveTrackRef(player.Tracklist, afterTrack)
		if !ok {
			return &ValidationError{Field: "after_track", Message: "unknown track: " + afterTrack}
		}
		afterTrack = resolved
	}

	logger.Debug("[mpris] adding track %s after %s for %s", uri, afterTrack, busName)
	return m.callMethod(busName, MPRIS_METHOD_ADD_TRACK, uri, dbus.ObjectPath(afterTrack), setAsCurrent)
}

// RemoveTrack asks the player to remove the referenced track from its tracklist.
func (m *MPRISBackend) RemoveTrack(busName, trackRef string) error {
	player, err := m.editableTracklistPlayer(busName)
	if err != nil {
		return err
	}
	trackID, ok := resolveTrackRef(player.Tracklist, trackRef)
	if !ok {
		return &ValidationError{Field: "track_id", Message: "unknown track: " + trackRef}
	}

	logger.Debug("[mpris] removing track %s for %s", trackID, busName)
	return m.callMethod(busName, MPRIS_METHOD_REMOVE_TRACK, dbus.ObjectPath(trackID))
}
