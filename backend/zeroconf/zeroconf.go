package zeroconf

import (
	"context"
	"sync"
	"time"

	"github.com/grandcat/zeroconf"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

type server interface {
	Shutdown()
}

type ZeroConfBackend struct {
	Config *config.ZeroConfig

	server server
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex

	// register publishes the service; a failure is retried from retryDelay,
	// doubling up to retryMax, since the network may not be up yet at boot.
	register   func(cfg *config.ZeroConfig) (server, error)
	retryDelay time.Duration
	retryMax   time.Duration
}

func New(ctx context.Context, cfg *config.ZeroConfig) (*ZeroConfBackend, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if len(cfg.Listen) == 0 {
		logger.Debug("[zeroconf] no interface selected, zeroconf disabled")
		return nil, nil
	}

	subCtx, cancel := context.WithCancel(ctx)

	return &ZeroConfBackend{
		Config:     cfg,
		ctx:        subCtx,
		cancel:     cancel,
		register:   register,
		retryDelay: time.Second,
		retryMax:   30 * time.Second,
	}, nil
}

func register(cfg *config.ZeroConfig) (server, error) {
	s, err := zeroconf.Register(
		cfg.InstanceName,
		cfg.ServiceType,
		cfg.Domain,
		cfg.Port,
		cfg.TxtRecords,
		cfg.Listen,
	)
	if err != nil {
		return nil, err
	}
	return s, nil
}

func (z *ZeroConfBackend) Start() error {
	go func() {
		<-z.ctx.Done()
		z.Close()
	}()

	if err := z.publish(); err != nil {
		logger.Warn("[zeroconf] publish failed, retrying in background: %v", err)
		go z.retry()
	}

	return nil
}

func (z *ZeroConfBackend) retry() {
	delay := z.retryDelay
	for {
		select {
		case <-z.ctx.Done():
			return
		case <-time.After(delay):
		}

		err := z.publish()
		if err == nil {
			return
		}
		delay = min(delay*2, z.retryMax)
		logger.Debug("[zeroconf] publish failed, retry in %s: %v", delay, err)
	}
}

func (z *ZeroConfBackend) publish() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil || z.ctx.Err() != nil {
		return nil
	}

	s, err := z.register(z.Config)
	if err != nil {
		return err
	}

	z.server = s
	logger.Debug("[zeroconf] '%s' published on local network (type: %s, port: %d, iface: %v)",
		z.Config.InstanceName, z.Config.ServiceType, z.Config.Port, z.Config.Listen)

	return nil
}

func (z *ZeroConfBackend) Close() {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		z.server.Shutdown()
		z.server = nil
		logger.Debug("[zeroconf] '%s' unpublished", z.Config.InstanceName)
	}

	if z.cancel != nil {
		z.cancel()
		z.cancel = nil
	}
}
