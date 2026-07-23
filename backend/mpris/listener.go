package mpris

import (
	"context"
	"slices"
	"strings"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// NewListener creates a new MPRIS listener
func NewListener(backend *MPRISBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)

	return &Listener{
		backend:   backend,
		ctx:       ctx,
		cancel:    cancel,
		lastState: make(map[string]PlaybackStatus),
	}
}

// Start starts listening to MPRIS D-Bus signals
func (l *Listener) Start() error {
	// Use the backend connection
	conn := l.backend.conn

	if err := l.backend.addListenMatchRules(); err != nil {
		return err
	}

	ch := make(chan *dbus.Signal, 10)
	conn.Signal(ch)

	go l.listen(ch)

	logger.Info("[mpris] listener started (D-Bus signal-based)")
	return nil
}

// listen continuously listens to D-Bus signals
func (l *Listener) listen(ch <-chan *dbus.Signal) {
	for {
		select {
		case <-l.ctx.Done():
			return
		case sig, ok := <-ch:
			if !ok {
				return
			}
			logger.Debug("[mpris] received signal: %s from %s", sig.Name, sig.Sender)
			l.handleSignal(sig)
		}
	}
}

// handleSignal processes a D-Bus signal
func (l *Listener) handleSignal(sig *dbus.Signal) {
	switch sig.Name {
	case DBUS_PROP_CHANGED_SIGNAL:
		l.handlePropertiesChanged(sig)
	case DBUS_NAME_OWNER_CHANGED:
		l.handleNameOwnerChanged(sig)
	case MPRIS_SIGNAL_TRACKLIST_REPLACED, MPRIS_SIGNAL_TRACK_ADDED,
		MPRIS_SIGNAL_TRACK_REMOVED, MPRIS_SIGNAL_TRACK_METADATA_CHANGED:
		if busName := l.resolveSender(sig); busName != "" {
			l.handleTrackListSignal(busName, sig)
		}
	default:
		logger.Debug("[mpris] unhandled signal: %s", sig.Name)
	}
}

// handlePropertiesChanged processes MPRIS property changes
func (l *Listener) handlePropertiesChanged(sig *dbus.Signal) {
	// Body[0] = interface name
	// Body[1] = changed properties (map[string]Variant)
	// Body[2] = invalidated properties ([]string)

	iface, ok := arg[string](sig, 0)
	if !ok {
		return
	}

	changed, ok := arg[map[string]dbus.Variant](sig, 1)
	if !ok {
		return
	}

	// The signal contains the unique name (:1.107), not the well-known name
	// Find the corresponding MPRIS player
	busName := l.backend.findPlayerByUniqueName(sig.Sender)
	if busName == "" {
		// Signal from unknown player, ignore
		return
	}

	if iface == MPRIS_TRACKLIST_IFACE {
		if v, ok := changed["CanEditTracks"]; ok {
			if err := l.backend.UpdateCanEditTracks(busName, v); err != nil {
				logger.Error("[mpris] failed to update CanEditTracks for %s: %v", busName, err)
			}
		}
		// Some players (VLC) never emit usable TrackList signals (VLC mangles
		// their interface name) and only report queue changes here — as a
		// changed value or a bare invalidation. Skipping when the IDs match
		// the cache keeps players that emit both from doubling up.
		if v, ok := changed["Tracks"]; ok {
			if ids, ok := v.Value().([]dbus.ObjectPath); ok && !l.backend.tracklistMatches(busName, ids) {
				l.refreshTracklist(busName, ids)
			}
			return
		}
		if invalidated, ok := arg[[]string](sig, 2); ok && slices.Contains(invalidated, "Tracks") {
			l.refetchTracklist(busName)
		}
		return
	}
	if iface != MPRIS_PLAYER_IFACE {
		return
	}

	// Check if PlaybackStatus changed for deduplication
	if statusVar, hasStatus := changed["PlaybackStatus"]; hasStatus {
		if status, ok := extract[string](statusVar); ok {
			newStatus := PlaybackStatus(status)

			// Deduplication
			l.lastStateMu.RLock()
			lastStatus := l.lastState[busName]
			l.lastStateMu.RUnlock()

			if lastStatus != newStatus {
				l.lastStateMu.Lock()
				l.lastState[busName] = newStatus
				l.lastStateMu.Unlock()

				logger.Debug("[mpris] player %s changed status: %s -> %s", busName, lastStatus, newStatus)

				// Refresh Position alongside the status change so the SSE
				// we're about to broadcast carries a fresh timestamp. The
				// heartbeat only polls every 5 s, so without this the client
				// receives the cached position (up to 5 s stale) and jumps
				// when its interpolation snaps back to that older anchor.
				if posVariant, err := l.backend.getProperty(busName, MPRIS_PLAYER_IFACE, "Position"); err == nil {
					if _, ok := changed["Position"]; !ok {
						changed["Position"] = posVariant
					}
				}

				// If player switches to Playing, ensure heartbeat is running
				if newStatus == StatusPlaying {
					l.backend.heartbeat.Start()
				}
			}
		}
	}

	// Log the properties that will be updated
	propNames := make([]string, 0, len(changed))
	for propName := range changed {
		propNames = append(propNames, propName)
	}
	logger.Debug("[mpris] updating %s properties: %v", busName, propNames)

	// Update properties in cache from signal data
	if err := l.backend.UpdatePlayerProperties(busName, changed); err != nil {
		logger.Error("[mpris] failed to update player %s properties: %v", busName, err)
	}
}

// handleNameOwnerChanged detects when a player appears or disappears
func (l *Listener) handleNameOwnerChanged(sig *dbus.Signal) {
	// Body[0] = bus name
	// Body[1] = old owner
	// Body[2] = new owner

	if len(sig.Body) < 3 {
		return
	}

	busName, ok := sig.Body[0].(string)
	if !ok || !strings.HasPrefix(busName, MPRIS_PREFIX+".") {
		return
	}

	oldOwner, _ := sig.Body[1].(string)
	newOwner, _ := sig.Body[2].(string)

	if oldOwner == "" && newOwner != "" {
		// New player appeared
		logger.Info("[mpris] new player detected: %s", busName)
		if _, err := l.backend.ReloadPlayerFromDBus(busName); err != nil {
			logger.Error("[mpris] failed to add new player %s: %v", busName, err)
		}
	} else if oldOwner != "" && newOwner == "" {
		// Player disappeared
		logger.Info("[mpris] player removed: %s", busName)
		if err := l.backend.RemovePlayer(busName); err != nil {
			logger.Error("[mpris] failed to remove player %s: %v", busName, err)
		}
	}
}

// resolveSender maps a signal's unique-name sender to a cached busName.
// Dropping unknown senders is the safety net for the broad interface+path
// match rule, which delivers TrackList signals from any sender.
func (l *Listener) resolveSender(sig *dbus.Signal) string {
	busName := l.backend.findPlayerByUniqueName(sig.Sender)
	if busName == "" {
		logger.Debug("[mpris] dropping %s from unknown sender %s", sig.Name, sig.Sender)
	}
	return busName
}

// refreshTracklist replaces a player's cached tracklist from a list of IDs,
// fetching their metadata in one call (IDs only on failure).
func (l *Listener) refreshTracklist(busName string, ids []dbus.ObjectPath) {
	tracks := tracksFromIDs(ids)
	if len(ids) > 0 {
		if metas, err := newPlayer(l.backend, busName).getTracksMetadata(ids); err == nil {
			tracks = tracksFromMetadata(ids, metas)
		} else {
			logger.Debug("[mpris] GetTracksMetadata failed for %s, keeping IDs only: %v", busName, err)
		}
	}

	if err := l.backend.ReplaceTracklist(busName, tracks); err != nil {
		logger.Error("[mpris] failed to replace tracklist for %s: %v", busName, err)
	}
}

// refetchTracklist re-reads the Tracks property after an invalidation
// (no value in the signal) and refreshes the cache if it changed.
func (l *Listener) refetchTracklist(busName string) {
	v, err := l.backend.getProperty(busName, MPRIS_TRACKLIST_IFACE, "Tracks")
	if err != nil {
		logger.Debug("[mpris] failed to fetch Tracks for %s: %v", busName, err)
		return
	}
	ids, ok := v.Value().([]dbus.ObjectPath)
	if !ok {
		return
	}
	if !l.backend.tracklistMatches(busName, ids) {
		l.refreshTracklist(busName, ids)
	}
}

// handleTrackListSignal dispatches TrackList signals once the sender is
// resolved to a cached player.
func (l *Listener) handleTrackListSignal(busName string, sig *dbus.Signal) {
	switch sig.Name {
	case MPRIS_SIGNAL_TRACKLIST_REPLACED:
		l.handleTrackListReplaced(busName, sig)
	case MPRIS_SIGNAL_TRACK_ADDED:
		l.handleTrackAdded(busName, sig)
	case MPRIS_SIGNAL_TRACK_REMOVED:
		l.handleTrackRemoved(busName, sig)
	case MPRIS_SIGNAL_TRACK_METADATA_CHANGED:
		l.handleTrackMetadataChanged(busName, sig)
	}
}

// handleTrackListReplaced processes a wholesale tracklist replacement.
// Body[0] = new track IDs; Body[1] = current track (unused)
func (l *Listener) handleTrackListReplaced(busName string, sig *dbus.Signal) {
	ids, ok := arg[[]dbus.ObjectPath](sig, 0)
	if !ok {
		return
	}

	l.refreshTracklist(busName, ids)
}

// handleTrackAdded processes a track insertion.
// Body[0] = track metadata; Body[1] = the track it was inserted after
func (l *Listener) handleTrackAdded(busName string, sig *dbus.Signal) {
	meta, ok := arg[map[string]dbus.Variant](sig, 0)
	if !ok {
		return
	}
	afterTrack, ok := arg[dbus.ObjectPath](sig, 1)
	if !ok {
		return
	}

	if err := l.backend.AddTrackToCache(busName, trackFromSignalMetadata(meta), string(afterTrack)); err != nil {
		logger.Error("[mpris] failed to add track for %s: %v", busName, err)
	}
}

// handleTrackRemoved processes a track removal.
// Body[0] = removed track ID
func (l *Listener) handleTrackRemoved(busName string, sig *dbus.Signal) {
	trackID, ok := arg[dbus.ObjectPath](sig, 0)
	if !ok {
		return
	}

	if err := l.backend.RemoveTrackFromCache(busName, string(trackID)); err != nil {
		logger.Error("[mpris] failed to remove track for %s: %v", busName, err)
	}
}

// handleTrackMetadataChanged processes per-track metadata updates.
// Body[0] = track ID; Body[1] = new metadata
func (l *Listener) handleTrackMetadataChanged(busName string, sig *dbus.Signal) {
	trackID, ok := arg[dbus.ObjectPath](sig, 0)
	if !ok {
		return
	}
	meta, ok := arg[map[string]dbus.Variant](sig, 1)
	if !ok {
		return
	}

	if err := l.backend.UpdateTrackMetadataInCache(busName, string(trackID), trackFromSignalMetadata(meta)); err != nil {
		logger.Error("[mpris] failed to update track metadata for %s: %v", busName, err)
	}
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[mpris] stopping listener")
	l.cancel()
	logger.Debug("[mpris] listener stopped")
}
