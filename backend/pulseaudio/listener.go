package pulseaudio

import (
	"context"

	"github.com/the-jonsey/pulseaudio"

	"github.com/b0bbywan/go-odio-api/logger"
)

// Listener écoute les changements pulseaudio
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

// Start démarre l'écoute des événements pulseaudio
func (l *Listener) Start() error {
	// Subscribe aux changements de sink inputs
	updates, err := l.backend.client.UpdatesByType(pulseaudio.SUBSCRIPTION_MASK_SINK_INPUT)
	if err != nil {
		return err
	}

	// Goroutine d'écoute
	go l.listen(updates)

	logger.Info("PulseAudio listener started")
	return nil
}

func (l *Listener) listen(updates <-chan struct{}) {
	for {
		select {
		case <-l.ctx.Done():
			logger.Debug("PulseAudio listener context done")
			return

		case data, ok := <-updates:
			if !ok {
				logger.Debug("PulseAudio updates channel closed")
				return
			}

			// Un sink input a changé, recharger le cache
			logger.Debug("Sink inputs changed event received, refreshing cache: %v", data)
			if _, err := l.backend.refreshCache(); err != nil {
				logger.Warn("Failed to refresh clients: %v", err)
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	logger.Info("Stopping pulseaudio listener")

	// Cancel le context pour arrêter la goroutine
	l.cancel()
}
