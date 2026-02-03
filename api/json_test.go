package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestJSONHandler(t *testing.T) {
	handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"status": "ok"}, nil
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Content-Type = %s, want application/json", w.Header().Get("Content-Type"))
	}

	var result map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &result)
	if err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if result["status"] != "ok" {
		t.Errorf("status = %s, want ok", result["status"])
	}
}

func TestJSONHandlerError(t *testing.T) {
	handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return nil, http.ErrServerClosed
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status code = %d, want 500", w.Code)
	}
}

func BenchmarkJSONHandler(b *testing.B) {
	handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
		return map[string]string{"test": "data"}, nil
	})

	req := httptest.NewRequest("GET", "/test", nil)

	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		handler(w, req)
	}
}
