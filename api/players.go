package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/mpris"
)

// ListPlayersHandler retourne la liste de tous les lecteurs MPRIS
func ListPlayersHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return m.ListPlayers()
	})
}

// withPlayer wrapper pour actions simples sans body
func withPlayer(
	fn func(string) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		if err := fn(busName); err != nil {
			// Gérer les erreurs de capability
			var capErr *mpris.CapabilityError
			if errors.As(err, &capErr) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// withPlayerAndBody wrapper générique pour actions avec body et validation
func withPlayerAndBody[T any](
	validate func(*T) error,
	action func(string, *T) error,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		if busName == "" {
			http.Error(w, "missing player name", http.StatusNotFound)
			return
		}

		defer r.Body.Close()
		var req T
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if validate != nil {
			if err := validate(&req); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
		}

		if err := action(busName, &req); err != nil {
			// Gérer les erreurs de capability
			var capErr *mpris.CapabilityError
			if errors.As(err, &capErr) {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// Handlers pour actions simples
func PlayHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.Play)
}

func PauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.Pause)
}

func PlayPauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.PlayPause)
}

func StopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.Stop)
}

func NextHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.Next)
}

func PreviousHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m.Previous)
}

// Handlers pour actions avec body
func SeekHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		nil,
		func(busName string, req *mpris.SeekRequest) error {
			return m.Seek(busName, req.Offset)
		},
	)
}

func SetPositionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		func(req *mpris.PositionRequest) error {
			if req.TrackID == "" {
				return &validationError{"missing track_id"}
			}
			return nil
		},
		func(busName string, req *mpris.PositionRequest) error {
			return m.SetPosition(busName, req.TrackID, req.Position)
		},
	)
}

func SetVolumeHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		func(req *mpris.VolumeRequest) error {
			if req.Volume < 0 || req.Volume > 1 {
				return &validationError{"volume must be between 0 and 1"}
			}
			return nil
		},
		func(busName string, req *mpris.VolumeRequest) error {
			return m.SetVolume(busName, req.Volume)
		},
	)
}

func SetLoopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		func(req *mpris.LoopRequest) error {
			switch mpris.LoopStatus(req.Loop) {
			case mpris.LoopNone, mpris.LoopTrack, mpris.LoopPlaylist:
				return nil
			default:
				return &validationError{"loop must be None, Track, or Playlist"}
			}
		},
		func(busName string, req *mpris.LoopRequest) error {
			return m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop))
		},
	)
}

func SetShuffleHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		nil,
		func(busName string, req *mpris.ShuffleRequest) error {
			return m.SetShuffle(busName, req.Shuffle)
		},
	)
}

type validationError struct {
	message string
}

func (e *validationError) Error() string {
	return e.message
}
