package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// sseHandler returns an http.HandlerFunc that streams SSE events to clients.
func sseHandler(b *backend.Broadcaster) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		filter, err := parseFilter(r)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		w.Header().Set("X-Accel-Buffering", "no")

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "streaming unsupported", http.StatusInternalServerError)
			return
		}

		if err := sendServerInfoToFlusher(flusher, w, "connected"); err != nil {
			return
		}

		ch := b.SubscribeFunc(filter)
		defer b.Unsubscribe(ch)
		keepAlive := time.NewTimer(30 * time.Second)
		defer keepAlive.Stop()

		for {
			select {
			case <-r.Context().Done():
				if err := sendServerInfoToFlusher(flusher, w, "bye"); err != nil {
					logger.Warn("[sse] failed to close events connection: %v", err)
				}
				return
			case <-keepAlive.C:
				if err := sendServerInfoToFlusher(flusher, w, "love"); err != nil {
					logger.Warn("[sse] failed to send keepalive, closing: %v", err)
					return
				}
				keepAlive.Reset(30 * time.Second)
			case e, ok := <-ch:
				if !ok {
					return
				}
				if err := sendToFlusher(flusher, w, e); err != nil {
					return
				}
				keepAlive.Reset(30 * time.Second)
			}
		}
	}
}

func sendServerInfoToFlusher(flusher http.Flusher, w http.ResponseWriter, message string) error {
	return sendToFlusher(
		flusher,
		w,
		events.Event{Type: events.TypeServerInfo, Data: message},
	)
}

func sendToFlusher(flusher http.Flusher, w http.ResponseWriter, e events.Event) error {
	data, err := json.Marshal(e.Data)
	if err != nil {
		logger.Warn("[sse] failed to marshal event data: %v", err)
		return err
	}
	if _, err = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", e.Type, data); err != nil {
		logger.Error("[sse] failed to write to flusher: %v", err)
		http.Error(w, "failed to send data to flusher", http.StatusInternalServerError)
		return err
	}
	flusher.Flush()
	return nil
}

// parseFilter builds an event filter from the request's query parameters:
//   - ?types=player.updated,player.added  — comma-separated event type names to include
//   - ?backend=mpris,audio               — comma-separated backend names to include (resolved via events.BackendTypes)
//   - ?exclude=player.position           — comma-separated event type names to exclude
//
// server.info is always included when include filters are specified.
// Returns an error if server.info is in the exclude list.
func parseFilter(r *http.Request) (func(events.Event) bool, error) {
	q := r.URL.Query()

	var include []string
	for _, t := range strings.Split(q.Get("types"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			include = append(include, t)
		}
	}
	for _, name := range strings.Split(q.Get("backend"), ",") {
		include = append(include, events.BackendTypes[strings.TrimSpace(name)]...)
	}
	if len(include) > 0 && !slices.Contains(include, events.TypeServerInfo) {
		include = append(include, events.TypeServerInfo)
	}

	var exclude []string
	for _, t := range strings.Split(q.Get("exclude"), ",") {
		if t = strings.TrimSpace(t); t != "" {
			if t == events.TypeServerInfo {
				return nil, errors.New("server.info cannot be excluded")
			}
			exclude = append(exclude, t)
		}
	}

	return events.NewFilter(include, exclude), nil
}
