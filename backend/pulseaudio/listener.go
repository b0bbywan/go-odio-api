package pulseaudio

import (
	"context"

	"github.com/the-jonsey/pulseaudio"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// Listener listens for pulseaudio changes
type Listener struct {
	backend *PulseAudioBackend
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewListener(backend *PulseAudioBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)
	return &Listener{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts listening for pulseaudio events
func (l *Listener) Start() error {
	// Subscribe to sink input changes
	updates, err := l.backend.client.UpdatesByType(pulseaudio.SUBSCRIPTION_MASK_SINK_INPUT)
	if err != nil {
		return err
	}

	// Listening goroutine
	go l.listen(updates)

	logger.Info("[pulseaudio] listener started")
	return nil
}

func (l *Listener) listen(updates <-chan struct{}) {
	for {
		select {
		case <-l.ctx.Done():
			logger.Debug("[pulseaudio] listener context done")
			return

		case _, ok := <-updates:
			if !ok {
				logger.Debug("[pulseaudio] updates channel closed")
				return
			}

			logger.Debug("[pulseaudio] sink inputs changed, refreshing cache")
			old, err := l.backend.ListClients()
			if err != nil {
				logger.Warn("[pulseaudio] failed to get clients before refresh: %v", err)
				continue
			}

			clients, err := l.backend.refreshCache()
			if err != nil {
				logger.Warn("[pulseaudio] failed to refresh clients: %v", err)
				continue
			}

			changed, removed := diffClients(old, clients)
			if len(changed) > 0 {
				l.backend.notify(events.Event{Type: events.TypeAudioUpdated, Data: changed})
			}
			if len(removed) > 0 {
				l.backend.notify(events.Event{Type: events.TypeAudioRemoved, Data: removed})
			}
		}
	}
}

// diffClients returns clients that were added/modified and clients that were removed.
func diffClients(old, new []AudioClient) (changed []AudioClient, removed []AudioClient) {
	newByName := make(map[string]struct{}, len(new))
	for _, c := range new {
		newByName[c.Name] = struct{}{}
	}

	oldByName := make(map[string]AudioClient, len(old))
	for _, c := range old {
		oldByName[c.Name] = c
		if _, exists := newByName[c.Name]; !exists {
			removed = append(removed, c)
		}
	}

	for _, c := range new {
		o, exists := oldByName[c.Name]
		if !exists || clientChanged(o, c) {
			changed = append(changed, c)
		}
	}
	return
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[pulseaudio] stopping listener")

	// Cancel the context to stop the goroutine
	l.cancel()
}
