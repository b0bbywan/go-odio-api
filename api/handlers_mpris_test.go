package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/mpris"
)

// TestHandleMPRISError tests the MPRIS error mapping function
func TestHandleMPRISError(t *testing.T) {
	tests := []struct {
		name           string
		err            error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:           "no error returns 202 Accepted",
			err:            nil,
			wantStatusCode: http.StatusAccepted,
		},
		{
			name: "InvalidBusNameError returns 400 Bad Request",
			err: &mpris.InvalidBusNameError{
				BusName: "invalid",
				Reason:  "contains illegal characters",
			},
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "invalid player name",
		},
		{
			name: "ValidationError returns 400 Bad Request",
			err: &mpris.ValidationError{
				Field:   "volume",
				Message: "must be between 0 and 1",
			},
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "volume: must be between 0 and 1",
		},
		{
			name: "PlayerNotFoundError returns 404 Not Found",
			err: &mpris.PlayerNotFoundError{
				BusName: "org.mpris.MediaPlayer2.spotify",
			},
			wantStatusCode: http.StatusNotFound,
			wantBodyMatch:  "player not found",
		},
		{
			name: "CapabilityError returns 403 Forbidden",
			err: &mpris.CapabilityError{
				Required: "CanPlay",
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "action not allowed",
		},
		{
			name:           "generic error returns 500 Internal Server Error",
			err:            http.ErrServerClosed,
			wantStatusCode: http.StatusInternalServerError,
			wantBodyMatch:  "Server closed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			handleMPRISError(w, tt.err)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if tt.wantBodyMatch != "" {
				body := w.Body.String()
				if !strings.Contains(body, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", body, tt.wantBodyMatch)
				}
			}
		})
	}
}

// TestWithPlayer tests the middleware for extracting busName
func TestWithPlayer(t *testing.T) {
	tests := []struct {
		name      string
		busName   string
		wantCalls int
	}{
		{
			name:      "valid busName is passed to next handler",
			busName:   "org.mpris.MediaPlayer2.spotify",
			wantCalls: 1,
		},
		{
			name:      "empty busName is passed to next handler",
			busName:   "",
			wantCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var receivedBusName string

			nextFunc := func(w http.ResponseWriter, r *http.Request, busName string) {
				calls++
				receivedBusName = busName
				w.WriteHeader(http.StatusOK)
			}

			handler := withPlayer(nextFunc)

			req := httptest.NewRequest("POST", "/players/"+tt.busName+"/play", nil)
			req.SetPathValue("player", tt.busName)
			w := httptest.NewRecorder()

			handler(w, req)

			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}

			if receivedBusName != tt.busName {
				t.Errorf("busName = %q, want %q", receivedBusName, tt.busName)
			}
		})
	}
}

