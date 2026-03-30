package mpris

import (
	"context"
	"sync"
	"time"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/logger"
)

// Heartbeat manages periodic position updates for playing players.
// It starts automatically when at least one player is Playing and stops
// automatically when no player is Playing anymore.
type Heartbeat struct {
	backend *MPRISBackend
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	active bool
}

// NewHeartbeat creates a new heartbeat manager
func NewHeartbeat(backend *MPRISBackend) *Heartbeat {
	ctx, cancel := context.WithCancel(backend.ctx)
	return &Heartbeat{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
		active:  false,
	}
}

// Start starts the heartbeat if it's not already active.
// This function is idempotent: calling multiple times only starts one heartbeat.
func (h *Heartbeat) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.active {
		return // Already active
	}

	h.active = true
	go h.run()
}

// Stop stops the heartbeat
func (h *Heartbeat) Stop() {
	h.cancel()
}

// IsRunning returns true if the heartbeat is active
func (h *Heartbeat) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.active
}

// run is the main heartbeat loop
func (h *Heartbeat) run() {
	defer func() {
		h.mu.Lock()
		h.active = false
		h.mu.Unlock()
		logger.Debug("[mpris] position heartbeat stopped")
	}()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Debug("[mpris] position heartbeat started")

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			hasPlaying := h.updatePlayingPositions()
			if !hasPlaying {
				return // Auto-stop: no more players Playing
			}
		}
	}
}

// updatePlayingPositions updates the position of all playing players.
// Returns true if at least one player is Playing.
func (h *Heartbeat) updatePlayingPositions() bool {
	players, ok := h.backend.cache.Get(CACHE_KEY)
	if !ok {
		return false
	}

	hasPlaying := false
	positions := make(map[string]positionUpdate)
	for _, player := range players {
		// Update only Playing players
		if player.PlaybackStatus != StatusPlaying {
			continue
		}

		hasPlaying = true

		// Get current position via helper
		variant, err := h.backend.getProperty(player.BusName, MPRIS_PLAYER_IFACE, "Position")
		if err != nil {
			continue
		}

		// Some MPRIS implementations (e.g. go-librespot) return 0 for Position
		// even during active playback. Skip the update to avoid resetting the
		// seeker to the beginning of the track.
		pos, ok := idbus.ExtractInt64(variant)
		if !ok || pos <= 0 {
			logger.Debug("[mpris] skipping zero/invalid position for %s", player.BusName)
			continue
		}

		positions[player.BusName] = positionUpdate{
			position:  pos,
			trackID:   player.Metadata["mpris:trackid"],
			emittedAt: time.Now().UnixMilli(),
		}
	}

	if len(positions) > 0 {
		h.backend.UpdatePositions(positions)
	}

	return hasPlaying
}
