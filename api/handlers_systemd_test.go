package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

// Mock systemd backend for testing
type mockSystemdBackend struct {
	enableFunc       func(string, systemd.UnitScope) error
	disableFunc      func(string, systemd.UnitScope) error
	startFunc        func(string, systemd.UnitScope) error
	stopFunc         func(string, systemd.UnitScope) error
	restartFunc      func(string, systemd.UnitScope) error
	listServicesFunc func() ([]systemd.Service, error)
}

func (m *mockSystemdBackend) EnableService(name string, scope systemd.UnitScope) error {
	if m.enableFunc != nil {
		return m.enableFunc(name, scope)
	}
	return nil
}

func (m *mockSystemdBackend) DisableService(name string, scope systemd.UnitScope) error {
	if m.disableFunc != nil {
		return m.disableFunc(name, scope)
	}
	return nil
}

func (m *mockSystemdBackend) StartService(name string, scope systemd.UnitScope) error {
	if m.startFunc != nil {
		return m.startFunc(name, scope)
	}
	return nil
}

func (m *mockSystemdBackend) StopService(name string, scope systemd.UnitScope) error {
	if m.stopFunc != nil {
		return m.stopFunc(name, scope)
	}
	return nil
}

func (m *mockSystemdBackend) RestartService(name string, scope systemd.UnitScope) error {
	if m.restartFunc != nil {
		return m.restartFunc(name, scope)
	}
	return nil
}

func (m *mockSystemdBackend) ListServices() ([]systemd.Service, error) {
	if m.listServicesFunc != nil {
		return m.listServicesFunc()
	}
	return []systemd.Service{}, nil
}

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
		mockFunc       func(string, systemd.UnitScope) error
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "valid system scope and unit",
			pathScope: "system",
			pathUnit:  "test.service",
			mockFunc: func(name string, scope systemd.UnitScope) error {
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
			mockFunc: func(name string, scope systemd.UnitScope) error {
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
			mock := &mockSystemdBackend{
				startFunc: tt.mockFunc,
			}

			handler := withService(mock, mock.StartService)

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

// TestStartServiceHandler - CRITICAL: system scope must ALWAYS return 403
func TestStartServiceHandler(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		setupMock      func() *mockSystemdBackend
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "system scope always returns 403 Forbidden",
			pathScope: "system",
			pathUnit:  "test.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					startFunc: func(name string, scope systemd.UnitScope) error {
						// Simulate backend behavior - always returns PermissionSystemError
						return &systemd.PermissionSystemError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:      "user scope with whitelisted unit returns 202",
			pathScope: "user",
			pathUnit:  "allowed.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					startFunc: func(name string, scope systemd.UnitScope) error {
						return nil // Success
					},
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
		{
			name:      "user scope with non-whitelisted unit returns 403",
			pathScope: "user",
			pathUnit:  "unmanaged.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					startFunc: func(name string, scope systemd.UnitScope) error {
						return &systemd.PermissionUserError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "cannot act on unmanaged user unit",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			handler := withService(mock, mock.StartService)

			req := httptest.NewRequest("POST", "/services/"+tt.pathScope+"/"+tt.pathUnit+"/start", nil)
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

// TestStopServiceHandler - Same security requirements as Start
func TestStopServiceHandler(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		setupMock      func() *mockSystemdBackend
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "system scope always returns 403 Forbidden",
			pathScope: "system",
			pathUnit:  "test.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					stopFunc: func(name string, scope systemd.UnitScope) error {
						return &systemd.PermissionSystemError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:      "user scope with whitelisted unit returns 202",
			pathScope: "user",
			pathUnit:  "allowed.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					stopFunc: func(name string, scope systemd.UnitScope) error {
						return nil
					},
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			handler := withService(mock, mock.StopService)

			req := httptest.NewRequest("POST", "/services/"+tt.pathScope+"/"+tt.pathUnit+"/stop", nil)
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

// TestEnableServiceHandler - Enable also requires system scope protection
func TestEnableServiceHandler(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		setupMock      func() *mockSystemdBackend
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "system scope always returns 403 Forbidden",
			pathScope: "system",
			pathUnit:  "test.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					enableFunc: func(name string, scope systemd.UnitScope) error {
						return &systemd.PermissionSystemError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:      "user scope with whitelisted unit returns 202",
			pathScope: "user",
			pathUnit:  "allowed.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					enableFunc: func(name string, scope systemd.UnitScope) error {
						return nil
					},
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			handler := withService(mock, mock.EnableService)

			req := httptest.NewRequest("POST", "/services/"+tt.pathScope+"/"+tt.pathUnit+"/enable", nil)
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

// TestDisableServiceHandler - Disable also requires system scope protection
func TestDisableServiceHandler(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		setupMock      func() *mockSystemdBackend
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "system scope always returns 403 Forbidden",
			pathScope: "system",
			pathUnit:  "test.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					disableFunc: func(name string, scope systemd.UnitScope) error {
						return &systemd.PermissionSystemError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:      "user scope with whitelisted unit returns 202",
			pathScope: "user",
			pathUnit:  "allowed.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					disableFunc: func(name string, scope systemd.UnitScope) error {
						return nil
					},
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			handler := withService(mock, mock.DisableService)

			req := httptest.NewRequest("POST", "/services/"+tt.pathScope+"/"+tt.pathUnit+"/disable", nil)
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

// TestRestartServiceHandler - Restart also requires system scope protection
func TestRestartServiceHandler(t *testing.T) {
	tests := []struct {
		name           string
		pathScope      string
		pathUnit       string
		setupMock      func() *mockSystemdBackend
		wantStatusCode int
		wantBodyMatch  string
	}{
		{
			name:      "system scope always returns 403 Forbidden",
			pathScope: "system",
			pathUnit:  "test.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					restartFunc: func(name string, scope systemd.UnitScope) error {
						return &systemd.PermissionSystemError{Unit: name}
					},
				}
			},
			wantStatusCode: http.StatusForbidden,
			wantBodyMatch:  "can not act on system units",
		},
		{
			name:      "user scope with whitelisted unit returns 202",
			pathScope: "user",
			pathUnit:  "allowed.service",
			setupMock: func() *mockSystemdBackend {
				return &mockSystemdBackend{
					restartFunc: func(name string, scope systemd.UnitScope) error {
						return nil
					},
				}
			},
			wantStatusCode: http.StatusAccepted,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mock := tt.setupMock()
			handler := withService(mock, mock.RestartService)

			req := httptest.NewRequest("POST", "/services/"+tt.pathScope+"/"+tt.pathUnit+"/restart", nil)
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
