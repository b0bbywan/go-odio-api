package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

func MuteClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		if err := pa.ToggleMute(sink); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func MuteMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := pa.ToggleMuteMaster(); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	}
}

func SetVolumeClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			if err := pa.SetVolume(sink, req.Volume); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})(w, r)
	})
}

func SetVolumeMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
		if err := pa.SetVolumeMaster(req.Volume); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func withSink(
	pa *pulseaudio.PulseAudioBackend,
	fn func(w http.ResponseWriter, r *http.Request, sink string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sink := r.PathValue("sink")
		if sink == "" {
			http.Error(w, "missing sink", http.StatusNotFound)
			return
		}

		fn(w, r, sink)
	}
}
