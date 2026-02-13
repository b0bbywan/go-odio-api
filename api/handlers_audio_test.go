package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestWithSink tests the middleware for extracting sink from path
func TestWithSink(t *testing.T) {
	tests := []struct {
		name           string
		sink           string
		wantStatusCode int
		wantBodyMatch  string
		wantCalls      int
	}{
		{
			name:           "valid sink is passed to next handler",
			sink:           "alsa_output.pci-0000_00_1f.3.analog-stereo",
			wantStatusCode: http.StatusOK,
			wantCalls:      1,
		},
		{
			name:           "empty sink returns 404 Not Found",
			sink:           "",
			wantStatusCode: http.StatusNotFound,
			wantBodyMatch:  "missing sink",
			wantCalls:      0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := 0
			var receivedSink string

			nextFunc := func(w http.ResponseWriter, r *http.Request, sink string) {
				calls++
				receivedSink = sink
				w.WriteHeader(http.StatusOK)
			}

			handler := withSink(nil, nextFunc)

			req := httptest.NewRequest("POST", "/audio/sinks/"+tt.sink+"/mute", nil)
			req.SetPathValue("sink", tt.sink)
			w := httptest.NewRecorder()

			handler(w, req)

			if w.Code != tt.wantStatusCode {
				t.Errorf("status = %d, want %d", w.Code, tt.wantStatusCode)
			}

			if calls != tt.wantCalls {
				t.Errorf("calls = %d, want %d", calls, tt.wantCalls)
			}

			if tt.wantCalls > 0 && receivedSink != tt.sink {
				t.Errorf("sink = %q, want %q", receivedSink, tt.sink)
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
