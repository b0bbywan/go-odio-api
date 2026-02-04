package api

import (
	"encoding/json"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/mpris"
)

type seekRequest struct {
	Offset int64 `json:"offset"`
}

type positionRequest struct {
	TrackID  string `json:"track_id"`
	Position int64  `json:"position"`
}

type volumeRequest struct {
	Volume float64 `json:"volume"`
}

type loopRequest struct {
	Loop string `json:"loop"`
}

type shuffleRequest struct {
	Shuffle bool `json:"shuffle"`
}

// ListPlayersHandler retourne la liste de tous les lecteurs MPRIS
func ListPlayersHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return m.ListPlayers()
	})
}

// withPlayer wrapper pour extraire le player et vérifier une capability
func withPlayer(
	m *mpris.MPRISBackend,
	checkCapability func(*mpris.Player) bool,
	capabilityName string,
	fn func(string) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if checkCapability != nil && !checkCapability(player) {
			http.Error(w, "action not allowed (requires "+capabilityName+")", http.StatusBadRequest)
			return
		}

		if err := fn(busName); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// PlayHandler lance la lecture
func PlayHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPlay }, "CanPlay", m.Play)
}

// PauseHandler met en pause
func PauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPause }, "CanPause", m.Pause)
}

// PlayPauseHandler bascule play/pause
func PlayPauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPlay || p.CanPause }, "CanPlay or CanPause", m.PlayPause)
}

// StopHandler arrête la lecture
func StopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanControl }, "CanControl", m.Stop)
}

// NextHandler passe à la piste suivante
func NextHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanGoNext }, "CanGoNext", m.Next)
}

// PreviousHandler revient à la piste précédente
func PreviousHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanGoPrevious }, "CanGoPrevious", m.Previous)
}

// SeekHandler déplace la position de lecture
func SeekHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if !player.CanSeek {
			http.Error(w, "action not allowed (requires CanSeek)", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		var req seekRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if err := m.Seek(busName, req.Offset); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// SetPositionHandler définit la position de lecture
func SetPositionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if !player.CanSeek {
			http.Error(w, "action not allowed (requires CanSeek)", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		var req positionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if req.TrackID == "" {
			http.Error(w, "missing track_id", http.StatusBadRequest)
			return
		}

		if err := m.SetPosition(busName, req.TrackID, req.Position); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// SetVolumeHandler définit le volume
func SetVolumeHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if !player.CanControl {
			http.Error(w, "action not allowed (requires CanControl)", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		var req volumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if req.Volume < 0 || req.Volume > 1 {
			http.Error(w, "volume must be between 0 and 1", http.StatusBadRequest)
			return
		}

		if err := m.SetVolume(busName, req.Volume); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// SetLoopHandler définit le mode de répétition
func SetLoopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if !player.CanControl {
			http.Error(w, "action not allowed (requires CanControl)", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		var req loopRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		// Valider le loop status
		switch mpris.LoopStatus(req.Loop) {
		case mpris.LoopNone, mpris.LoopTrack, mpris.LoopPlaylist:
			// OK
		default:
			http.Error(w, "loop must be None, Track, or Playlist", http.StatusBadRequest)
			return
		}

		if err := m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop)); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// SetShuffleHandler active/désactive le mode aléatoire
func SetShuffleHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		if !player.CanControl {
			http.Error(w, "action not allowed (requires CanControl)", http.StatusBadRequest)
			return
		}

		defer r.Body.Close()
		var req shuffleRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if err := m.SetShuffle(busName, req.Shuffle); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}
