package api

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/events"
)

// TestSSEHandler_ContentType verifies GET /events returns 200 with text/event-stream.
func TestSSEHandler_ContentType(t *testing.T) {
	upstream := make(chan events.Event)
	b := backend.NewBroadcaster(context.Background(), upstream)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	// Use a cancellable context so the handler exits after we've checked headers.
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	// Run the handler in the background and cancel quickly.
	done := make(chan struct{})
	go func() {
		defer close(done)
		sseHandler(b)(w, req)
	}()

	// Give the handler a moment to write headers and the initial comment.
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

// TestSSEHandler_ConnectedComment verifies the initial ": connected" comment is sent.
func TestSSEHandler_ConnectedComment(t *testing.T) {
	upstream := make(chan events.Event)
	b := backend.NewBroadcaster(context.Background(), upstream)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sseHandler(b)(w, req)
	}()

	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if !strings.Contains(body, ": connected") {
		t.Errorf("expected initial ': connected' comment in body, got: %q", body)
	}
}

// TestParseFilter_NoParams returns nil (pass-all) when no query params are given.
func TestParseFilter_NoParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	if parseFilter(req) != nil {
		t.Error("parseFilter with no params should return nil (pass-all)")
	}
}

// TestParseFilter_TypesParam verifies ?types= builds a type-based filter.
func TestParseFilter_TypesParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/events?types=player.updated,player.added", nil)
	f := parseFilter(req)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if !f(events.Event{Type: events.TypePlayerUpdated}) {
		t.Errorf("filter should pass %s", events.TypePlayerUpdated)
	}
	if !f(events.Event{Type: events.TypePlayerAdded}) {
		t.Errorf("filter should pass %s", events.TypePlayerAdded)
	}
	if f(events.Event{Type: events.TypeAudioUpdated}) {
		t.Errorf("filter should block %s", events.TypeAudioUpdated)
	}
}

// TestParseFilter_BackendParam verifies ?backend= expands to the backend's types.
func TestParseFilter_BackendParam(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/events?backend=audio", nil)
	f := parseFilter(req)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if !f(events.Event{Type: events.TypeAudioUpdated}) {
		t.Errorf("filter should pass %s", events.TypeAudioUpdated)
	}
	if f(events.Event{Type: events.TypePlayerUpdated}) {
		t.Errorf("filter should block %s", events.TypePlayerUpdated)
	}
}

// TestParseFilter_BothParams verifies ?types= and ?backend= are merged (union).
func TestParseFilter_BothParams(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/events?types=service.updated&backend=audio", nil)
	f := parseFilter(req)
	if f == nil {
		t.Fatal("expected non-nil filter")
	}
	if !f(events.Event{Type: events.TypeServiceUpdated}) {
		t.Errorf("filter should pass %s (from types param)", events.TypeServiceUpdated)
	}
	if !f(events.Event{Type: events.TypeAudioUpdated}) {
		t.Errorf("filter should pass %s (from backend param)", events.TypeAudioUpdated)
	}
	if f(events.Event{Type: events.TypePlayerUpdated}) {
		t.Errorf("filter should block %s", events.TypePlayerUpdated)
	}
}

// TestSSEHandler_FilteredDelivery verifies that events not matching ?types= are not sent.
func TestSSEHandler_FilteredDelivery(t *testing.T) {
	upstream := make(chan events.Event, 4)
	b := backend.NewBroadcaster(context.Background(), upstream)

	req := httptest.NewRequest(http.MethodGet, "/events?types=audio.updated", nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	done := make(chan struct{})
	go func() {
		defer close(done)
		sseHandler(b)(w, req)
	}()

	time.Sleep(20 * time.Millisecond)

	// Push a non-matching event (should be filtered) and a matching one.
	upstream <- events.Event{Type: events.TypePlayerUpdated, Data: "player"}
	upstream <- events.Event{Type: events.TypeAudioUpdated, Data: "audio"}

	time.Sleep(30 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	if strings.Contains(body, "player.updated") {
		t.Error("player.updated should not appear when filter is audio.updated only")
	}
	if !strings.Contains(body, "audio.updated") {
		t.Errorf("audio.updated should appear in filtered SSE body, got: %q", body)
	}
}

// TestSSEHandler_EventDelivery verifies that an event pushed to the broadcaster
// appears in the SSE response body.
func TestSSEHandler_EventDelivery(t *testing.T) {
	upstream := make(chan events.Event, 1)
	b := backend.NewBroadcaster(context.Background(), upstream)

	req := httptest.NewRequest(http.MethodGet, "/events", nil)
	ctx, cancel := context.WithCancel(context.Background())
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		defer close(done)
		sseHandler(b)(w, req)
	}()

	// Wait for the handler to subscribe and write the initial comment.
	time.Sleep(20 * time.Millisecond)

	// Push an event.
	upstream <- events.Event{Type: events.TypePlayerUpdated, Data: map[string]string{"bus_name": "org.mpris.MediaPlayer2.test"}}

	// Wait for it to be written.
	time.Sleep(20 * time.Millisecond)
	cancel()
	<-done

	body := w.Body.String()
	scanner := bufio.NewScanner(strings.NewReader(body))
	foundEvent := false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "event: "+events.TypePlayerUpdated) {
			foundEvent = true
			break
		}
	}
	if !foundEvent {
		t.Errorf("expected 'event: %s' line in SSE body, got: %q", events.TypePlayerUpdated, body)
	}
}
