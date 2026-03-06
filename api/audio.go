package api

import (
	"net/http"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

func AudioHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		clients, err := pa.ListClients()
		if err != nil {
			return nil, err
		}
		outputs, err := pa.ListOutputs()
		if err != nil {
			return nil, err
		}
		clientsUpdated := pa.CacheUpdatedAt()
		outputsUpdated := pa.OutputCacheUpdatedAt()
		if outputsUpdated.After(clientsUpdated) {
			setCacheHeader(w, outputsUpdated)
		} else {
			setCacheHeader(w, clientsUpdated)
		}
		return map[string]any{
			"clients": clients,
			"outputs": outputs,
		}, nil
	})
}

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

func SetDefaultOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		if err := pa.SetDefaultOutput(output); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func MuteOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		if err := pa.ToggleMuteOutput(output); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusAccepted)
	})
}

func SetVolumeOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			if err := pa.SetVolumeOutput(output, req.Volume); err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})(w, r)
	})
}

func withOutput(
	pa *pulseaudio.PulseAudioBackend,
	fn func(w http.ResponseWriter, r *http.Request, output string),
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		output := r.PathValue("output")
		if output == "" {
			http.Error(w, "missing output", http.StatusNotFound)
			return
		}
		fn(w, r, output)
	}
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
