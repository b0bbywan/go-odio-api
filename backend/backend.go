package backend

import (
	"context"

	"github.com/b0bbywan/go-odio-api/backend/login1"
	"github.com/b0bbywan/go-odio-api/backend/mpris"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/backend/zeroconf"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

type Backend struct {
	Login1   *login1.Login1Backend
	MPRIS    *mpris.MPRISBackend
	Pulse    *pulseaudio.PulseAudioBackend
	Systemd  *systemd.SystemdBackend
	Zeroconf *zeroconf.ZeroConfBackend
}

func New(
	ctx context.Context,
	login1cfg *config.Login1Config,
	mpriscfg *config.MPRISConfig,
	pulscfg *config.PulseAudioConfig,
	syscfg *config.SystemdConfig,
	zerocfg *config.ZeroConfig,
) (*Backend, error) {
	var b Backend
	var err error

	if b.Login1, err = login1.New(ctx, login1cfg); err != nil {
		return nil, err
	}

	if b.MPRIS, err = mpris.New(ctx, mpriscfg); err != nil {
		return nil, err
	}

	if b.Pulse, err = pulseaudio.New(ctx, pulscfg); err != nil {
		return nil, err
	}

	if b.Systemd, err = systemd.New(ctx, syscfg); err != nil {
		return nil, err
	}

	if b.Zeroconf, err = zeroconf.New(ctx, zerocfg); err != nil {
		return nil, err
	}

	return &b, nil
}

func (b *Backend) Start() error {
	if b.MPRIS != nil {
		if err := b.MPRIS.Start(); err != nil {
			return err
		}
	}

	if b.Pulse != nil {
		if err := b.Pulse.Start(); err != nil {
			return err
		}
	}

	if b.Systemd != nil {
		if err := b.Systemd.Start(); err != nil {
			return err
		}
	}

	if b.Zeroconf != nil {
		if err := b.Zeroconf.Start(); err != nil {
			return err
		}
	}

	return nil
}

// NewBroadcaster merges all enabled backend event channels and returns a
// Broadcaster ready to fan out to SSE clients.
func (b *Backend) NewBroadcaster(ctx context.Context) *Broadcaster {
	var srcs []<-chan events.Event
	if b.MPRIS != nil {
		srcs = append(srcs, b.MPRIS.Events())
	}
	if b.Pulse != nil {
		srcs = append(srcs, b.Pulse.Events())
	}
	if b.Systemd != nil {
		srcs = append(srcs, b.Systemd.Events())
	}
	return NewBroadcaster(ctx, fanIn(ctx, srcs...))
}

func (b *Backend) Close() {
	if b.Login1 != nil {
		b.Login1.Close()
	}

	if b.MPRIS != nil {
		b.MPRIS.Close()
	}
	if b.Pulse != nil {
		b.Pulse.Close()
	}
	if b.Systemd != nil {
		b.Systemd.Close()
	}
	if b.Zeroconf != nil {
		b.Zeroconf.Close()
	}
}
