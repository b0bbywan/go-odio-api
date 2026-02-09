package zeroconf

import (
	"context"
	"fmt"
	"os"
	"net"
	"sync"

	"github.com/hashicorp/mdns"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// ZeroConfBackend gère un service mDNS
type ZeroConfBackend struct {
	Config *config.ZeroConfig

	server *mdns.Server
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

// Start publie le service mDNS
func (z *ZeroConfBackend) Start() error {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		return fmt.Errorf("service already started")
	}

	hostname, err := os.Hostname()
	if err != nil {
		logger.Debug("[backend] failed to get hostname: %v", err)
		hostname = "UNKNOWN"
	}

	// Détecte l'IP LAN de l'interface active
	hostIP := getLocalIPv4()
	if hostIP == "" {
		return fmt.Errorf("could not determine local LAN IP")
	}

	// Crée le service mDNS
	service, err := mdns.NewMDNSService(
		z.Config.InstanceName, // Nom de ton service
		z.Config.ServiceType,  // Type de service, ex: "_http._tcp"
		"local.",              // Domaine mDNS
		hostname + ".",              // IP LAN détectée
		z.Config.Port,         // Port
		nil,                   // Priorité / poids (optionnel)
		z.Config.TxtRecords,   // TXT records
	)
	if err != nil {
		return fmt.Errorf("failed to create mDNS service: %w", err)
	}

	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return fmt.Errorf("failed to start mDNS server: %w", err)
	}

	z.server = server

	logger.Info("[zeroconf] Service '%s' published on local network (type: %s, port: %d, IP: %s)\n",
		z.Config.InstanceName, z.Config.ServiceType, z.Config.Port, hostIP)

	// Goroutine qui attend l'annulation du contexte
	go func() {
		<-z.ctx.Done()
		z.Shutdown()
	}()

	return nil
}

// Shutdown arrête proprement le service mDNS
func (z *ZeroConfBackend) Shutdown() {
	z.mu.Lock()
	defer z.mu.Unlock()

	if z.server != nil {
		z.server.Shutdown()
		z.server = nil
		logger.Debug("[zeroconf] Service '%s' stopped\n", z.Config.InstanceName)
	}

	if z.cancel != nil {
		z.cancel()
		z.cancel = nil
	}
}

// getLocalIPv4 retourne la première IP IPv4 non-loopback
func getLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range ifaces {
		// Ignore interfaces désactivées ou loopback
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP.IsLoopback() {
				continue
			}
			if ipNet.IP.To4() != nil {
				return ipNet.IP.String()
			}
		}
	}
	return ""
}
