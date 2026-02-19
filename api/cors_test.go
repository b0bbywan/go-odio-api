package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/b0bbywan/go-odio-api/config"
)

// nopHandler is a trivial handler that records it was reached.
var nopHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestCORSMiddlewareWildcard(t *testing.T) {
	cfg := &config.CORSConfig{Origins: []string{"*"}}
	handler := corsMiddleware(cfg)(nopHandler)

	tests := []struct {
		name           string
		origin         string
		wantOrigin     string
		wantVary       string
		wantStatusCode int
	}{
		{
			name:           "origin present — wildcard returned",
			origin:         "https://app.example.com",
			wantOrigin:     "*",
			wantStatusCode: http.StatusOK,
		},
		{
			name:           "no Origin header — no ACAO header",
			origin:         "",
			wantOrigin:     "",
			wantStatusCode: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}
			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("ACAO = %q, want %q", got, tt.wantOrigin)
			}
			if w.Header().Get("Vary") != "" {
				t.Errorf("Vary should not be set for wildcard, got %q", w.Header().Get("Vary"))
			}
		})
	}
}

func TestCORSMiddlewareSpecificOrigins(t *testing.T) {
	cfg := &config.CORSConfig{Origins: []string{
		"https://allowed.example.com",
		"https://other.example.com",
	}}
	handler := corsMiddleware(cfg)(nopHandler)

	tests := []struct {
		name       string
		origin     string
		wantOrigin string
		wantVary   bool
	}{
		{
			name:       "allowed origin reflected",
			origin:     "https://allowed.example.com",
			wantOrigin: "https://allowed.example.com",
			wantVary:   true,
		},
		{
			name:       "second allowed origin reflected",
			origin:     "https://other.example.com",
			wantOrigin: "https://other.example.com",
			wantVary:   true,
		},
		{
			name:       "unknown origin — no ACAO header",
			origin:     "https://evil.example.com",
			wantOrigin: "",
			wantVary:   false,
		},
		{
			name:       "no Origin header — no ACAO header",
			origin:     "",
			wantOrigin: "",
			wantVary:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			if tt.origin != "" {
				req.Header.Set("Origin", tt.origin)
			}
			w := httptest.NewRecorder()
			handler.ServeHTTP(w, req)

			if got := w.Header().Get("Access-Control-Allow-Origin"); got != tt.wantOrigin {
				t.Errorf("ACAO = %q, want %q", got, tt.wantOrigin)
			}
			hasVary := w.Header().Get("Vary") == "Origin"
			if hasVary != tt.wantVary {
				t.Errorf("Vary set = %v, want %v", hasVary, tt.wantVary)
			}
		})
	}
}

func TestCORSMiddlewarePreflight(t *testing.T) {
	cfg := &config.CORSConfig{Origins: []string{"*"}}
	handler := corsMiddleware(cfg)(nopHandler)

	req := httptest.NewRequest(http.MethodOptions, "/players/foo/play_pause", nil)
	req.Header.Set("Origin", "https://app.example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("preflight status = %d, want 204", w.Code)
	}
	if got := w.Header().Get("Access-Control-Allow-Origin"); got != "*" {
		t.Errorf("ACAO = %q, want *", got)
	}
	if got := w.Header().Get("Access-Control-Allow-Methods"); got == "" {
		t.Error("Access-Control-Allow-Methods should be set on preflight")
	}
	if got := w.Header().Get("Access-Control-Allow-Headers"); got == "" {
		t.Error("Access-Control-Allow-Headers should be set on preflight")
	}
}

func TestCORSMiddlewarePreflightDoesNotReachHandler(t *testing.T) {
	cfg := &config.CORSConfig{Origins: []string{"*"}}
	reached := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached = true
		w.WriteHeader(http.StatusOK)
	})
	handler := corsMiddleware(cfg)(inner)

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "https://app.example.com")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if reached {
		t.Error("inner handler should not be reached on OPTIONS preflight")
	}
}
