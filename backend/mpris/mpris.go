package mpris

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/logger"
)

// validateBusName valide qu'un busName est conforme à MPRIS
func validateBusName(busName string) error {
	if busName == "" {
		return &InvalidBusNameError{BusName: busName, Reason: "empty bus name"}
	}
	if !strings.HasPrefix(busName, mprisPrefix+".") {
		return &InvalidBusNameError{BusName: busName, Reason: "must start with org.mpris.MediaPlayer2."}
	}
	// Vérifier qu'il ne contient pas de caractères dangereux
	if strings.Contains(busName, "..") || strings.Contains(busName, "/") || strings.ContainsAny(busName, "\x00\r\n") {
		return &InvalidBusNameError{BusName: busName, Reason: "contains illegal characters"}
	}
	return nil
}

// callWithTimeout exécute un appel D-Bus avec timeout
func callWithTimeout(call *dbus.Call) error {
	done := make(chan error, 1)

	go func() {
		done <- call.Err
	}()

	select {
	case err := <-done:
		return err
	case <-time.After(dbusCallTimeout):
		return &dbusTimeoutError{}
	}
}

type dbusTimeoutError struct{}

func (e *dbusTimeoutError) Error() string {
	return "D-Bus call timeout"
}

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
	err := m.conn.BusObject().Call(dbusListNamesMethod, 0).Store(&names)
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
func (m *MPRISBackend) GetPlayer(busName string) (*Player, error) {
	if err := validateBusName(busName); err != nil {
		return nil, err
	}

	players, ok := m.cache.Get(cacheKey)
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
	if err := validateBusName(busName); err != nil {
		return nil, err
	}

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
	if err := validateBusName(busName); err != nil {
		return err
	}

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
	// Créer un player et charger ses propriétés
	player := newPlayer(m.conn, busName)
	if err := player.Load(); err != nil {
		return Player{}, err
	}
	return *player, nil
}

// Play démarre la lecture
func (m *MPRISBackend) Play(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanPlay() {
		return &CapabilityError{Required: "CanPlay"}
	}

	logger.Debug("[mpris] playing %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Play", 0))
}

// Pause met en pause la lecture
func (m *MPRISBackend) Pause(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanPause() {
		return &CapabilityError{Required: "CanPause"}
	}

	logger.Debug("[mpris] pausing %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Pause", 0))
}

// PlayPause bascule entre lecture et pause
func (m *MPRISBackend) PlayPause(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanPlay() && !player.CanPause() {
		return &CapabilityError{Required: "CanPlay or CanPause"}
	}

	logger.Debug("[mpris] toggling play/pause for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".PlayPause", 0))
}

// Stop arrête la lecture
func (m *MPRISBackend) Stop(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] stopping %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Stop", 0))
}

// Next passe à la piste suivante
func (m *MPRISBackend) Next(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanGoNext() {
		return &CapabilityError{Required: "CanGoNext"}
	}

	logger.Debug("[mpris] next track for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Next", 0))
}

// Previous revient à la piste précédente
func (m *MPRISBackend) Previous(busName string) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanGoPrevious() {
		return &CapabilityError{Required: "CanGoPrevious"}
	}

	logger.Debug("[mpris] previous track for %s", busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Previous", 0))
}

// Seek déplace la position de lecture
func (m *MPRISBackend) Seek(busName string, offset int64) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanSeek() {
		return &CapabilityError{Required: "CanSeek"}
	}

	logger.Debug("[mpris] seeking %d for %s", offset, busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".Seek", 0, offset))
}

// SetPosition définit la position de lecture
func (m *MPRISBackend) SetPosition(busName, trackID string, position int64) error {
	if trackID == "" {
		return &ValidationError{Field: "track_id", Message: "cannot be empty"}
	}

	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanSeek() {
		return &CapabilityError{Required: "CanSeek"}
	}

	logger.Debug("[mpris] setting position to %d for %s", position, busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(mprisPlayerIface+".SetPosition", 0, dbus.ObjectPath(trackID), position))
}

// SetVolume définit le volume
func (m *MPRISBackend) SetVolume(busName string, volume float64) error {
	if volume < 0 || volume > 1 {
		return &ValidationError{Field: "volume", Message: "must be between 0 and 1"}
	}

	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] setting volume to %.2f for %s", volume, busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(dbusPropSet, 0, mprisPlayerIface, "Volume", dbus.MakeVariant(volume)))
}

// SetLoopStatus définit le statut de boucle
func (m *MPRISBackend) SetLoopStatus(busName string, status LoopStatus) error {
	switch status {
	case LoopNone, LoopTrack, LoopPlaylist:
		// Valid
	default:
		return &ValidationError{Field: "loop", Message: "must be None, Track, or Playlist"}
	}

	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] setting loop status to %s for %s", status, busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(dbusPropSet, 0, mprisPlayerIface, "LoopStatus", dbus.MakeVariant(string(status))))
}

// SetShuffle active/désactive le mode aléatoire
func (m *MPRISBackend) SetShuffle(busName string, shuffle bool) error {
	player, err := m.GetPlayer(busName)
	if err != nil {
		return err
	}
	if !player.CanControl() {
		return &CapabilityError{Required: "CanControl"}
	}

	logger.Debug("[mpris] setting shuffle to %v for %s", shuffle, busName)
	obj := m.conn.Object(busName, mprisPath)
	return callWithTimeout(obj.Call(dbusPropSet, 0, mprisPlayerIface, "Shuffle", dbus.MakeVariant(shuffle)))
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
