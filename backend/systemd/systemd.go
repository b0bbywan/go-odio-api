package systemd

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New now takes the services list from the config
func New(ctx context.Context, config *config.SystemdConfig) (*SystemdBackend, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	sysC, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	userC, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}

	return &SystemdBackend{
		sysConn:  sysC,
		userConn: userC,
		ctx:      ctx,
		config:   config,
		cache:    cache.New[[]Service](0), // TTL=0 = no expiration
	}, nil
}

// Start loads the initial cache and starts the listener
func (s *SystemdBackend) Start() error {
	logger.Debug("[systemd] starting backend (headless=%v)", s.config.Headless)

	// Load the cache at startup
	if _, err := s.ListServices(); err != nil {
		return err
	}

	// Start the listener for systemd changes
	s.listener = NewListener(s)
	if err := s.listener.Start(); err != nil {
		return err
	}

	logger.Info("[systemd] backend started successfully")
	return nil
}

func (s *SystemdBackend) ListServices() ([]Service, error) {
	// Check the cache first
	if services, ok := s.cache.Get(cacheKey); ok {
		logger.Debug("[systemd] returning %d units from cache", len(services))
		return services, nil
	}

	// Cache miss, load from D-Bus
	logger.Debug("[systemd] cache miss, loading units")
	out := make([]Service, 0, len(s.config.SystemServices)+len(s.config.UserServices))
	start := time.Now()

	sysSvcs, err := s.listServices(s.ctx, s.sysConn, ScopeSystem, s.config.SystemServices)
	if err != nil {
		logger.Warn("[systemd] failed to list system services: %v", err)
	}
	userSvcs, err := s.listServices(s.ctx, s.userConn, ScopeUser, s.config.UserServices)
	if err != nil {
		logger.Warn("[systemd] failed to list user services: %v", err)
	}
	elapsed := time.Since(start)

	out = append(out, sysSvcs...)
	out = append(out, userSvcs...)
	logger.Debug("[systemd] loaded %d units in %s", len(out), elapsed)

	// Update the cache
	s.cache.Set(cacheKey, out)

	return out, nil
}

// GetService retrieves a specific service from the cache
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

// UpdateService updates a specific service in the cache
func (s *SystemdBackend) UpdateService(updated Service) error {
	services, ok := s.cache.Get(cacheKey)
	if !ok {
		// If no cache, reload everything
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
		// Service not in cache, add it
		services = append(services, updated)
	}

	s.cache.Set(cacheKey, services)
	return nil
}

// RefreshService reloads a specific service from systemd and updates the cache
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
				Name:        unit.Name,
				Scope:       scope,
				ActiveState: unit.ActiveState,
				Running:     unit.SubState == "running",
				Exists:      loaded,
			}
			enabled, err := conn.GetUnitPropertyContext(ctx, unit.Name, "UnitFileState")
			if err != nil {
				logger.Warn("[systemd] failed to get %s UnitFileState: %v", unit.Name, err)
			} else {
				svc.Enabled = enabled.Value.Value().(string) == "enabled"
			}
			description, err := conn.GetUnitPropertyContext(ctx, unit.Name, "Description")
			if err != nil {
				logger.Warn("[systemd] failed to get %s Description: %v", unit.Name, err)
			} else {
				svc.Description = description.Value.Value().(string)
			}

			services = append(services, svc)
		}
	}

	return services, nil
}

func (s *SystemdBackend) EnableService(name string, scope UnitScope) error {
	logger.Debug("[systemd] enabling service %s/%s", scope, name)
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

	logger.Debug("[systemd] service %s/%s enabled successfully", scope, name)
	return nil
}

func (s *SystemdBackend) DisableService(name string, scope UnitScope) error {
	logger.Debug("[systemd] disabling service %s/%s", scope, name)
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

	logger.Debug("[systemd] service %s/%s disabled successfully", scope, name)
	return nil
}

func (s *SystemdBackend) RestartService(name string, scope UnitScope) error {
	logger.Debug("[systemd] restarting service %s/%s", scope, name)
	if err := restartUnit(s.ctx, s.connForScope(scope), name); err != nil {
		return err
	}

	logger.Debug("[systemd] service %s/%s restarted successfully", scope, name)
	return nil
}

func (s *SystemdBackend) connForScope(scope UnitScope) *dbus.Conn {
	if scope == ScopeUser {
		return s.userConn
	}
	return s.sysConn
}

// invalidateCache invalidates the entire cache (used if need to reload everything)
func (s *SystemdBackend) invalidateCache() {
	s.cache.Delete(cacheKey)
}

// InvalidateCache is the public API to invalidate the cache if necessary
func (s *SystemdBackend) InvalidateCache() {
	s.invalidateCache()
}

// Close cleanly closes the connections and stops the listener
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
