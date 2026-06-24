package api

import (
	"errors"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/sendspin"
	"github.com/b0bbywan/go-odio-api/logger"
)

type setMuteRequest struct {
	Muted bool `json:"muted"`
}

func handleSendspinError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	if errors.Is(err, sendspin.ErrNotConnected) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}
	http.Error(w, err.Error(), http.StatusBadGateway)
}

func withSendspinAction(action func() error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleSendspinError(w, action())
	}
}

func (s *Server) registerSendspinRoutes(b *sendspin.SendspinBackend) {
	s.mux.HandleFunc(
		"GET /sendspin",
		JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return b.Status(), nil
		}),
	)
	s.mux.HandleFunc("POST /sendspin/play", withSendspinAction(b.Play))
	s.mux.HandleFunc("POST /sendspin/pause", withSendspinAction(b.Pause))
	s.mux.HandleFunc("POST /sendspin/stop", withSendspinAction(b.Stop))
	s.mux.HandleFunc(
		"POST /sendspin/volume",
		// Reuse the 0..1 convention of the audio endpoints; sendspin wants 0-100.
		withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			handleSendspinError(w, b.SetVolume(int(req.Volume*100)))
		}),
	)
	s.mux.HandleFunc(
		"POST /sendspin/mute",
		withBody(nil, func(w http.ResponseWriter, r *http.Request, req *setMuteRequest) {
			handleSendspinError(w, b.SetMuted(req.Muted))
		}),
	)
	logger.Info("[api] sendspin routes registered at /sendspin")
}
