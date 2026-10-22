package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

func TestWithProcessing(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{name: "applied", want: http.StatusAccepted},
		{name: "unknown feature", err: &pulseaudio.NotFoundError{Resource: "processing feature", Name: "reverb"}, want: http.StatusNotFound},
		{name: "not connected", err: &pulseaudio.NotReadyError{Message: "not ready"}, want: http.StatusServiceUnavailable},
		{name: "feature in error", err: errors.New("module initialization failed"), want: http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got string
			mux := http.NewServeMux()
			mux.HandleFunc("POST /audio/processing/{feature}/enable", withProcessing(func(feature string) error {
				got = feature
				return tt.err
			}))
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, httptest.NewRequest("POST", "/audio/processing/network_sink/enable", nil))

			if got != "network_sink" {
				t.Errorf("feature = %q, want network_sink", got)
			}
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}
