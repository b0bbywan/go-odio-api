package api

import (
	"encoding/json"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/mpris"
)

type playerActionRequest struct {
	Action   string  `json:"action"`
	Offset   *int64  `json:"offset,omitempty"`    // pour seek
	Position *int64  `json:"position,omitempty"`  // pour set_position
	TrackID  string  `json:"track_id,omitempty"`  // pour set_position
	Volume   *float64 `json:"volume,omitempty"`    // pour set_volume
	Loop     string  `json:"loop,omitempty"`      // pour set_loop (None, Track, Playlist)
	Shuffle  *bool   `json:"shuffle,omitempty"`   // pour set_shuffle
}

// ListPlayersHandler retourne la liste de tous les lecteurs MPRIS
func ListPlayersHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return m.ListPlayers()
	})
}

// PlayerActionHandler exécute une action sur un lecteur MPRIS
func PlayerActionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		defer r.Body.Close()

		var req playerActionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		// Récupérer le player depuis le cache pour vérifier les capabilities
		player, found := m.GetPlayer(busName)
		if !found {
			http.Error(w, "player not found", http.StatusNotFound)
			return
		}

		// Vérifier que l'action est permise selon les capabilities
		if err := validateAction(req, player); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		// Exécuter l'action
		if err := executeAction(m, busName, req); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// validateAction vérifie que l'action est permise selon les capabilities du player
func validateAction(req playerActionRequest, player *mpris.Player) error {
	switch req.Action {
	case "play":
		if !player.CanPlay {
			return errActionNotAllowed("play", "CanPlay")
		}
	case "pause":
		if !player.CanPause {
			return errActionNotAllowed("pause", "CanPause")
		}
	case "play_pause":
		if !player.CanPlay && !player.CanPause {
			return errActionNotAllowed("play_pause", "CanPlay or CanPause")
		}
	case "stop":
		if !player.CanControl {
			return errActionNotAllowed("stop", "CanControl")
		}
	case "next":
		if !player.CanGoNext {
			return errActionNotAllowed("next", "CanGoNext")
		}
	case "previous":
		if !player.CanGoPrevious {
			return errActionNotAllowed("previous", "CanGoPrevious")
		}
	case "seek":
		if !player.CanSeek {
			return errActionNotAllowed("seek", "CanSeek")
		}
		if req.Offset == nil {
			return errMissingParam("offset")
		}
	case "set_position":
		if !player.CanSeek {
			return errActionNotAllowed("set_position", "CanSeek")
		}
		if req.Position == nil {
			return errMissingParam("position")
		}
		if req.TrackID == "" {
			return errMissingParam("track_id")
		}
	case "set_volume":
		if !player.CanControl {
			return errActionNotAllowed("set_volume", "CanControl")
		}
		if req.Volume == nil {
			return errMissingParam("volume")
		}
		if *req.Volume < 0 || *req.Volume > 1 {
			return errInvalidParam("volume", "must be between 0 and 1")
		}
	case "set_loop":
		if !player.CanControl {
			return errActionNotAllowed("set_loop", "CanControl")
		}
		if req.Loop == "" {
			return errMissingParam("loop")
		}
		// Valider que le loop status est valide
		switch mpris.LoopStatus(req.Loop) {
		case mpris.LoopNone, mpris.LoopTrack, mpris.LoopPlaylist:
			// OK
		default:
			return errInvalidParam("loop", "must be None, Track, or Playlist")
		}
	case "set_shuffle":
		if !player.CanControl {
			return errActionNotAllowed("set_shuffle", "CanControl")
		}
		if req.Shuffle == nil {
			return errMissingParam("shuffle")
		}
	default:
		return errUnknownAction(req.Action)
	}

	return nil
}

// executeAction exécute l'action sur le backend MPRIS
func executeAction(m *mpris.MPRISBackend, busName string, req playerActionRequest) error {
	switch req.Action {
	case "play":
		return m.Play(busName)
	case "pause":
		return m.Pause(busName)
	case "play_pause":
		return m.PlayPause(busName)
	case "stop":
		return m.Stop(busName)
	case "next":
		return m.Next(busName)
	case "previous":
		return m.Previous(busName)
	case "seek":
		return m.Seek(busName, *req.Offset)
	case "set_position":
		return m.SetPosition(busName, req.TrackID, *req.Position)
	case "set_volume":
		return m.SetVolume(busName, *req.Volume)
	case "set_loop":
		return m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop))
	case "set_shuffle":
		return m.SetShuffle(busName, *req.Shuffle)
	default:
		return errUnknownAction(req.Action)
	}
}

// Helper functions pour générer des erreurs claires
func errActionNotAllowed(action, capability string) error {
	return &APIError{
		Message: "action not allowed: " + action + " (requires " + capability + ")",
	}
}

func errMissingParam(param string) error {
	return &APIError{
		Message: "missing required parameter: " + param,
	}
}

func errInvalidParam(param, reason string) error {
	return &APIError{
		Message: "invalid parameter " + param + ": " + reason,
	}
}

func errUnknownAction(action string) error {
	return &APIError{
		Message: "unknown action: " + action,
	}
}

type APIError struct {
	Message string
}

func (e *APIError) Error() string {
	return e.Message
}
