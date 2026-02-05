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
func New(ctx context.Context, timeout time.Duration) (*MPRISBackend, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, err
	}

	return &MPRISBackend{
		conn:    conn,
		ctx:     ctx,
		timeout: timeout,
		cache:   cache.New[[]Player](0), // TTL=0 = pas d'expiration
	}, nil
}

// Start charge le cache initial et démarre le listener
func (m *MPRISBackend) Start() error {
	logger.Debug("[mpris] starting backend")

	// Charger le cache au démarrage
	players, err := m.ListPlayers()
	if err != nil {
		return err
	}

	// Démarrer le listener pour les changements MPRIS
	m.listener = NewListener(m)
	if err := m.listener.Start(); err != nil {
		return err
	}

	// Créer et démarrer le heartbeat si un player est déjà en Playing
	m.heartbeat = NewHeartbeat(m)
	m.heartbeat.StartIfAnyPlaying(players)

	logger.Info("[mpris] backend started successfully")
	return nil
}

// ListPlayers liste tous les lecteurs MPRIS disponibles.
// Cette fonction utilise le cache en priorité. Si le cache est vide,
// elle effectue un appel D-Bus pour lister les players et met à jour le cache.
// Pour forcer un rechargement d'un player spécifique, utilisez ReloadPlayerFromDBus.
func (m *MPRISBackend) ListPlayers() ([]Player, error) {
	// Vérifier le cache d'abord
	if players, ok := m.cache.Get(CACHE_KEY); ok {
		logger.Debug("[mpris] returning %d players from cache", len(players))
		return players, nil
	}

	// Cache miss, charger depuis D-Bus
	logger.Debug("[mpris] cache miss, loading players")
	start := time.Now()

	// Lister tous les bus names
	names, err := m.listDBusNames()
	if err != nil {
		return nil, err
	}

	// Filtrer uniquement les lecteurs MPRIS
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

	// Mettre à jour le cache
	m.cache.Set(CACHE_KEY, players)

	return players, nil
}

// GetPlayerFromCache récupère un lecteur spécifique du cache uniquement.
// Si le player n'est pas en cache, retourne PlayerNotFoundError.
// Pour forcer un rechargement depuis D-Bus, utilisez ReloadPlayerFromDBus.
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

// UpdatePlayer met à jour un lecteur spécifique dans le cache.
// Si le player existe, il est remplacé. Sinon, il est ajouté au cache.
// ATTENTION: Si le cache est vide, cette fonction recharge TOUS les players via ListPlayers.
func (m *MPRISBackend) UpdatePlayer(updated Player) error {
	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		// Si pas de cache, on recharge tout
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
		// Lecteur pas dans le cache, on l'ajoute
		players = append(players, updated)
	}

	m.cache.Set(CACHE_KEY, players)
	return nil
}

// UpdatePlayerProperties met à jour sélectivement les propriétés d'un player dans le cache.
// Utilisé principalement par le listener pour mettre à jour le cache lors de la réception
// de signaux D-Bus PropertiesChanged. Ne fait PAS d'appel D-Bus.
func (m *MPRISBackend) UpdatePlayerProperties(busName string, changed map[string]dbus.Variant) error {
	players, ok := m.cache.Get(CACHE_KEY)
	if !ok {
		return &PlayerNotFoundError{BusName: busName}
	}

	for i, player := range players {
		if player.BusName != busName {
			continue
		}

		// Mettre à jour seulement les propriétés qui ont changé
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

// UpdateProperty met à jour une seule propriété d'un player dans le cache
func (m *MPRISBackend) UpdateProperty(busName, property string, value dbus.Variant) error {
	return m.UpdatePlayerProperties(busName, map[string]dbus.Variant{
		property: value,
	})
}

// ReloadPlayerFromDBus recharge un lecteur spécifique depuis D-Bus et met à jour le cache.
// Cette fonction force un appel D-Bus même si le player est déjà en cache.
// Utilisez cette fonction quand vous avez besoin des données les plus récentes.
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

// RemovePlayer supprime un lecteur du cache (quand il se ferme)
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

// findPlayerByUniqueName trouve le busName d'un player à partir de son unique name D-Bus.
// Les signaux D-Bus contiennent le unique name (ex: ":1.107") et non le well-known name
// (ex: "org.mpris.MediaPlayer2.spotify"). Cette fonction fait le mapping entre les deux
// en cherchant dans le cache. Retourne "" si le player n'est pas trouvé.
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

// getPlayerFromDBus charge un lecteur MPRIS depuis D-Bus avec toutes ses propriétés.
// Cette fonction privée est le point d'entrée unique pour charger un player depuis D-Bus.
// Elle crée un nouveau Player et appelle loadFromDBus() pour récupérer toutes les propriétés
// en utilisant GetAll (2 appels D-Bus au lieu de ~15 appels individuels).
func (m *MPRISBackend) getPlayerFromDBus(busName string) (Player, error) {
	player := newPlayer(m, busName)
	if err := player.loadFromDBus(); err != nil {
		return Player{}, err
	}
	return *player, nil
}

// Play démarre la lecture
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

// Pause met en pause la lecture
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

// PlayPause bascule entre lecture et pause
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

// Stop arrête la lecture
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

// Next passe à la piste suivante
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

// Previous revient à la piste précédente
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

// Seek déplace la position de lecture
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

// SetPosition définit la position de lecture
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

// SetVolume définit le volume
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

// SetLoopStatus définit le statut de boucle
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

// SetShuffle active/désactive le mode aléatoire
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

// InvalidateCache invalide tout le cache
func (m *MPRISBackend) InvalidateCache() {
	m.cache.Delete(CACHE_KEY)
}

// Close ferme proprement les connexions et arrête le listener
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
		_ = m.conn.Close() // Ignore close error in cleanup
		m.conn = nil
	}
}
