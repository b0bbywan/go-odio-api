package backend

import (
	"context"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
)

type Backend struct {
	Pulse	*pulseaudio.PulseAudioBackend
	Systemd	*systemd.SystemdBackend
}

func New(ctx context.Context, config *config.SystemdConfig) (*Backend, error) {
	var backend Backend
	if p, err := pulseaudio.New(ctx); err != nil {
		return nil, err
	} else {
		backend.Pulse = p
	}

	if s, err := systemd.New(ctx, config); err != nil {
		return nil, err
	} else {
		backend.Systemd = s
	}

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
