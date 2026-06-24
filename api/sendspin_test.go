package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/sendspin"
)

func TestHandleSendspinError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"success", nil, http.StatusAccepted},
		{"not connected", sendspin.ErrNotConnected, http.StatusServiceUnavailable},
		{"upstream failure", errors.New("boom"), http.StatusBadGateway},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleSendspinError(w, tt.err)
			if w.Code != tt.want {
				t.Errorf("status = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

func TestWithSendspinActionPropagatesError(t *testing.T) {
	h := withSendspinAction(func() error { return sendspin.ErrNotConnected })
	w := httptest.NewRecorder()
	h(w, httptest.NewRequest("POST", "/sendspin/play", nil))
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", w.Code, http.StatusServiceUnavailable)
	}
}
