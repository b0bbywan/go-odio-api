package pulseaudio

import (
	"context"
	"log"

	"github.com/the-jonsey/pulseaudio"
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

	log.Println("PulseAudio listener started")
	return nil
}

func (l *Listener) listen(updates <-chan struct{}) {
	for {
		select {
		case <-l.ctx.Done():
			return

		case _, ok := <-updates:
			if !ok {
				return
			}

			// Un sink input a changé, recharger le cache
			log.Println("Sink inputs changed, refreshing cache")
			if _, err := l.backend.refreshCache(); err != nil {
				log.Printf("Failed to refresh clients: %v", err)
			}
		}
	}
}

// Stop arrête le listener
func (l *Listener) Stop() {
	log.Println("Stopping pulseaudio listener")

	// Cancel le context pour arrêter la goroutine
	l.cancel()
}
