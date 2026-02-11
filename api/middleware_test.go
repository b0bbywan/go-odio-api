package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWithBody tests the JSON body parsing and validation middleware
func TestWithBody(t *testing.T) {
	type testRequest struct {
		Value int `json:"value"`
	}

	tests := []struct {
		name           string
		body           string
		validate       func(*testRequest) error
		wantStatusCode int
		wantBodyMatch  string
		wantCalls      int
		wantValue      int
	}{
		{
			name: "valid JSON without validation passes through",
			body: `{"value": 42}`,
			validate: func(req *testRequest) error {
				return nil
			},
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
			wantValue:      42,
		},
		{
			name: "valid JSON with nil validation passes through",
			body: `{"value": 99}`,
			validate: func(req *testRequest) error {
				return nil
			},
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
			wantValue:      99,
		},
		{
			name:           "invalid JSON returns 400 Bad Request",
			body:           `{invalid json}`,
			validate:       nil,
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "invalid JSON payload",
			wantCalls:      0,
		},
		{
			name: "validation error returns 400 Bad Request",
			body: `{"value": -1}`,
			validate: func(req *testRequest) error {
				if req.Value < 0 {
					return http.ErrAbortHandler
				}
				return nil
			},
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "http: abort",
			wantCalls:      0,
		},
		{
			name:           "empty body returns 400 Bad Request",
			body:           ``,
			validate:       nil,
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "invalid JSON payload",
			wantCalls:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var receivedValue int

			nextFunc := func(w http.ResponseWriter, r *http.Request, req *testRequest) {
				calls++
				receivedValue = req.Value
				w.WriteHeader(http.StatusOK)
			}

			handler := withBody(tt.validate, nextFunc)

			req := httptest.NewRequest("POST", "/test", bytes.NewBufferString(tt.body))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}

			if tt.wantCalls > 0 && receivedValue != tt.wantValue {
				t.Errorf("value = %d, want %d", receivedValue, tt.wantValue)
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

// TestWithBodyVolume tests withBody specifically with volume validation
func TestWithBodyVolume(t *testing.T) {
	tests := []struct {
		name           string
		volume         float32
		wantStatusCode int
		wantBodyMatch  string
		wantCalls      int
	}{
		{
			name:           "valid volume 0.5 passes",
			volume:         0.5,
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
		},
		{
			name:           "valid volume 0 passes",
			volume:         0.0,
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
		},
		{
			name:           "valid volume 1 passes",
			volume:         1.0,
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
		},
		{
			name:           "volume > 1 fails validation",
			volume:         1.5,
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "volume must be between 0 and 1",
			wantCalls:      0,
		},
		{
			name:           "volume < 0 fails validation",
			volume:         -0.1,
			wantStatusCode: http.StatusBadRequest,
			wantBodyMatch:  "volume must be between 0 and 1",
			wantCalls:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0

			nextFunc := func(w http.ResponseWriter, r *http.Request, req *setVolumeRequest) {
				calls++
				w.WriteHeader(http.StatusOK)
			}

			handler := withBody(validateVolume, nextFunc)

			body := map[string]float32{"volume": tt.volume}
			bodyBytes, err := json.Marshal(body)
			if err != nil {
				t.Fatalf("failed to marshal body: %v", err)
			}

			req := httptest.NewRequest("POST", "/test", bytes.NewBuffer(bodyBytes))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}

			if tt.wantBodyMatch != "" {
				respBody := w.Body.String()
				if !strings.Contains(respBody, tt.wantBodyMatch) {
					t.Errorf("body = %q, want to contain %q", respBody, tt.wantBodyMatch)
				}
			}
		})
	}
}

// TestValidateVolume tests the volume validation function directly
func TestValidateVolume(t *testing.T) {
	tests := []struct {
		name    string
		volume  float32
		wantErr bool
		errMsg  string
	}{
		{
			name:    "volume 0.5 is valid",
			volume:  0.5,
			wantErr: false,
		},
		{
			name:    "volume 0 is valid",
			volume:  0.0,
			wantErr: false,
		},
		{
			name:    "volume 1 is valid",
			volume:  1.0,
			wantErr: false,
		},
		{
			name:    "volume 1.1 is invalid",
			volume:  1.1,
			wantErr: true,
			errMsg:  "volume must be between 0 and 1",
		},
		{
			name:    "volume -0.1 is invalid",
			volume:  -0.1,
			wantErr: true,
			errMsg:  "volume must be between 0 and 1",
		},
		{
			name:    "volume 2.0 is invalid",
			volume:  2.0,
			wantErr: true,
			errMsg:  "volume must be between 0 and 1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := &setVolumeRequest{Volume: tt.volume}
			err := validateVolume(req)

			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				} else if err.Error() != tt.errMsg {
					t.Errorf("error = %q, want %q", err.Error(), tt.errMsg)
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
			}
		})
	}
}
