package ui

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/events"
)

func testAPIPort(t *testing.T, server *httptest.Server) int {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(u.Port())
	if err != nil {
		t.Fatal(err)
	}
	return port
}

func newTestHandler(b *backend.Broadcaster) *Handler {
	return &Handler{
		tmpl:        LoadTemplates(),
		client:      NewAPIClient(0), // port 0 — API calls will fail, but that's expected in tests
		broadcaster: b,
	}
}

func TestSSEEvents_ContentType(t *testing.T) {
	upstream := make(chan events.Event)
	b := backend.NewBroadcaster(context.Background(), upstream)
	h := newTestHandler(b)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ui/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.SSEEvents(w, req)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
	ct := resp.Header.Get("Content-Type")
	if ct != "text/event-stream" {
		t.Errorf("expected Content-Type text/event-stream, got %q", ct)
	}
}

func TestSSEEvents_PlayerPositionTriggersMPRIS(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := backend.NewBroadcaster(context.Background(), upstream)

	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("[]")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	apiServer := httptest.NewServer(apiMux)
	defer apiServer.Close()

	h := &Handler{
		tmpl:        LoadTemplates(),
		client:      NewAPIClient(testAPIPort(t, apiServer)),
		broadcaster: b,
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ui/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.SSEEvents(w, req)
	}()

	time.Sleep(20 * time.Millisecond)
	upstream <- events.Event{Type: events.TypePlayerPosition, Data: "pos"}
	time.Sleep(350 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, "event: section-mpris") {
		t.Errorf("expected section-mpris SSE event for player.position, got: %s", body)
	}
}

func TestSSEEvents_DebounceCoalesces(t *testing.T) {
	upstream := make(chan events.Event, 16)
	b := backend.NewBroadcaster(context.Background(), upstream)

	// Use a real API server to serve section data
	apiMux := http.NewServeMux()
	apiMux.HandleFunc("/players", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if _, err := w.Write([]byte("[]")); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	apiServer := httptest.NewServer(apiMux)
	defer apiServer.Close()

	apiPort := testAPIPort(t, apiServer)

	h := &Handler{
		tmpl:        LoadTemplates(),
		client:      NewAPIClient(apiPort),
		broadcaster: b,
	}

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/ui/events", nil).WithContext(ctx)
	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		h.SSEEvents(w, req)
	}()

	// Let the handler subscribe to the broadcaster
	time.Sleep(20 * time.Millisecond)

	// Send 5 rapid player events — should coalesce into 1 SSE event
	for i := 0; i < 5; i++ {
		upstream <- events.Event{Type: events.TypePlayerUpdated, Data: "test"}
	}

	// Wait for one debounce cycle
	time.Sleep(350 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	count := strings.Count(body, "event: section-mpris")
	if count != 1 {
		t.Errorf("expected 1 coalesced section-mpris event, got %d\nbody: %s", count, body)
	}
}

func TestSSEEvents_EventMapping(t *testing.T) {
	tests := []struct {
		name         string
		eventType    string
		apiEndpoint  string
		apiResponse  string
		wantSSEEvent string
	}{
		{
			name:         "player.updated maps to section-mpris",
			eventType:    events.TypePlayerUpdated,
			apiEndpoint:  "/players",
			apiResponse:  "[]",
			wantSSEEvent: "event: section-mpris",
		},
		{
			name:         "player.added maps to section-mpris",
			eventType:    events.TypePlayerAdded,
			apiEndpoint:  "/players",
			apiResponse:  "[]",
			wantSSEEvent: "event: section-mpris",
		},
		{
			name:         "player.removed maps to section-mpris",
			eventType:    events.TypePlayerRemoved,
			apiEndpoint:  "/players",
			apiResponse:  "[]",
			wantSSEEvent: "event: section-mpris",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			upstream := make(chan events.Event, 4)
			b := backend.NewBroadcaster(context.Background(), upstream)

			apiMux := http.NewServeMux()
			apiMux.HandleFunc(tt.apiEndpoint, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				if _, err := w.Write([]byte(tt.apiResponse)); err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
				}
			})
			apiServer := httptest.NewServer(apiMux)
			defer apiServer.Close()

			apiPort := testAPIPort(t, apiServer)

			h := &Handler{
				tmpl:        LoadTemplates(),
				client:      NewAPIClient(apiPort),
				broadcaster: b,
			}

			ctx, cancel := context.WithCancel(context.Background())
			req := httptest.NewRequest(http.MethodGet, "/ui/events", nil).WithContext(ctx)
			w := httptest.NewRecorder()

			done := make(chan struct{})
			go func() {
				defer close(done)
				h.SSEEvents(w, req)
			}()

			time.Sleep(20 * time.Millisecond)
			upstream <- events.Event{Type: tt.eventType, Data: "test"}
			time.Sleep(350 * time.Millisecond)
			cancel()
			<-done

			body := w.Body.String()
			if !strings.Contains(body, tt.wantSSEEvent) {
				t.Errorf("expected %q in response, got: %s", tt.wantSSEEvent, body)
			}

			// Verify it contains data lines (HTML)
			scanner := bufio.NewScanner(strings.NewReader(body))
			hasData := false
			for scanner.Scan() {
				if strings.HasPrefix(scanner.Text(), "data: ") {
					hasData = true
					break
				}
			}
			if !hasData {
				t.Errorf("expected data: lines in SSE response, got: %s", body)
			}
		})
	}
}
