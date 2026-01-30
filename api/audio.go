package api

import (
	"encoding/json"
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

func MuteClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		if err := pa.ToggleMute(sink); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func MuteMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pa.ToggleMuteMaster(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func SetVolumeClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		withVolume(func(w http.ResponseWriter, r *http.Request, volume float32) {
			if err := pa.SetVolume(sink, volume); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		})(w, r)
	})
}

func SetVolumeMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withVolume(func(w http.ResponseWriter, r *http.Request, volume float32) {
		if err := pa.SetVolumeMaster(volume); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
}

func withSink(
	pa *pulseaudio.PulseAudioBackend,
	fn func(w http.ResponseWriter, r *http.Request, sink string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sink := r.PathValue("sink")
		if sink == "" {
			http.Error(w, "missing sink", http.StatusBadRequest)
			return
		}

		fn(w, r, sink)
	}
}

func withVolume(
	next func(w http.ResponseWriter, r *http.Request, volume float32),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()

		var req setVolumeRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid JSON payload", http.StatusBadRequest)
			return
		}

		if req.Volume < 0 || req.Volume > 1 {
			http.Error(w, "volume must be between 0 and 1", http.StatusBadRequest)
			return
		}

		next(w, r, req.Volume)
	}
}
