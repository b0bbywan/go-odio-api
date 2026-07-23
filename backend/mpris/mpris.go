package mpris

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New creates a new MPRIS backend
func New(ctx context.Context, cfg *config.MPRISConfig) (*MPRISBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	return &MPRISBackend{
		conn:    conn,
		ctx:     ctx,
		timeout: cfg.Timeout,
		events:  make(chan events.Event, 64),
	}, nil
}

// updatePlayers hands fn a private copy of the cached players and stores fn's
// result; fn may return nil to abort the write. playersMu serializes writers so
// concurrent read-modify-writes can't drop each other; readers stay lock-free.
// Returns false without calling fn when the cache was never loaded (nil) —
// storing would wrongly mark the cache as valid.
func (m *MPRISBackend) updatePlayers(fn func([]Player) []Player) bool {
	m.playersMu.Lock()
	defer m.playersMu.Unlock()

	snapshot := m.players.Load()
	if snapshot == nil {
		return false
	}

	players := make([]Player, len(snapshot))
	copy(players, snapshot)

	if updated := fn(players); updated != nil {
		m.players.Store(updated)
	}
	return true
}

// Start loads the initial cache and starts the listener
func (m *MPRISBackend) Start() error {
	logger.Debug("[mpris] starting backend")

	// Load cache at startup
	_, err := m.ListPlayers()
	if err != nil {
		return err
	}

	// Start the listener for MPRIS changes
	m.listener = NewListener(m)
	if err := m.listener.Start(); err != nil {
		return err
	}

	// Start the heartbeat (will auto-stop if no player is Playing)
	m.heartbeat = NewHeartbeat(m)
	m.heartbeat.Start()

	logger.Info("[mpris] backend started successfully")
	return nil
}

// ListPlayers lists all available MPRIS players.
// This function uses the cache as priority. If the cache is empty,
// it performs a D-Bus call to list players and updates the cache.
// To force reload of a specific player, use ReloadPlayerFromDBus.
func (m *MPRISBackend) ListPlayers() ([]Player, error) {
	// Check cache first
	if players := m.players.Load(); players != nil {
		logger.Debug("[mpris] returning %d players from cache", len(players))
		return players, nil
	}

	// Cache miss, load from D-Bus
	logger.Debug("[mpris] cache miss, loading players")
	start := time.Now()

	// List all bus names
	names, err := m.listDBusNames()
	if err != nil {
		return nil, err
	}

	// Filter only MPRIS players
	players := make([]Player, 0)
	for _, name := range names {
		if strings.HasPrefix(name, MPRIS_PREFIX+".") {
			player, err := m.getPlayerFromDBus(name)
			if err != nil {
				logger.Warn("[mpris] failed to get player info for %s: %v", name, err)
				continue
			}
			players = append(players, player)
		}
	}

	elapsed := time.Since(start)
	logger.Debug("[mpris] loaded %d players in %s", len(players), elapsed)

	// Update cache
	m.players.Store(players)

	return players, nil
}

// GetPlayerFromCache retrieves a specific player from cache only.
// If the player is not in cache, returns PlayerNotFoundError.
// To force reload from D-Bus, use ReloadPlayerFromDBus.
func (m *MPRISBackend) GetPlayerFromCache(busName string) (*Player, error) {
	if err := validateBusName(busName); err != nil {
		return nil, err
	}

	players := m.players.Load()
	if players == nil {
		return nil, &PlayerNotFoundError{BusName: busName}
	}

	for _, player := range players {
		if player.BusName == busName {
			return &player, nil
		}
	}
	return nil, &PlayerNotFoundError{BusName: busName}
}

// UpdatePlayer updates a specific player in the cache.
// If the player exists, it is replaced. Otherwise, it is added to the cache.
// WARNING: If the cache is empty, this function reloads ALL players via ListPlayers.
func (m *MPRISBackend) UpdatePlayer(updated Player) error {
	found := false
	ok := m.updatePlayers(func(players []Player) []Player {
		for i, player := range players {
			if player.BusName == updated.BusName {
				players[i] = updated
				found = true
				return players
			}
		}
		// Player not in cache, add it
		return append(players, updated)
	})
	if !ok {
		// If no cache, reload everything
		_, err := m.ListPlayers()
		return err
	}

	eventType := events.TypePlayerUpdated
	if !found {
		eventType = events.TypePlayerAdded
	}
	m.notify(events.Event{Type: eventType, Data: playerEnvelope(updated)})
	return nil
}

// shouldAcceptPosition decides whether a freshly-reported MPRIS position
// should overwrite the cache. Single source of truth for the per-player
// guards used by both the heartbeat poller and the listener path:
//   - drop zero/negative values (some players, like go-librespot, report 0
//     mid-playback)
//   - drop Bluez/AVRCP-style latched repeats (no real change)
//   - drop Position == Length (Chrome briefly reports the previous track's
//     duration as the new track's starting position, which would otherwise
//     snap the seeker to the end of the new track for ~5 s)
func shouldAcceptPosition(p *Player, pos int64) bool {
	if pos <= 0 {
		logger.Debug("[mpris] skipping zero/invalid position for %s", p.BusName)
		return false
	}
	if pos == p.Position {
		logger.Debug("[mpris] skipping unchanged position for %s (%d)", p.BusName, pos)
		return false
	}
	if length, _ := strconv.ParseInt(p.Metadata["mpris:length"], 10, 64); length > 0 && pos == length {
		logger.Debug("[mpris] skipping suspicious Position == Length for %s (%d)", p.BusName, pos)
		return false
	}
	return true
}

// UpdatePlayerProperties selectively updates player properties in the cache.
// Mainly used by the listener to update cache upon receiving
// D-Bus PropertiesChanged signals. Does NOT make D-Bus calls.
func (m *MPRISBackend) UpdatePlayerProperties(busName string, changed map[string]dbus.Variant) error {
	var updated Player
	found := false
	ok := m.updatePlayers(func(players []Player) []Player {
		i := -1
		for j := range players {
			if players[j].BusName == busName {
				i = j
				break
			}
		}
		if i < 0 {
			return nil
		}
		found = true

		// Update only properties that have changed
		for key, variant := range changed {
			switch key {
			case "PlaybackStatus":
				if val, ok := extract[string](variant); ok {
					players[i].PlaybackStatus = PlaybackStatus(val)
				}
			case "LoopStatus":
				if val, ok := extract[string](variant); ok {
					players[i].LoopStatus = LoopStatus(val)
				}
			case "Shuffle":
				if val, ok := extract[bool](variant); ok {
					players[i].Shuffle = val
				}
			case "Volume":
				if val, ok := extract[float64](variant); ok {
					players[i].Volume = &val
				}
			case "Metadata":
				if metaMap, ok := extract[map[string]dbus.Variant](variant); ok {
					oldTrackID := players[i].Metadata["mpris:trackid"]
					players[i].Metadata = make(map[string]string)
					for k, v := range metaMap {
						players[i].Metadata[k] = formatMetadataValue(v.Value())
					}
					// Track changed — reset stale position from previous track
					newTrackID := players[i].Metadata["mpris:trackid"]
					if newTrackID != oldTrackID {
						logger.Debug("[mpris] %s trackid changed %q -> %q, resetting position", busName, oldTrackID, newTrackID)
						players[i].Position = 0
						players[i].PositionUpdatedAt = time.Now()
					}
				}
			case "Rate":
				if val, ok := extract[float64](variant); ok {
					players[i].Rate = val
				}
			case "Position":
				if val, ok := extract[int64](variant); ok && shouldAcceptPosition(&players[i], val) {
					players[i].Position = val
					players[i].PositionUpdatedAt = time.Now()
				}
			case "CanPlay", "CanPause", "CanGoNext", "CanGoPrevious", "CanSeek", "CanControl":
				players[i].Capabilities.setFromProp(key, variant)
			}
		}

		updated = players[i]
		return players
	})
	if !ok || !found {
		return &PlayerNotFoundError{BusName: busName}
	}

	m.notify(events.Event{Type: events.TypePlayerUpdated, Data: playerEnvelope(updated)})
	logger.Debug("[mpris] updated %d properties for player %s", len(changed), busName)
	return nil
}

// UpdateProperty updates a single property of a player in the cache
func (m *MPRISBackend) UpdateProperty(busName, property string, value dbus.Variant) error {
	return m.UpdatePlayerProperties(busName, map[string]dbus.Variant{
		property: value,
	})
}

// UpdatePositions updates positions for multiple players in a single cache pass
// and emits a single player.position event with all updated positions.
func (m *MPRISBackend) UpdatePositions(positions map[string]positionUpdate) {
	var updates []map[string]any
	m.updatePlayers(func(players []Player) []Player {
		for i, player := range players {
			if u, ok := positions[player.BusName]; ok {
				players[i].Position = u.position
				players[i].PositionUpdatedAt = time.UnixMilli(u.emittedAt)
				updates = append(updates, map[string]any{
					"bus_name":   player.BusName,
					"track_id":   u.trackID,
					"position":   u.position,
					"emitted_at": u.emittedAt,
				})
			}
		}
		if len(updates) == 0 {
			return nil
		}
		return players
	})

	if len(updates) > 0 {
		m.notify(events.Event{Type: events.TypePlayerPosition, Data: updates})
	}
}

// ReloadPlayerFromDBus reloads a specific player from D-Bus and updates the cache.
// This function forces a D-Bus call even if the player is already in cache.
// Use this function when you need the most recent data.
func (m *MPRISBackend) ReloadPlayerFromDBus(busName string) (*Player, error) {
	if err := validateBusName(busName); err != nil {
		return nil, err
	}

	player, err := m.getPlayerFromDBus(busName)
	if err != nil {
		return nil, err
	}

	if err := m.UpdatePlayer(player); err != nil {
		return nil, err
	}

	return &player, nil
}

// RemovePlayer removes a player from cache (when it closes)
func (m *MPRISBackend) RemovePlayer(busName string) error {
	if err := validateBusName(busName); err != nil {
		return err
	}

	ok := m.updatePlayers(func(players []Player) []Player {
		filtered := make([]Player, 0, len(players))
		for _, player := range players {
			if player.BusName != busName {
				filtered = append(filtered, player)
			}
		}
		return filtered
	})
	if !ok {
		return nil
	}

	m.notify(events.Event{
		Type: events.TypePlayerRemoved,
		Data: map[string]string{"bus_name": busName},
	})
	logger.Debug("[mpris] removed player %s from cache", busName)
	return nil
}

// findPlayerByUniqueName finds the busName of a player from its unique D-Bus name.
// D-Bus signals contain the unique name (e.g., ":1.107") and not the well-known name
// (e.g., "org.mpris.MediaPlayer2.spotify"). This function maps between the two
// by searching in the cache. Returns "" if the player is not found.
func (m *MPRISBackend) findPlayerByUniqueName(uniqueName string) string {
	players := m.players.Load()

	for _, player := range players {
		if player.uniqueName == uniqueName {
			return player.BusName
		}
	}
	return ""
}

// getPlayerFromDBus loads an MPRIS player from D-Bus with all its properties.
// This private function is the single entry point for loading a player from D-Bus.
// It creates a new Player and calls loadFromDBus() to retrieve all properties
// using GetAll (2 D-Bus calls instead of ~15 individual calls).
func (m *MPRISBackend) getPlayerFromDBus(busName string) (Player, error) {
	player := newPlayer(m, busName)
	if err := player.loadFromDBus(); err != nil {
		return Player{}, err
	}
	return *player, nil
}

// requireCapability gates an action on a cached player's capability,
// mapping a missing player or capability to the matching error.
func (m *MPRISBackend) requireCapability(busName, required string, can func(*Player) bool) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !can(player) {
		return &CapabilityError{Required: required}
	}
	return nil
}

// Play starts playback
func (m *MPRISBackend) Play(busName string) error {
	if err := m.requireCapability(busName, "CanPlay", (*Player).CanPlay); err != nil {
		return err
	}

	logger.Debug("[mpris] playing %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PLAY)
}

// Pause pauses playback
func (m *MPRISBackend) Pause(busName string) error {
	if err := m.requireCapability(busName, "CanPause", (*Player).CanPause); err != nil {
		return err
	}

	logger.Debug("[mpris] pausing %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PAUSE)
}

// PlayPause toggles between play and pause
func (m *MPRISBackend) PlayPause(busName string) error {
	canEither := func(p *Player) bool { return p.CanPlay() || p.CanPause() }
	if err := m.requireCapability(busName, "CanPlay or CanPause", canEither); err != nil {
		return err
	}

	logger.Debug("[mpris] toggling play/pause for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PLAY_PAUSE)
}

// Stop stops playback
func (m *MPRISBackend) Stop(busName string) error {
	if err := m.requireCapability(busName, "CanControl", (*Player).CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] stopping %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_STOP)
}

// Next skips to the next track
func (m *MPRISBackend) Next(busName string) error {
	if err := m.requireCapability(busName, "CanGoNext", (*Player).CanGoNext); err != nil {
		return err
	}

	logger.Debug("[mpris] next track for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_NEXT)
}

// Previous goes back to the previous track
func (m *MPRISBackend) Previous(busName string) error {
	if err := m.requireCapability(busName, "CanGoPrevious", (*Player).CanGoPrevious); err != nil {
		return err
	}

	logger.Debug("[mpris] previous track for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PREVIOUS)
}

// Seek moves the playback position
func (m *MPRISBackend) Seek(busName string, offset int64) error {
	if err := m.requireCapability(busName, "CanSeek", (*Player).CanSeek); err != nil {
		return err
	}

	logger.Debug("[mpris] seeking %d for %s", offset, busName)
	return m.callMethod(busName, MPRIS_METHOD_SEEK, offset)
}

// SetPosition seeks to an absolute position in microseconds.
// trackID may be empty; if so it is resolved from the cached player metadata.
// Falls back to a relative Seek when no valid track ID is available.
func (m *MPRISBackend) SetPosition(busName, trackID string, position int64) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanSeek() {
		return &CapabilityError{Required: "CanSeek"}
	}

	if trackID == "" {
		trackID = player.Metadata["mpris:trackid"]
	}

	if trackID == "" || trackID == MPRIS_NO_TRACK {
		// Player doesn't expose a usable track ID; fall back to relative seek
		// using the cached position as the reference point.
		offset := position - player.Position
		logger.Debug("[mpris] seek fallback (no trackid): offset=%d for %s", offset, busName)
		return m.callMethod(busName, MPRIS_METHOD_SEEK, offset)
	}

	logger.Debug("[mpris] setting position to %d for %s", position, busName)
	return m.callMethod(busName, MPRIS_METHOD_SET_POSITION, dbus.ObjectPath(trackID), position)
}

// SetVolume sets the volume
func (m *MPRISBackend) SetVolume(busName string, volume float64) error {
	if volume < 0 || volume > 1 {
		return &ValidationError{Field: "volume", Message: "must be between 0 and 1"}
	}

	if err := m.requireCapability(busName, "CanControl", (*Player).CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting volume to %.2f for %s", volume, busName)
	return m.setProperty(busName, "Volume", volume)
}

// SetLoopStatus sets the loop status
func (m *MPRISBackend) SetLoopStatus(busName string, status LoopStatus) error {
	switch status {
	case LoopNone, LoopTrack, LoopPlaylist:
		// Valid
	default:
		return &ValidationError{Field: "loop", Message: "must be None, Track, or Playlist"}
	}

	if err := m.requireCapability(busName, "CanControl", (*Player).CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting loop status to %s for %s", status, busName)
	return m.setProperty(busName, "LoopStatus", string(status))
}

// SetShuffle enables/disables shuffle mode
func (m *MPRISBackend) SetShuffle(busName string, shuffle bool) error {
	if err := m.requireCapability(busName, "CanControl", (*Player).CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting shuffle to %v for %s", shuffle, busName)
	return m.setProperty(busName, "Shuffle", shuffle)
}

// CacheUpdatedAt returns the last time the player cache was written to.
func (m *MPRISBackend) CacheUpdatedAt() time.Time {
	return m.players.UpdatedAt()
}

// InvalidateCache invalidates the entire cache
func (m *MPRISBackend) InvalidateCache() {
	m.players.Reset()
}

// Close cleanly closes connections and stops the listener
func (m *MPRISBackend) Close() {
	if m.heartbeat != nil {
		m.heartbeat.Stop()
		m.heartbeat = nil
	}
	if m.listener != nil {
		m.listener.Stop()
		m.listener = nil
	}
	if m.conn != nil {
		if err := m.conn.Close(); err != nil {
			logger.Info("Failed to close D-Bus connection: %v", err)
		}
		m.conn = nil
	}
	close(m.events)
}

func playerEnvelope(p Player) map[string]any {
	return map[string]any{
		"data":       p,
		"emitted_at": time.Now().UnixMilli(),
	}
}

func (m *MPRISBackend) notify(e events.Event) {
	select {
	case m.events <- e:
	default:
		logger.Warn("[mpris] event channel full, dropping %s event", e.Type)
	}
}

// Events returns the read-only event channel for this backend.
func (m *MPRISBackend) Events() <-chan events.Event { return m.events }
