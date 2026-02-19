package mpris

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
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
		cache:   cache.New[[]Player](0), // TTL=0 = no expiration
	}, nil
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
	if players, ok := m.cache.Get(CACHE_KEY); ok {
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
	m.cache.Set(CACHE_KEY, players)

	return players, nil
}

// GetPlayerFromCache retrieves a specific player from cache only.
// If the player is not in cache, returns PlayerNotFoundError.
// To force reload from D-Bus, use ReloadPlayerFromDBus.
func (m *MPRISBackend) GetPlayerFromCache(busName string) (*Player, error) {
	if err := validateBusName(busName); err != nil {
		return nil, err
	}

	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
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
	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		// If no cache, reload everything
		if _, err := m.ListPlayers(); err != nil {
			return err
		}
		return nil
	}

	found := false
	for i, player := range players {
		if player.BusName == updated.BusName {
			players[i] = updated
			found = true
			break
		}
	}

	if !found {
		// Player not in cache, add it
		players = append(players, updated)
	}

	m.cache.Set(CACHE_KEY, players)
	return nil
}

// UpdatePlayerProperties selectively updates player properties in the cache.
// Mainly used by the listener to update cache upon receiving
// D-Bus PropertiesChanged signals. Does NOT make D-Bus calls.
func (m *MPRISBackend) UpdatePlayerProperties(busName string, changed map[string]dbus.Variant) error {
	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		return &PlayerNotFoundError{BusName: busName}
	}

	for i, player := range players {
		if player.BusName != busName {
			continue
		}

		// Update only properties that have changed
		for key, variant := range changed {
			switch key {
			case "PlaybackStatus":
				if val, ok := extractString(variant); ok {
					players[i].PlaybackStatus = PlaybackStatus(val)
				}
			case "LoopStatus":
				if val, ok := extractString(variant); ok {
					players[i].LoopStatus = LoopStatus(val)
				}
			case "Shuffle":
				if val, ok := extractBool(variant); ok {
					players[i].Shuffle = val
				}
			case "Volume":
				if val, ok := extractFloat64(variant); ok {
					players[i].Volume = val
				}
			case "Metadata":
				if metaMap, ok := extractMetadataMap(variant); ok {
					players[i].Metadata = make(map[string]string)
					for k, v := range metaMap {
						players[i].Metadata[k] = formatMetadataValue(v.Value())
					}
				}
			case "Rate":
				if val, ok := extractFloat64(variant); ok {
					players[i].Rate = val
				}
			case "Position":
				if val, ok := extractInt64(variant); ok {
					players[i].Position = val
				}
			}
		}

		m.cache.Set(CACHE_KEY, players)
		logger.Debug("[mpris] updated %d properties for player %s", len(changed), busName)
		return nil
	}

	return &PlayerNotFoundError{BusName: busName}
}

// UpdateProperty updates a single property of a player in the cache
func (m *MPRISBackend) UpdateProperty(busName, property string, value dbus.Variant) error {
	return m.UpdatePlayerProperties(busName, map[string]dbus.Variant{
		property: value,
	})
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

	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		return nil
	}

	filtered := make([]Player, 0, len(players))
	for _, player := range players {
		if player.BusName != busName {
			filtered = append(filtered, player)
		}
	}

	m.cache.Set(CACHE_KEY, filtered)
	logger.Debug("[mpris] removed player %s from cache", busName)
	return nil
}

// findPlayerByUniqueName finds the busName of a player from its unique D-Bus name.
// D-Bus signals contain the unique name (e.g., ":1.107") and not the well-known name
// (e.g., "org.mpris.MediaPlayer2.spotify"). This function maps between the two
// by searching in the cache. Returns "" if the player is not found.
func (m *MPRISBackend) findPlayerByUniqueName(uniqueName string) string {
	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		return ""
	}

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

// Play starts playback
func (m *MPRISBackend) Play(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanPlay() {
		return &CapabilityError{Required: "CanPlay"}
	}

	logger.Debug("[mpris] playing %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PLAY)
}

// Pause pauses playback
func (m *MPRISBackend) Pause(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanPause() {
		return &CapabilityError{Required: "CanPause"}
	}

	logger.Debug("[mpris] pausing %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PAUSE)
}

// PlayPause toggles between play and pause
func (m *MPRISBackend) PlayPause(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanPlay() && !player.CanPause() {
		return &CapabilityError{Required: "CanPlay or CanPause"}
	}

	logger.Debug("[mpris] toggling play/pause for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PLAY_PAUSE)
}

// Stop stops playback
func (m *MPRISBackend) Stop(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] stopping %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_STOP)
}

// Next skips to the next track
func (m *MPRISBackend) Next(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanGoNext() {
		return &CapabilityError{Required: "CanGoNext"}
	}

	logger.Debug("[mpris] next track for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_NEXT)
}

// Previous goes back to the previous track
func (m *MPRISBackend) Previous(busName string) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanGoPrevious() {
		return &CapabilityError{Required: "CanGoPrevious"}
	}

	logger.Debug("[mpris] previous track for %s", busName)
	return m.callMethod(busName, MPRIS_METHOD_PREVIOUS)
}

// Seek moves the playback position
func (m *MPRISBackend) Seek(busName string, offset int64) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanSeek() {
		return &CapabilityError{Required: "CanSeek"}
	}

	logger.Debug("[mpris] seeking %d for %s", offset, busName)
	return m.callMethod(busName, MPRIS_METHOD_SEEK, offset)
}

// SetPosition sets the playback position
func (m *MPRISBackend) SetPosition(busName, trackID string, position int64) error {
	if trackID == "" {
		return &ValidationError{Field: "track_id", Message: "cannot be empty"}
	}

	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanSeek() {
		return &CapabilityError{Required: "CanSeek"}
	}

	logger.Debug("[mpris] setting position to %d for %s", position, busName)
	return m.callMethod(busName, MPRIS_METHOD_SET_POSITION, dbus.ObjectPath(trackID), position)
}

// SetVolume sets the volume
func (m *MPRISBackend) SetVolume(busName string, volume float64) error {
	if volume < 0 || volume > 1 {
		return &ValidationError{Field: "volume", Message: "must be between 0 and 1"}
	}

	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
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

	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] setting loop status to %s for %s", status, busName)
	return m.setProperty(busName, "LoopStatus", string(status))
}

// SetShuffle enables/disables shuffle mode
func (m *MPRISBackend) SetShuffle(busName string, shuffle bool) error {
	player, err := m.GetPlayerFromCache(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] setting shuffle to %v for %s", shuffle, busName)
	return m.setProperty(busName, "Shuffle", shuffle)
}

// CacheUpdatedAt returns the last time the player cache was written to.
func (m *MPRISBackend) CacheUpdatedAt() time.Time {
	return m.cache.UpdatedAt()
}

// InvalidateCache invalidates the entire cache
func (m *MPRISBackend) InvalidateCache() {
	m.cache.Delete(CACHE_KEY)
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
}
