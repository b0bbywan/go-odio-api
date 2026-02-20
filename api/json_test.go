package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestJSONHandlerStatusError(t *testing.T) {
	tests := []struct {
		name     string
		code     int
		msg      string
		wantCode int
	}{
		{"404", http.StatusNotFound, "not found", http.StatusNotFound},
		{"403", http.StatusForbidden, "forbidden", http.StatusForbidden},
		{"400", http.StatusBadRequest, "bad request", http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
				return nil, httpError(tt.code, errors.New(tt.msg))
			})

			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			handler(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantCode)
			}
			if body := w.Body.String(); !strings.Contains(body, tt.msg) {
				t.Errorf("body = %q, want to contain %q", body, tt.msg)
			}
		})
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

// TestWithBodyContentType tests Content-Type validation
func TestWithBodyContentType(t *testing.T) {
	tests := []struct {
		name        string
		contentType string
		wantCode    int
		wantBody    string
	}{
		{
			name:        "missing Content-Type",
			contentType: "",
			wantCode:    http.StatusUnsupportedMediaType,
			wantBody:    "Content-Type must be application/json",
		},
		{
			name:        "wrong Content-Type",
			contentType: "text/plain",
			wantCode:    http.StatusUnsupportedMediaType,
			wantBody:    "Content-Type must be application/json",
		},
		{
			name:        "valid Content-Type",
			contentType: "application/json",
			wantCode:    http.StatusOK,
			wantBody:    "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withBody(
				nil,
				func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
					w.WriteHeader(http.StatusOK)
				},
			)

			body := bytes.NewBufferString(`{"volume": 0.5}`)
			req := httptest.NewRequest("POST", "/test", body)
			if tt.contentType != "" {
				req.Header.Set("Content-Type", tt.contentType)
			}
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("response body = %q, want to contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestWithBodySizeLimit tests request body size limit
func TestWithBodySizeLimit(t *testing.T) {
	tests := []struct {
		name     string
		bodySize int
		wantCode int
		wantBody string
	}{
		{
			name:     "small body (1KB)",
			bodySize: 1024,
			wantCode: http.StatusOK,
			wantBody: "",
		},
		{
			name:     "medium body (500KB)",
			bodySize: 500 * 1024,
			wantCode: http.StatusOK,
			wantBody: "",
		},
		{
			name:     "body at limit (1MB)",
			bodySize: 1 << 20,
			wantCode: http.StatusRequestEntityTooLarge,
			wantBody: "request body too large",
		},
		{
			name:     "body over limit (2MB)",
			bodySize: 2 << 20,
			wantCode: http.StatusRequestEntityTooLarge,
			wantBody: "request body too large",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := withBody(
				nil,
				func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
					w.WriteHeader(http.StatusOK)
				},
			)

			// Create a large JSON body
			body := bytes.NewBufferString(`{"volume": 0.5, "data": "`)
			body.WriteString(strings.Repeat("a", tt.bodySize))
			body.WriteString(`"}`)

			req := httptest.NewRequest("POST", "/test", body)
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("status code = %d, want %d", w.Code, tt.wantCode)
			}

			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("response body = %q, want to contain %q", w.Body.String(), tt.wantBody)
			}
		})
	}
}

// TestWithBodyValidJSON tests that valid JSON with proper Content-Type works
func TestWithBodyValidJSON(t *testing.T) {
	handler := withBody(
		validateVolume,
		func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
			if req.Volume != 0.5 {
				t.Errorf("volume = %f, want 0.5", req.Volume)
			}
			w.WriteHeader(http.StatusAccepted)
		},
	)

	body := bytes.NewBufferString(`{"volume": 0.5}`)
	req := httptest.NewRequest("POST", "/test", body)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status code = %d, want 202", w.Code)
	}
}
