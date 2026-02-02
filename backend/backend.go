package backend

import (
	"context"

	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

type Backend struct {
	Pulse   *pulseaudio.PulseAudioBackend
	Systemd *systemd.SystemdBackend
}

func New(ctx context.Context, services []string) (*Backend, error) {
	var backend Backend
	p, err := pulseaudio.New(ctx)
	if err != nil {
		return nil, err
	}
	backend.Pulse = p

	s, err := systemd.New(ctx, services)
	if err != nil {
		return nil, err
	}
	backend.Systemd = s

	return &backend, nil
}

func (b *Backend) Start() error {
	if b.Pulse != nil {
		if err := b.Pulse.Start(); err != nil {
			return err
		}
	}

	return nil
}

func (b *Backend) Close() {
	if b.Pulse != nil {
		b.Pulse.Close()
	}
	if b.Systemd != nil {
		b.Systemd.Close()
	}
}
