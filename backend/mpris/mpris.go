package mpris

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New crée un nouveau backend MPRIS
func New(ctx context.Context) (*MPRISBackend, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	return &MPRISBackend{
		conn:  conn,
		ctx:   ctx,
		cache: cache.New[[]Player](0), // TTL=0 = pas d'expiration
	}, nil
}

// Start charge le cache initial et démarre le listener
func (m *MPRISBackend) Start() error {
	logger.Debug("[mpris] starting backend")

	// Charger le cache au démarrage
	if _, err := m.ListPlayers(); err != nil {
		return err
	}

	// Démarrer le listener pour les changements MPRIS
	m.listener = NewListener(m)
	if err := m.listener.Start(); err != nil {
		return err
	}

	logger.Info("[mpris] backend started successfully")
	return nil
}

// ListPlayers liste tous les lecteurs MPRIS disponibles
func (m *MPRISBackend) ListPlayers() ([]Player, error) {
	start := time.Now()

	// Lister tous les bus names
	var names []string
	err := m.conn.BusObject().Call("org.freedesktop.DBus.ListNames", 0).Store(&names)
	if err != nil {
		return nil, err
	}

	// Filtrer uniquement les lecteurs MPRIS
	players := make([]Player, 0)
	for _, name := range names {
		if strings.HasPrefix(name, mprisPrefix+".") {
			player, err := m.getPlayerInfo(name)
			if err != nil {
				logger.Warn("[mpris] failed to get player info for %s: %v", name, err)
				continue
			}
			players = append(players, player)
		}
	}

	elapsed := time.Since(start)
	logger.Debug("[mpris] listed %d players in %s", len(players), elapsed)

	// Mettre à jour le cache
	m.cache.Set(cacheKey, players)

	return players, nil
}

// GetPlayer récupère un lecteur spécifique du cache
func (m *MPRISBackend) GetPlayer(busName string) (*Player, bool) {
	players, ok := m.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	for _, player := range players {
		if player.BusName == busName {
			return &player, true
		}
	}
	return nil, false
}

// UpdatePlayer met à jour un lecteur spécifique dans le cache
func (m *MPRISBackend) UpdatePlayer(updated Player) error {
	players, ok := m.cache.Get(cacheKey)
	if !ok {
		// Si pas de cache, on recharge tout
		_, err := m.ListPlayers()
		return err
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
		// Lecteur pas dans le cache, on l'ajoute
		players = append(players, updated)
	}

	m.cache.Set(cacheKey, players)
	return nil
}

// RefreshPlayer recharge un lecteur spécifique depuis D-Bus et met à jour le cache
func (m *MPRISBackend) RefreshPlayer(busName string) (*Player, error) {
	player, err := m.getPlayerInfo(busName)
	if err != nil {
		return nil, err
	}

	if err := m.UpdatePlayer(player); err != nil {
		return nil, err
	}

	return &player, nil
}

// RemovePlayer supprime un lecteur du cache (quand il se ferme)
func (m *MPRISBackend) RemovePlayer(busName string) error {
	players, ok := m.cache.Get(cacheKey)
	if !ok {
		return nil
	}

	filtered := make([]Player, 0, len(players))
	for _, player := range players {
		if player.BusName != busName {
			filtered = append(filtered, player)
		}
	}

	m.cache.Set(cacheKey, filtered)
	logger.Debug("[mpris] removed player %s from cache", busName)
	return nil
}

// getPlayerInfo récupère toutes les informations d'un lecteur MPRIS
func (m *MPRISBackend) getPlayerInfo(busName string) (Player, error) {
	// Charger toutes les propriétés via reflection
	player := loadPlayer(m.conn, busName)
	return player, nil
}

// Play démarre la lecture
func (m *MPRISBackend) Play(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanPlay); err != nil {
		return err
	}

	logger.Debug("[mpris] playing %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Play", 0).Err
}

// Pause met en pause la lecture
func (m *MPRISBackend) Pause(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanPause); err != nil {
		return err
	}

	logger.Debug("[mpris] pausing %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Pause", 0).Err
}

// PlayPause bascule entre lecture et pause
func (m *MPRISBackend) PlayPause(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanPlay, Caps.CanPause); err != nil {
		return err
	}

	logger.Debug("[mpris] toggling play/pause for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".PlayPause", 0).Err
}

// Stop arrête la lecture
func (m *MPRISBackend) Stop(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] stopping %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Stop", 0).Err
}

// Next passe à la piste suivante
func (m *MPRISBackend) Next(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanGoNext); err != nil {
		return err
	}

	logger.Debug("[mpris] next track for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Next", 0).Err
}

// Previous revient à la piste précédente
func (m *MPRISBackend) Previous(busName string) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanGoPrevious); err != nil {
		return err
	}

	logger.Debug("[mpris] previous track for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Previous", 0).Err
}

// Seek déplace la position de lecture
func (m *MPRISBackend) Seek(busName string, offset int64) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanSeek); err != nil {
		return err
	}

	logger.Debug("[mpris] seeking %d for %s", offset, busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".Seek", 0, offset).Err
}

// SetPosition définit la position de lecture
func (m *MPRISBackend) SetPosition(busName, trackID string, position int64) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanSeek); err != nil {
		return err
	}

	logger.Debug("[mpris] setting position to %d for %s", position, busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(mprisPlayerIface+".SetPosition", 0, dbus.ObjectPath(trackID), position).Err
}

// SetVolume définit le volume
func (m *MPRISBackend) SetVolume(busName string, volume float64) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting volume to %.2f for %s", volume, busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(dbusPropIface+".Set", 0, mprisPlayerIface, "Volume", dbus.MakeVariant(volume)).Err
}

// SetLoopStatus définit le statut de boucle
func (m *MPRISBackend) SetLoopStatus(busName string, status LoopStatus) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting loop status to %s for %s", status, busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(dbusPropIface+".Set", 0, mprisPlayerIface, "LoopStatus", dbus.MakeVariant(string(status))).Err
}

// SetShuffle active/désactive le mode aléatoire
func (m *MPRISBackend) SetShuffle(busName string, shuffle bool) error {
	player, found := m.GetPlayer(busName)
	if !found {
		return &CapabilityError{Required: "player not found"}
	}
	if err := checkCapabilities(player, Caps.CanControl); err != nil {
		return err
	}

	logger.Debug("[mpris] setting shuffle to %v for %s", shuffle, busName)
	obj := m.conn.Object(busName, mprisPath)
	return obj.Call(dbusPropIface+".Set", 0, mprisPlayerIface, "Shuffle", dbus.MakeVariant(shuffle)).Err
}

// InvalidateCache invalide tout le cache
func (m *MPRISBackend) InvalidateCache() {
	m.cache.Delete(cacheKey)
}

// Close ferme proprement les connexions et arrête le listener
func (m *MPRISBackend) Close() {
	if m.listener != nil {
		m.listener.Stop()
		m.listener = nil
	}
	if m.conn != nil {
		m.conn.Close()
		m.conn = nil
	}
}
