package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/b0bbywan/go-odio-api/backend"
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

		if err := sendServerInfoToFlusher(flusher, w, "connected"); err != nil {
			return
		}

		ch := b.Subscribe()
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
		return err
	}
	flusher.Flush()
	return nil
}
