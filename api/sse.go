package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// sseHandler returns an http.HandlerFunc that streams SSE events to clients.
func sseHandler(b *backend.Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		// Send initial keep-alive comment so the client knows the connection is live.
		if err := sendToFlusher(flusher, w, ": connected"); err != nil {
			return
		}

		ch := b.SubscribeFunc(parseFilter(r))
		defer b.Unsubscribe(ch)
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-r.Context().Done():
				if err := sendToFlusher(flusher, w, ": bye"); err != nil {
					return
				}
				return
			case <-ticker.C:
				if err := sendToFlusher(flusher, w, ": keep-alive"); err != nil {
					return
				}
			case e, ok := <-ch:
				if !ok {
					return
				}
				data, err := json.Marshal(e.Data)
				if err != nil {
					logger.Warn("[sse] failed to marshal event data: %v", err)
					continue
				}
				if err = sendToFlusher(flusher, w, fmt.Sprintf("event: %s\ndata: %s", e.Type, data)); err != nil {
					return
				}
			}
		}
	}
}

// parseFilter builds an event filter from the request's query parameters:
//   - ?types=player.updated,player.added  — comma-separated event type names
//   - ?backend=mpris,audio               — comma-separated backend names (resolved via events.BackendTypes)
//
// Types from both params are merged (union). A nil return means pass-all.
func parseFilter(r *http.Request) func(events.Event) bool {
	q := r.URL.Query()
	var allTypes []string

	if raw := q.Get("types"); raw != "" {
		for _, t := range strings.Split(raw, ",") {
			if t = strings.TrimSpace(t); t != "" {
				allTypes = append(allTypes, t)
			}
		}
	}

	if raw := q.Get("backend"); raw != "" {
		for _, name := range strings.Split(raw, ",") {
			name = strings.TrimSpace(name)
			if ts, ok := events.BackendTypes[name]; ok {
				allTypes = append(allTypes, ts...)
			}
		}
	}

	return events.FilterTypes(allTypes)
}

func sendToFlusher(flusher http.Flusher, w http.ResponseWriter, data string) error {
	if _, err := fmt.Fprintf(w, "%s\n\n", data); err != nil {
		logger.Error("[sse] failed to send data to flusher: %v", err)
		http.Error(w, "failed to send data to flusher", http.StatusInternalServerError)
		return err
	}
	flusher.Flush()
	return nil
}
