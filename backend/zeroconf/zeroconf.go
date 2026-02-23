package zeroconf

import (
	"context"
	"net"
	"os"
	"sync"

	"github.com/grandcat/zeroconf"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

type ZeroConfBackend struct {
	Config *config.ZeroConfig

	server *zeroconf.Server
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.Mutex
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
		Config: cfg,
		ctx:    subCtx,
		cancel: cancel,
	}, nil
}

func ipv4sFromInterfaces(ifaces []net.Interface) []string {
	var ips []string
	for _, iface := range ifaces {
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok {
				if v4 := ipnet.IP.To4(); v4 != nil {
					ips = append(ips, v4.String())
				}
			}
		}
	}
	return ips
}

func (z *ZeroConfBackend) Start() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		return nil
	}

	hostname, _ := os.Hostname()
	ips := ipv4sFromInterfaces(z.Config.Listen)
	server, err := zeroconf.RegisterProxy(
		z.Config.InstanceName,
		z.Config.ServiceType,
		z.Config.Domain,
		z.Config.Port,
		hostname+".",
		ips,
		z.Config.TxtRecords,
		z.Config.Listen,
	)
	if err != nil {
		return err
	}

	z.server = server
	logger.Debug("[zeroconf] '%s' published on local network (type: %s, port: %d, iface: %v)",
		z.Config.InstanceName, z.Config.ServiceType, z.Config.Port, z.Config.Listen)

	go func() {
		<-z.ctx.Done()
		z.Close()
	}()

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
