package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/b0bbywan/go-odio-api/backend/mpris"
)

// withPlayer extracts the busName and calls next
func withPlayer(
	next func(w http.ResponseWriter, r *http.Request, busName string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		busName := r.PathValue("player")
		next(w, r, busName)
	}
}

// handleMPRISError handles MPRIS errors and returns the appropriate HTTP response
func handleMPRISError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	// Handle invalid busName errors
	var invalidBusNameErr *mpris.InvalidBusNameError
	if errors.As(err, &invalidBusNameErr) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle validation errors
	var validErr *mpris.ValidationError
	if errors.As(err, &validErr) {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Handle player not found errors
	var notFoundErr *mpris.PlayerNotFoundError
	if errors.As(err, &notFoundErr) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Tracklist unsupported: the resource doesn't exist for this player
	var unsupportedErr *mpris.TracklistUnsupportedError
	if errors.As(err, &unsupportedErr) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Handle capability errors
	var capErr *mpris.CapabilityError
	if errors.As(err, &capErr) {
		http.Error(w, err.Error(), http.StatusForbidden)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

// Handlers for simple actions
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

// Handlers for actions with body
func SeekHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.SeekRequest) {
			handleMPRISError(w, m.Seek(busName, req.Offset))
		})(w, r)
	})
}

func SetPositionHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.PositionRequest) {
			handleMPRISError(w, m.SetPosition(busName, req.TrackID, req.Position))
		})(w, r)
	})
}

func SetVolumeHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.VolumeRequest) {
			handleMPRISError(w, m.SetVolume(busName, req.Volume))
		})(w, r)
	})
}

func SetLoopHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.LoopRequest) {
			handleMPRISError(w, m.SetLoopStatus(busName, mpris.LoopStatus(req.Loop)))
		})(w, r)
	})
}

func SetShuffleHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.ShuffleRequest) {
			handleMPRISError(w, m.SetShuffle(busName, req.Shuffle))
		})(w, r)
	})
}

// withTrack extracts the {trackid} parameter: the last segment of a track's
// object path (or the %2F-encoded full path), resolved against the cached
// tracklist by the backend.
func withTrack(
	next func(w http.ResponseWriter, r *http.Request, busName, trackRef string),
) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		next(w, r, busName, r.PathValue("trackid"))
	})
}

func TracklistHandler(getTracklist func(string) (*mpris.TracklistResponse, error)) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		resp, err := getTracklist(busName)
		if err != nil {
			handleMPRISError(w, err)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(resp); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
}

func GoToHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withTrack(func(w http.ResponseWriter, r *http.Request, busName, trackID string) {
		handleMPRISError(w, m.GoTo(busName, trackID))
	})
}

func RemoveTrackHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withTrack(func(w http.ResponseWriter, r *http.Request, busName, trackID string) {
		handleMPRISError(w, m.RemoveTrack(busName, trackID))
	})
}

func AddTrackHandler(m *mpris.MPRISBackend) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *mpris.AddTrackRequest) {
			handleMPRISError(w, m.AddTrack(busName, req.Uri, req.AfterTrack, req.SetAsCurrent))
		})(w, r)
	})
}

func CoverHandler(getPlayer func(string) (*mpris.Player, error)) http.HandlerFunc {
	return withPlayer(func(w http.ResponseWriter, r *http.Request, busName string) {
		player, err := getPlayer(busName)
		if err != nil {
			handleMPRISError(w, err)
			return
		}

		artUrl := player.Metadata["mpris:artUrl"]
		switch {
		case artUrl == "":
			http.NotFound(w, r)
		case strings.HasPrefix(artUrl, "file://"):
			// Standards-compliant MPRIS daemons percent-encode reserved
			// characters in file:// URIs (RFC 3986). Parse to recover the
			// decoded filesystem path.
			u, err := url.Parse(artUrl)
			if err != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeFile(w, r, u.Path)
		case strings.HasPrefix(artUrl, "http://"), strings.HasPrefix(artUrl, "https://"):
			http.Redirect(w, r, artUrl, http.StatusTemporaryRedirect)
		default:
			http.NotFound(w, r)
		}
	})
}
