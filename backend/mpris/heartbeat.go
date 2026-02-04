package mpris

import (
	"context"
	"sync"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/logger"
)

// Heartbeat gère la mise à jour périodique de la position des players en lecture.
// Il démarre automatiquement quand au moins un player est en Playing et s'arrête
// automatiquement quand plus aucun player n'est en Playing.
type Heartbeat struct {
	backend *MPRISBackend
	ctx     context.Context
	cancel  context.CancelFunc

	mu     sync.Mutex
	active bool
}

// NewHeartbeat crée un nouveau gestionnaire de heartbeat
func NewHeartbeat(backend *MPRISBackend) *Heartbeat {
	ctx, cancel := context.WithCancel(backend.ctx)
	return &Heartbeat{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
		active:  false,
	}
}

// Start démarre le heartbeat s'il n'est pas déjà actif.
// Cette fonction est idempotente : appeler plusieurs fois ne démarre qu'un seul heartbeat.
func (h *Heartbeat) Start() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.active {
		return // Déjà actif
	}

	h.active = true
	go h.run()
}

// StartIfAnyPlaying démarre le heartbeat si au moins un player est en Playing.
// Utile au démarrage de l'application pour détecter si un player joue déjà.
func (h *Heartbeat) StartIfAnyPlaying(players []Player) {
	for _, player := range players {
		if player.PlaybackStatus == StatusPlaying {
			logger.Debug("[mpris] detected player %s already playing, starting heartbeat", player.BusName)
			h.Start()
			return
		}
	}
}

// Stop arrête le heartbeat
func (h *Heartbeat) Stop() {
	h.cancel()
}

// IsRunning retourne true si le heartbeat est actif
func (h *Heartbeat) IsRunning() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.active
}

// run est la boucle principale du heartbeat
func (h *Heartbeat) run() {
	defer func() {
		h.mu.Lock()
		h.active = false
		h.mu.Unlock()
		logger.Debug("[mpris] position heartbeat stopped")
	}()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	logger.Debug("[mpris] position heartbeat started")

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			hasPlaying := h.updatePlayingPositions()
			if !hasPlaying {
				return // Auto-stop: plus aucun player en Playing
			}
		}
	}
}

// updatePlayingPositions met à jour la position de tous les players en lecture.
// Retourne true si au moins un player est en Playing.
func (h *Heartbeat) updatePlayingPositions() bool {
	players, ok := h.backend.cache.Get(cacheKey)
	if !ok {
		return false
	}

	hasPlaying := false
	for _, player := range players {
		// Mettre à jour uniquement les players en Playing
		if player.PlaybackStatus != StatusPlaying {
			continue
		}

		hasPlaying = true

		// Récupérer la position actuelle
		obj := h.backend.conn.Object(player.BusName, mprisPath)
		call := obj.Call(dbusPropGet, 0, mprisPlayerIface, "Position")
		if err := h.backend.callWithTimeout(call); err != nil {
			continue
		}
		var variant dbus.Variant
		if err := call.Store(&variant); err != nil {
			continue
		}

		// Mettre à jour via UpdateProperty (gère le cache proprement)
		if err := h.backend.UpdateProperty(player.BusName, "Position", variant); err != nil {
			logger.Warn("[mpris] failed to update position for %s: %v", player.BusName, err)
		}
	}

	return hasPlaying
}
