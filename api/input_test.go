package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/b0bbywan/go-odio-api/backend/input"
)

func TestInputRoute(t *testing.T) {
	s := &Server{mux: http.NewServeMux()}
	s.registerInputRoutes(&input.InputBackend{})

	rec := httptest.NewRecorder()
	s.mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/input", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	// Lists must serialise as [] rather than null when nothing is handled.
	if got, want := strings.TrimSpace(rec.Body.String()), `{"devices":[],"bindings":[]}`; got != want {
		t.Errorf("body = %s, want %s", got, want)
	}
}
