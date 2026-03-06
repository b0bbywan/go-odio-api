package api

import (
	"errors"
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
			"kind":    pa.Kind(),
			"clients": clients,
			"outputs": outputs,
		}, nil
	})
}

func handleAudioError(w http.ResponseWriter, err error) {
	if err == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}

	var notFoundErr *pulseaudio.NotFoundError
	if errors.As(err, &notFoundErr) {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	var notReadyErr *pulseaudio.NotReadyError
	if errors.As(err, &notReadyErr) {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	http.Error(w, err.Error(), http.StatusInternalServerError)
}

func MuteClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		handleAudioError(w, pa.ToggleMute(sink))
	})
}

func MuteMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		handleAudioError(w, pa.ToggleMuteMaster())
	}
}

func SetVolumeClientHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withSink(pa, func(w http.ResponseWriter, r *http.Request, sink string) {
		withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			handleAudioError(w, pa.SetVolume(sink, req.Volume))
		})(w, r)
	})
}

func SetVolumeMasterHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
		handleAudioError(w, pa.SetVolumeMaster(req.Volume))
	})
}

func SetDefaultOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		handleAudioError(w, pa.SetDefaultOutput(output))
	})
}

func MuteOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		handleAudioError(w, pa.ToggleMuteOutput(output))
	})
}

func SetVolumeOutputHandler(pa *pulseaudio.PulseAudioBackend) http.HandlerFunc {
	return withOutput(pa, func(w http.ResponseWriter, r *http.Request, output string) {
		withBody(validateVolume, func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			handleAudioError(w, pa.SetVolumeOutput(output, req.Volume))
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
