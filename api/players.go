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

// withPlayer extrait le busName et appelle next
func withPlayer(
	next func(w http.ResponseWriter, r *http.Request, busName string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		next(w, r, busName)
	}
}

// withBody parse et valide le body JSON, puis appelle next
func withBody[T any](
	validate func(*T) error,
	next func(w http.ResponseWriter, r *http.Request, req *T),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
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

		next(w, r, &req)
	}
}

// handleMPRISError gère les erreurs MPRIS et retourne la réponse HTTP appropriée
func handleMPRISError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Gérer les erreurs de busName invalide
	var invalidBusNameErr *mpris.InvalidBusNameError
	if errors.As(err, &invalidBusNameErr) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Gérer les erreurs de player not found
	var notFoundErr *mpris.PlayerNotFoundError
	if errors.As(err, &notFoundErr) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Gérer les erreurs de capability
	var capErr *mpris.CapabilityError
	if errors.As(err, &capErr) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Handlers pour actions simples
func PlayHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.Play(busName))
	})
}

func PauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.Pause(busName))
	})
}

func PlayPauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.PlayPause(busName))
	})
}

func StopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.Stop(busName))
	})
}

func NextHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.Next(busName))
	})
}

func PreviousHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		handleMPRISError(w, m.Previous(busName))
	})
}

// Handlers pour actions avec body
func SeekHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.SeekRequest) {
			handleMPRISError(w, m.Seek(busName, req.Offset))
		})(w, r)
	})
}

func SetPositionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(
			func(req *mpris.PositionRequest) error {
				if req.TrackID == "" {
					return &validationError{"missing track_id"}
				}
				return nil
			},
			func(w http.ResponseWriter, r *http.Request, req *mpris.PositionRequest) {
				handleMPRISError(w, m.SetPosition(busName, req.TrackID, req.Position))
			},
		)(w, r)
	})
}

func SetVolumeHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(
			func(req *mpris.VolumeRequest) error {
				if req.Volume < 0 || req.Volume > 1 {
					return &validationError{"volume must be between 0 and 1"}
				}
				return nil
			},
			func(w http.ResponseWriter, r *http.Request, req *mpris.VolumeRequest) {
				handleMPRISError(w, m.SetVolume(busName, req.Volume))
			},
		)(w, r)
	})
}

func SetLoopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(
			func(req *mpris.LoopRequest) error {
				switch mpris.LoopStatus(req.Loop) {
				case mpris.LoopNone, mpris.LoopTrack, mpris.LoopPlaylist:
					return nil
				default:
					return &validationError{"loop must be None, Track, or Playlist"}
				}
			},
			func(w http.ResponseWriter, r *http.Request, req *mpris.LoopRequest) {
				handleMPRISError(w, m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop)))
			},
		)(w, r)
	})
}

func SetShuffleHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.ShuffleRequest) {
			handleMPRISError(w, m.SetShuffle(busName, req.Shuffle))
		})(w, r)
	})
}

type validationError struct {
	message string
}

func (e *validationError) Error() string {
	return e.message
}
