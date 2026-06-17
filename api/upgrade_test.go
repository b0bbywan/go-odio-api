package api

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/upgrade"
)

func TestWithUpgradeAction(t *testing.T) {
	cases := []struct {
		name     string
		err      error
		wantCode int
	}{
		{"success", nil, http.StatusAccepted},
		{"unit not configured", upgrade.ErrUnitNotConfigured, http.StatusNotFound},
		{"already in progress", upgrade.ErrUpgradeInProgress, http.StatusConflict},
		{"upstream failure", errors.New("dbus boom"), http.StatusBadGateway},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			handler := withUpgradeAction(func() error { return tc.err })
			rec := httptest.NewRecorder()
			handler(rec, httptest.NewRequest(http.MethodPost, "/upgrade/start", nil))
			if rec.Code != tc.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tc.wantCode)
			}
		})
	}
}
