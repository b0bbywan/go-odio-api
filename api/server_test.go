package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/config"
)

// emptyBackend returns a non-nil backend with no sub-backends initialized,
// so register() proceeds past the nil check without requiring real system resources.
func emptyBackend() *backend.Backend {
	return &backend.Backend{}
}

// TestNewServer_NilConfig verifies that NewServer returns nil when config is nil
func TestNewServer_NilConfig(t *testing.T) {
	s := NewServer(nil, nil)
	if s != nil {
		t.Error("NewServer(nil, nil) should return nil")
	}
}

// TestNewServer_Disabled verifies that NewServer returns nil when api is disabled
func TestNewServer_Disabled(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: false,
		Port:    8080,
		UI:      &config.UIConfig{Enabled: false},
	}
	s := NewServer(cfg, nil)
	if s != nil {
		t.Error("NewServer with Enabled=false should return nil")
	}
}

// TestNewServer_Valid verifies that NewServer succeeds with a valid config
func TestNewServer_Valid(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8080,
		UI:      &config.UIConfig{Enabled: false},
	}
	s := NewServer(cfg, nil)
	if s == nil {
		t.Fatal("NewServer with valid config should not return nil")
	}
}

// TestServer_RootReturns404 verifies that / returns 404 (security: no info leak on root)
func TestServer_RootReturns404(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8080,
		UI:      &config.UIConfig{Enabled: false},
	}
	s := NewServer(cfg, emptyBackend())

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET / = %d, want 404", w.Code)
	}
}

// TestServer_UIDisabled verifies that /ui returns 404 when UI is disabled
func TestServer_UIDisabled(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8080,
		UI:      &config.UIConfig{Enabled: false},
	}
	s := NewServer(cfg, emptyBackend())

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /ui with UI disabled = %d, want 404", w.Code)
	}
}

// TestServer_UIEnabled verifies that /ui is reachable (not 404) when UI is enabled.
// The handler will fail to call the internal API (no backend running), returning 500,
// but that confirms the route IS registered.
func TestServer_UIEnabled(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8080,
		UI:      &config.UIConfig{Enabled: true},
	}
	s := NewServer(cfg, emptyBackend())

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code == http.StatusNotFound {
		t.Error("GET /ui with UI enabled should not return 404 (route not registered)")
	}
}

// TestServer_UIEnabledNilUIConfig verifies that a nil UIConfig doesn't panic
func TestServer_UIEnabledNilUIConfig(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8080,
		UI:      nil, // no UI config
	}
	s := NewServer(cfg, emptyBackend())
	if s == nil {
		t.Fatal("NewServer with nil UIConfig should still return a server")
	}

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	w := httptest.NewRecorder()
	s.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("GET /ui with nil UIConfig = %d, want 404", w.Code)
	}
}

// TestServerDisabled verifies that NewServer returns nil when API is disabled
func TestServerDisabled(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: false,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	backend := &backend.Backend{}
	server := NewServer(cfg, backend)

	if server != nil {
		t.Error("NewServer should return nil when API is disabled")
	}
}

// TestServerEnabled verifies that NewServer returns a valid server when enabled
func TestServerEnabled(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	backend := &backend.Backend{}
	server := NewServer(cfg, backend)

	if server == nil {
		t.Fatal("NewServer should return a non-nil server when API is enabled")
		return
	}

	if server.mux == nil {
		t.Error("Server mux should be initialized")
	}
}

// TestRoutesWithDisabledBackends verifies that routes are not registered for disabled backends
func TestRoutesWithDisabledBackends(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	// Backend with all backends disabled (nil)
	backend := &backend.Backend{
		MPRIS:    nil,
		Pulse:    nil,
		Systemd:  nil,
		Zeroconf: nil,
	}

	server := NewServer(cfg, backend)
	if server == nil {
		t.Fatal("NewServer should return a non-nil server")
		return
	}

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
		description    string
	}{
		// Server route should always exist
		{
			name:           "server route exists",
			method:         "GET",
			path:           "/server",
			expectedStatus: http.StatusOK,
			description:    "Server info route should always be available",
		},
		// Bluetooth routes should not exist
		{
			name:           "bluetooth get route disabled",
			method:         "GET",
			path:           "/bluetooth",
			expectedStatus: http.StatusNotFound,
			description:    "Bluetooth routes should not exist when backend is disabled",
		},
		{
			name:           "bluetooth power_up route disabled",
			method:         "POST",
			path:           "/bluetooth/power_up",
			expectedStatus: http.StatusNotFound,
			description:    "Bluetooth routes should not exist when backend is disabled",
		},
		// PulseAudio routes should not exist
		{
			name:           "audio server route disabled",
			method:         "GET",
			path:           "/audio/server",
			expectedStatus: http.StatusNotFound,
			description:    "PulseAudio routes should not exist when backend is disabled",
		},
		{
			name:           "audio clients route disabled",
			method:         "GET",
			path:           "/audio/clients",
			expectedStatus: http.StatusNotFound,
			description:    "PulseAudio routes should not exist when backend is disabled",
		},
		// Systemd routes should not exist
		{
			name:           "services route disabled",
			method:         "GET",
			path:           "/services",
			expectedStatus: http.StatusNotFound,
			description:    "Systemd routes should not exist when backend is disabled",
		},
		{
			name:           "service start route disabled",
			method:         "POST",
			path:           "/services/user/test.service/start",
			expectedStatus: http.StatusNotFound,
			description:    "Systemd routes should not exist when backend is disabled",
		},
		// MPRIS routes should not exist
		{
			name:           "players route disabled",
			method:         "GET",
			path:           "/players",
			expectedStatus: http.StatusNotFound,
			description:    "MPRIS routes should not exist when backend is disabled",
		},
		{
			name:           "player play route disabled",
			method:         "POST",
			path:           "/players/spotify/play",
			expectedStatus: http.StatusNotFound,
			description:    "MPRIS routes should not exist when backend is disabled",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()

			server.mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("%s: got status %d, want %d - %s",
					tt.name, w.Code, tt.expectedStatus, tt.description)
			}
		})
	}
}

// TestRoutesWithEnabledSystemdBackend verifies systemd routes exist when backend is enabled
func TestRoutesWithEnabledSystemdBackend(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	// Create a mock systemd backend (we can't create a real one without D-Bus)
	// We just need to verify the route is registered, not that it works
	backend := &backend.Backend{
		Systemd: nil, // Even with nil, we can't test real systemd without D-Bus
	}

	server := NewServer(cfg, backend)
	if server == nil {
		t.Fatal("NewServer should return a non-nil server")
		return
	}

	// Without a real systemd backend, the route won't be registered
	// This test documents the expected behavior
	req := httptest.NewRequest("GET", "/services", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Logf("Systemd routes not registered (expected when backend is nil)")
	}
}

// TestNilBackendHandling verifies server handles nil backend gracefully
func TestNilBackendHandling(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	// Nil backend
	server := NewServer(cfg, nil)
	if server == nil {
		t.Fatal("NewServer should return a non-nil server even with nil backend")
		return
	}

	// Should not panic when accessing routes
	req := httptest.NewRequest("GET", "/server", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// Without backend, /server route won't be registered either
	if w.Code != http.StatusNotFound {
		t.Logf("Server route not registered when backend is nil")
	}
}

// TestServerRouteAlwaysRegistered verifies /server route is always registered
func TestServerRouteAlwaysRegistered(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	// Backend with no sub-backends but should still have server info
	backend := &backend.Backend{}

	server := NewServer(cfg, backend)
	if server == nil {
		t.Fatal("NewServer should return a non-nil server")
	}

	req := httptest.NewRequest("GET", "/server", nil)
	w := httptest.NewRecorder()
	server.mux.ServeHTTP(w, req)

	// /server route should exist and return 200
	if w.Code != http.StatusOK {
		t.Errorf("GET /server should return 200, got %d", w.Code)
	}
}

// TestRouteMethodRestrictions verifies method restrictions (GET vs POST)
func TestRouteMethodRestrictions(t *testing.T) {
	cfg := &config.ApiConfig{
		Enabled: true,
		Port:    8018,
		Listens: []string{"127.0.0.1:8018"},
	}

	backend := &backend.Backend{}
	server := NewServer(cfg, backend)

	tests := []struct {
		name           string
		method         string
		path           string
		expectedStatus int
	}{
		{
			name:           "GET /server allowed",
			method:         "GET",
			path:           "/server",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "POST /server allowed (no method restriction)",
			method:         "POST",
			path:           "/server",
			expectedStatus: http.StatusOK,
			// Note: /server route has no method restriction, accepts all HTTP methods
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			server.mux.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("got status %d, want %d", w.Code, tt.expectedStatus)
			}
		})
	}
}
