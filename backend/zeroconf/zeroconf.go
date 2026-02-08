package zeroconf

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/grandcat/zeroconf"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// ZeroConfBackend gère un service Zeroconf mDNS
type ZeroConfBackend struct {
	Config *config.ZeroConfig

	server *zeroconf.Server
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
}

// New crée un backend ZeroConf prêt à être publié
func New(ctx context.Context, cfg *config.ZeroConfig) (*ZeroConfBackend, error) {
	if !cfg.Enabled {
		return nil, nil
	}

	subCtx, cancel := context.WithCancel(ctx)

	return &ZeroConfBackend{
		Config: cfg,
		ctx:    subCtx,
		cancel: cancel,
	}, nil
}

// Start publie le service et lance la goroutine pour tenir le contexte
func (z *ZeroConfBackend) Start() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		return fmt.Errorf("service déjà démarré")
	}

	server, err := zeroconf.Register(
		z.Config.InstanceName,
		z.Config.ServiceType,
		z.Config.Domain,
		z.Config.Port,
		z.Config.TxtRecords,
		nil,
	)
	if err != nil {
		return err
	}

	z.server = server
	log.Printf("[discovery] Service '%s' publié sur le réseau local (type: %s, port: %d)\n",
		z.Config.InstanceName, z.Config.ServiceType, z.Config.Port)

	// Goroutine qui attend l'annulation du contexte
	go func() {
		<-z.ctx.Done()
		z.Shutdown()
	}()

	return nil
}

// Shutdown arrête proprement le service Zeroconf
func (z *ZeroConfBackend) Shutdown() {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		z.server.Shutdown()
		z.server = nil
		logger.Debug("[discovery] Service '%s' arrêté\n", z.Config.InstanceName)
	}

	if z.cancel != nil {
		z.cancel()
		z.cancel = nil
	}
}
