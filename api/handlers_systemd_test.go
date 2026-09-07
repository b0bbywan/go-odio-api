package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

// TestHandleSystemdError tests the error mapping function - MOST CRITICAL TEST
func TestHandleSystemdError(t *testing.T) {
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
			name:           "PermissionSystemError returns 403 Forbidden",
			err:            &systemd.PermissionSystemError{Unit: "test.service"},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:           "PermissionUserError returns 403 Forbidden",
			err:            &systemd.PermissionUserError{Unit: "unmanaged.service"},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "cannot act on unmanaged user unit",
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
			handleSystemdError(w, tt.err)

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

// TestWithService tests the middleware for extracting scope and unit
func TestWithService(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		fn             func(string, systemd.UnitScope) error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "valid system scope and unit",
			pathScope: "system",
			pathUnit:  "test.service",
			fn: func(name string, scope systemd.UnitScope) error {
				if scope != systemd.ScopeSystem {
					t.Errorf("scope = %v, want %v", scope, systemd.ScopeSystem)
				}
				if name != "test.service" {
					t.Errorf("name = %q, want %q", name, "test.service")
				}
				return nil
			},
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:      "valid user scope and unit",
			pathScope: "user",
			pathUnit:  "user-service.service",
			fn: func(name string, scope systemd.UnitScope) error {
				if scope != systemd.ScopeUser {
					t.Errorf("scope = %v, want %v", scope, systemd.ScopeUser)
				}
				if name != "user-service.service" {
					t.Errorf("name = %q, want %q", name, "user-service.service")
				}
				return nil
			},
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:           "invalid scope returns 404",
			pathScope:      "invalid",
			pathUnit:       "test.service",
			wantStatusCode: http.StatusNotFound,
			wantBodyMatch:  "invalid scope",
		},
		{
			name:           "missing unit name returns 404",
			pathScope:      "user",
			pathUnit:       "",
			wantStatusCode: http.StatusNotFound,
			wantBodyMatch:  "missing unit name",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withService(nil, tt.fn)

			req := httptest.NewRequest("POST", "/services/scope/unit/start", nil)
			req.SetPathValue("scope", tt.pathScope)
			req.SetPathValue("unit", tt.pathUnit)
			w := httptest.NewRecorder()

			handler(w, req)

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
