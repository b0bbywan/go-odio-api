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
