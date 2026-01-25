package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

func MuteClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		sink := r.PathValue("sink")
		err := pa.SetMute(sink)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		w.WriteHeader(200)
	}
}
