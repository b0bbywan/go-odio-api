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

// withPlayer wrapper pour actions simples sans body
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

// withPlayerAndBody wrapper générique pour actions avec body et validation
func withPlayerAndBody[T any](
	m *mpris.MPRISBackend,
	checkCapability func(*mpris.Player) bool,
	capabilityName string,
	validate func(*T) error,
	action func(string, *T) error,
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
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusAccepted)
	}
}

// Handlers pour actions simples
func PlayHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPlay }, "CanPlay", m.Play)
}

func PauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPause }, "CanPause", m.Pause)
}

func PlayPauseHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanPlay || p.CanPause }, "CanPlay or CanPause", m.PlayPause)
}

func StopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanControl }, "CanControl", m.Stop)
}

func NextHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanGoNext }, "CanGoNext", m.Next)
}

func PreviousHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(m, func(p *mpris.Player) bool { return p.CanGoPrevious }, "CanGoPrevious", m.Previous)
}

// Handlers pour actions avec body
func SeekHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		m,
		func(p *mpris.Player) bool { return p.CanSeek },
		"CanSeek",
		nil,
		func(busName string, req *seekRequest) error {
			return m.Seek(busName, req.Offset)
		},
	)
}

func SetPositionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		m,
		func(p *mpris.Player) bool { return p.CanSeek },
		"CanSeek",
		func(req *positionRequest) error {
			if req.TrackID == "" {
				return &validationError{"missing track_id"}
			}
			return nil
		},
		func(busName string, req *positionRequest) error {
			return m.SetPosition(busName, req.TrackID, req.Position)
		},
	)
}

func SetVolumeHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		m,
		func(p *mpris.Player) bool { return p.CanControl },
		"CanControl",
		func(req *volumeRequest) error {
			if req.Volume < 0 || req.Volume > 1 {
				return &validationError{"volume must be between 0 and 1"}
			}
			return nil
		},
		func(busName string, req *volumeRequest) error {
			return m.SetVolume(busName, req.Volume)
		},
	)
}

func SetLoopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		m,
		func(p *mpris.Player) bool { return p.CanControl },
		"CanControl",
		func(req *loopRequest) error {
			switch mpris.LoopStatus(req.Loop) {
			case mpris.LoopNone, mpris.LoopTrack, mpris.LoopPlaylist:
				return nil
			default:
				return &validationError{"loop must be None, Track, or Playlist"}
			}
		},
		func(busName string, req *loopRequest) error {
			return m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop))
		},
	)
}

func SetShuffleHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayerAndBody(
		m,
		func(p *mpris.Player) bool { return p.CanControl },
		"CanControl",
		nil,
		func(busName string, req *shuffleRequest) error {
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
