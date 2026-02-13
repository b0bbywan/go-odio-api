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
