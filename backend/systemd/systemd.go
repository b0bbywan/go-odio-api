package systemd

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New prend maintenant la liste des services depuis la config
func New(ctx context.Context, config *config.SystemdConfig) (*SystemdBackend, error) {
	sysC, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	userC, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}

	return &SystemdBackend{
		sysConn:      sysC,
		userConn:     userC,
		ctx:          ctx,
		config:       config,
		cache:        cache.New[[]Service](0), // TTL=0 = pas d'expiration
	}, nil
}

// Start charge le cache initial et démarre le listener
func (s *SystemdBackend) Start() error {
	// Charger le cache au démarrage
	if _, err := s.ListServices(); err != nil {
		return err
	}

	// Démarrer le listener pour les changements systemd
	s.listener = NewListener(s)
	if err := s.listener.Start(); err != nil {
		return err
	}

	return nil
}

func (s *SystemdBackend) ListServices() ([]Service, error) {
	out := make([]Service, 0, len(s.config.SystemServices)+len(s.config.UserServices))
	start := time.Now()

	sysSvcs, err := s.listServices(s.ctx, s.sysConn, ScopeSystem, s.config.SystemServices)
	if err != nil {
		logger.Warn("failed to list system services: %v", err)
	}
	userSvcs, err := s.listServices(s.ctx, s.userConn, ScopeUser, s.config.UserServices)
	if err != nil {
		logger.Warn("failed to list user services: %v", err)
	}
	elapsed := time.Since(start)
	logger.Debug("units listed in %s", elapsed)

	out = append(out, sysSvcs...)
	out = append(out, userSvcs...)

	// Mettre à jour le cache
	s.cache.Set(cacheKey, out)

	return out, nil
}


// GetService récupère un service spécifique du cache
func (s *SystemdBackend) GetService(name string, scope UnitScope) (*Service, bool) {
	services, ok := s.cache.Get(cacheKey)
	if !ok {
		return nil, false
	}

	for _, svc := range services {
		if svc.Name == name && svc.Scope == scope {
			return &svc, true
		}
	}
	return nil, false
}

// UpdateService met à jour un service spécifique dans le cache
func (s *SystemdBackend) UpdateService(updated Service) error {
	services, ok := s.cache.Get(cacheKey)
	if !ok {
		// Si pas de cache, on recharge tout
		_, err := s.ListServices()
		return err
	}

	found := false
	for i, svc := range services {
		if svc.Name == updated.Name && svc.Scope == updated.Scope {
			services[i] = updated
			found = true
			break
		}
	}

	if !found {
		// Service pas dans le cache, on l'ajoute
		services = append(services, updated)
	}

	s.cache.Set(cacheKey, services)
	return nil
}

// RefreshService recharge un service spécifique depuis systemd et met à jour le cache
func (s *SystemdBackend) RefreshService(name string, scope UnitScope) (*Service, error) {
	conn := s.connForScope(scope)

	props, err := conn.GetUnitPropertiesContext(s.ctx, name)
	if err != nil {
		props = nil
	}

	svc := serviceFromProps(name, scope, props)

	if err := s.UpdateService(svc); err != nil {
		return nil, err
	}

	return &svc, nil
}

func (s *SystemdBackend) listServices(
	ctx context.Context,
	conn *dbus.Conn,
	scope UnitScope,
	names []string,
) ([]Service, error) {
	services := make([]Service, 0, len(names))
	units, err := conn.ListUnitsByNamesContext(ctx, names)
	if err != nil {
		return nil, err
	}

	for _, unit := range units {
		if loaded := unit.LoadState == "loaded"; loaded {
			svc := Service{
				Name:  unit.Name,
				Scope: scope,
				ActiveState: unit.ActiveState,
				Running: unit.SubState == "running",
				Exists: loaded,
			}
			enabled, err := conn.GetUnitPropertyContext(ctx, unit.Name, "UnitFileState")
			if err != nil {
				logger.Warn("failed to get %s state: %v", unit.Name, err)
			} else {
				svc.Enabled = enabled.Value.Value().(string) == "enabled"
			}
			description, err := conn.GetUnitPropertyContext(ctx, unit.Name, "UnitFileState")
			if err != nil {
				logger.Warn("failed to get %s description: %v", unit.Name, err)
			} else {
				svc.Description = description.Value.Value().(string)
			}

			services = append(services, svc)
		}
	}

	return services, nil
}

func (s *SystemdBackend) EnableService(name string, scope UnitScope) error {
	conn := s.connForScope(scope)

	_, _, err := conn.EnableUnitFilesContext(
		s.ctx,
		[]string{name},
		false, // runtime
		true,  // force
	)
	if err != nil {
		return err
	}

	if err = conn.ReloadContext(s.ctx); err != nil {
		return err
	}

	if err = startUnit(s.ctx, conn, name); err != nil {
		return err
	}

	// Rafraîchir uniquement ce service dans le cache
	if _, err := s.RefreshService(name, scope); err != nil {
		logger.Warn("failed to refresh service %q in cache: %v", name, err)
	}
	return nil
}

func (s *SystemdBackend) DisableService(name string, scope UnitScope) error {
	conn := s.connForScope(scope)

	if err := stopUnit(s.ctx, conn, name); err != nil {
		return err
	}

	if _, err := conn.DisableUnitFilesContext(
		s.ctx,
		[]string{name},
		false, // runtime
	); err != nil {
		return err
	}

	if err := conn.ReloadContext(s.ctx); err != nil {
		return err
	}

	// Rafraîchir uniquement ce service dans le cache
	if _, err := s.RefreshService(name, scope); err != nil {
		logger.Warn("failed to refresh service %q in cache: %v", name, err)
	}
	return nil
}

func (s *SystemdBackend) RestartService(name string, scope UnitScope) error {
	if err := restartUnit(s.ctx, s.connForScope(scope), name); err != nil {
		return err
	}

	// Rafraîchir uniquement ce service dans le cache
	if _, err := s.RefreshService(name, scope); err != nil {
		logger.Warn("failed to refresh service %q in cache: %v", name, err)
	}
	return nil
}

func (s *SystemdBackend) connForScope(scope UnitScope) *dbus.Conn {
	if scope == ScopeUser {
		return s.userConn
	}
	return s.sysConn
}

// invalidateCache invalide tout le cache (utilisé si besoin de recharger tout)
func (s *SystemdBackend) invalidateCache() {
	s.cache.Delete(cacheKey)
}

// InvalidateCache est l'API publique pour invalider le cache si nécessaire
func (s *SystemdBackend) InvalidateCache() {
	s.invalidateCache()
}

// Close ferme proprement les connexions et arrête le listener
func (s *SystemdBackend) Close() {
	if s.listener != nil {
		s.listener.Stop()
		s.listener = nil
	}
	if s.sysConn != nil {
		s.sysConn.Close()
		s.sysConn = nil
	}
	if s.userConn != nil {
		s.userConn.Close()
		s.userConn = nil
	}
}
