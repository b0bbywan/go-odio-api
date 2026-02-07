package backend

import (
	"context"

	"github.com/b0bbywan/go-odio-api/backend/bluetooth"
	"github.com/b0bbywan/go-odio-api/backend/mpris"
	"github.com/b0bbywan/go-odio-api/backend/pulseaudio"
	"github.com/b0bbywan/go-odio-api/backend/systemd"
	"github.com/b0bbywan/go-odio-api/config"
)

type Backend struct {
	Bluetooth *bluetooth.BluetoothBackend
	MPRIS     *mpris.MPRISBackend
	Pulse     *pulseaudio.PulseAudioBackend
	Systemd   *systemd.SystemdBackend
}

func New(
	ctx context.Context,
	btcfg *config.BluetoothConfig,
	mpriscfg *config.MPRISConfig,
	pulscfg *config.PulseAudioConfig,
	syscfg *config.SystemdConfig,
) (*Backend, error) {
	var b Backend
	var err error

	if b.Bluetooth, err = bluetooth.New(ctx, btcfg); err != nil {
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

	return &b, nil
}

func (b *Backend) Start() error {
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

	if b.MPRIS != nil {
		if err := b.MPRIS.Start(); err != nil {
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
	if b.MPRIS != nil {
		b.MPRIS.Close()
	}
	if b.Bluetooth != nil {
		b.Bluetooth.Close()
	}
}
