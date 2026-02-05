package pulseaudio

import (
	"context"

	"github.com/the-jonsey/pulseaudio"

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

			// A sink input changed, refresh the cache
			logger.Debug("[pulseaudio] sink inputs changed, refreshing cache")
			if _, err := l.backend.refreshCache(); err != nil {
				logger.Warn("[pulseaudio] failed to refresh clients: %v", err)
			}
		}
	}
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[pulseaudio] stopping listener")

	// Cancel the context to stop the goroutine
	l.cancel()
}
