package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
)

// TestAudioHandler tests the GET /audio combined endpoint logic.
// Since AudioHandler depends on a concrete *PulseAudioBackend, we replicate
// the handler logic with test data to verify JSON shape and cache header selection.
func TestAudioHandler(t *testing.T) {
	clients := []pulseaudio.AudioClient{
		{ID: 1, Name: "Firefox", App: "firefox", Volume: 0.8},
		{ID: 2, Name: "Spotify", App: "spotify", Volume: 0.5, Muted: true},
	}
	outputs := []pulseaudio.AudioOutput{
		{Index: 0, Name: "speakers", Description: "Built-in Speakers", Volume: 1.0, Default: true, State: "running"},
	}

	t.Run("returns clients and outputs as JSON", func(t *testing.T) {
		handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return map[string]any{
				"clients": clients,
				"outputs": outputs,
			}, nil
		})

		req := httptest.NewRequest("GET", "/audio", nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", w.Code)
		}
		if ct := w.Header().Get("Content-Type"); ct != "application/json" {
			t.Fatalf("Content-Type = %s, want application/json", ct)
		}

		var result map[string]json.RawMessage
		if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if _, ok := result["clients"]; !ok {
			t.Error("response missing 'clients' key")
		}
		if _, ok := result["outputs"]; !ok {
			t.Error("response missing 'outputs' key")
		}

		var gotClients []pulseaudio.AudioClient
		if err := json.Unmarshal(result["clients"], &gotClients); err != nil {
			t.Fatalf("failed to unmarshal clients: %v", err)
		}
		if len(gotClients) != 2 {
			t.Errorf("clients count = %d, want 2", len(gotClients))
		}

		var gotOutputs []pulseaudio.AudioOutput
		if err := json.Unmarshal(result["outputs"], &gotOutputs); err != nil {
			t.Fatalf("failed to unmarshal outputs: %v", err)
		}
		if len(gotOutputs) != 1 {
			t.Errorf("outputs count = %d, want 1", len(gotOutputs))
		}
	})

	t.Run("returns error on failure", func(t *testing.T) {
		handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
			return nil, http.ErrServerClosed
		})

		req := httptest.NewRequest("GET", "/audio", nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("status = %d, want 500", w.Code)
		}
	})
}

// TestAudioHandlerCacheHeader tests that the cache header uses the most recent timestamp.
func TestAudioHandlerCacheHeader(t *testing.T) {
	older := time.Date(2025, 1, 1, 10, 0, 0, 0, time.UTC)
	newer := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		clientsUpdated time.Time
		outputsUpdated time.Time
		wantHeader     string
	}{
		{
			name:           "outputs newer than clients",
			clientsUpdated: older,
			outputsUpdated: newer,
			wantHeader:     newer.UTC().Format(time.RFC3339),
		},
		{
			name:           "clients newer than outputs",
			clientsUpdated: newer,
			outputsUpdated: older,
			wantHeader:     newer.UTC().Format(time.RFC3339),
		},
		{
			name:           "same timestamps uses clients",
			clientsUpdated: newer,
			outputsUpdated: newer,
			wantHeader:     newer.UTC().Format(time.RFC3339),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := JSONHandler(func(w http.ResponseWriter, r *http.Request) (any, error) {
				if tt.outputsUpdated.After(tt.clientsUpdated) {
					setCacheHeader(w, tt.outputsUpdated)
				} else {
					setCacheHeader(w, tt.clientsUpdated)
				}
				return map[string]any{"clients": []any{}, "outputs": []any{}}, nil
			})

			req := httptest.NewRequest("GET", "/audio", nil)
			w := httptest.NewRecorder()
			handler(w, req)

			got := w.Header().Get("X-Cache-Updated-At")
			if got != tt.wantHeader {
				t.Errorf("X-Cache-Updated-At = %q, want %q", got, tt.wantHeader)
			}
		})
	}
}

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
