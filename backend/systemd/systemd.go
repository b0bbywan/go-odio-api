package systemd

import (
	"context"
	"time"

	"github.com/coreos/go-systemd/v22/dbus"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// New now takes the services list from the config
func New(ctx context.Context, config *config.SystemdConfig) (*SystemdBackend, error) {
	if config == nil || !config.Enabled {
		return nil, nil
	}

	if len(config.SystemServices) == 0 && len(config.UserServices) == 0 {
		logger.Debug("[systemd] no unit configured, disabling backend")
		return nil, nil
	}

	var sysC, userC *dbus.Conn
	var err error
	if len(config.SystemServices) > 0 {
		sysC, err = dbus.NewSystemConnectionContext(ctx)
		if err != nil {
			return nil, err
		}
	}

	if len(config.UserServices) > 0 {
		userC, err = dbus.NewUserConnectionContext(ctx)
		if err != nil {
			return nil, err
		}
	}

	return &SystemdBackend{
		sysConn:  sysC,
		userConn: userC,
		ctx:      ctx,
		config:   config,
		cache:    cache.New[[]Service](0), // TTL=0 = no expiration
		events:   make(chan events.Event, 32),
	}, nil
}

// AddInternalUserUnits registers extra user units as internal: triggerable and
// state-tracked, but hidden from /services and service.updated events. Must be
// called before Start, since the listener snapshots the unit list then.
func (s *SystemdBackend) AddInternalUserUnits(names ...string) {
	for _, name := range names {
		if name == "" {
			continue
		}
		s.config.UserServices = append(s.config.UserServices, config.SystemdService{Name: name, Internal: true})
	}
}

// Start loads the initial cache and starts the listener
func (s *SystemdBackend) Start() error {
	logger.Debug("[systemd] starting backend (utmp=%v)", s.config.SupportsUTMP)

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
	close(s.events)
}

func (s *SystemdBackend) notify(e events.Event) {
	select {
	case s.events <- e:
	default:
		logger.Warn("[systemd] event channel full, dropping %s event", e.Type)
	}
}

// notifyService emits a service.updated event for every refreshed unit, internal
// or not, so other backends can react to its state over the event bus (e.g. the
// upgrade backend tracking its run unit). Internal units are kept out of the
// /services *listing* by PublicServices, not from the event stream.
func (s *SystemdBackend) notifyService(svc Service) {
	s.notify(events.Event{Type: events.TypeServiceUpdated, Data: svc})
}

// Events returns the read-only event channel for this backend.
func (s *SystemdBackend) Events() <-chan events.Event { return s.events }

func (b *SystemdBackend) canExecute(name string, scope UnitScope) error {
	switch scope {
	case ScopeSystem:
		return &PermissionSystemError{Unit: name}
	case ScopeUser:
		if !b.listener.userWatched[name] {
			return &PermissionUserError{Unit: name}
		}
	}
	return nil
}

// Execute runs a mutating action on a systemd unit.
//
// SECURITY: All mutating actions are intentionally executed using the *user*
// systemd D-Bus connection only. The system connection is strictly read-only
// in this backend and can not be used as is for start/stop/enable operations.
// This provides a structural safety guarantee, even in case of permission
// check regressions or future refactors.
func (s *SystemdBackend) Execute(
	ctx context.Context,
	name string,
	scope UnitScope,
	action unitActionFunc,
) error {
	if err := s.canExecute(name, scope); err != nil {
		return err
	}

	if err := action(ctx, s.userConn, name); err != nil {
		return err
	}

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

// PublicServices returns the configured services minus internal ones, for the
// public /services listing.
func (s *SystemdBackend) PublicServices() ([]Service, error) {
	all, err := s.ListServices()
	if err != nil {
		return nil, err
	}
	public := make([]Service, 0, len(all))
	for _, svc := range all {
		if !svc.Internal {
			public = append(public, svc)
		}
	}
	return public, nil
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
func (s *SystemdBackend) RefreshService(ctx context.Context, name string, scope UnitScope) (*Service, error) {
	conn := s.connForScope(scope)

	props, err := conn.GetUnitPropertiesContext(ctx, name)
	if err != nil {
		logger.Debug("[systemd] failed to get %s unit properties: %v", name, err)
		props = nil
	}

	svc := serviceFromProps(name, scope, props)
	// URL and Internal are config-derived, not D-Bus-derived, so serviceFromProps
	// can't know about them. Without this lookup, every refresh wipes them.
	svc.URL = s.configuredURL(name, scope)
	svc.Internal = s.configuredInternal(name, scope)

	if err := s.UpdateService(svc); err != nil {
		logger.Debug("[systemd] failed to update %s: %v", name, err)
		return nil, err
	}

	return &svc, nil
}

// configuredURL returns the URL declared in the config for this service, or
// "" if the service has no URL or isn't in the configured list.
func (s *SystemdBackend) configuredURL(name string, scope UnitScope) string {
	var configured []config.SystemdService
	switch scope {
	case ScopeSystem:
		configured = s.config.SystemServices
	case ScopeUser:
		configured = s.config.UserServices
	}
	for _, svc := range configured {
		if svc.Name == name {
			return svc.URL
		}
	}
	return ""
}

// configuredInternal reports whether the named unit was registered as internal.
func (s *SystemdBackend) configuredInternal(name string, scope UnitScope) bool {
	var configured []config.SystemdService
	switch scope {
	case ScopeSystem:
		configured = s.config.SystemServices
	case ScopeUser:
		configured = s.config.UserServices
	}
	for _, svc := range configured {
		if svc.Name == name {
			return svc.Internal
		}
	}
	return false
}

func (s *SystemdBackend) listServices(
	ctx context.Context,
	conn *dbus.Conn,
	scope UnitScope,
	configured []config.SystemdService,
) ([]Service, error) {
	if conn == nil {
		return nil, nil
	}
	names := make([]string, len(configured))
	urls := make(map[string]string, len(configured))
	internal := make(map[string]bool, len(configured))
	for i, svc := range configured {
		names[i] = svc.Name
		if svc.URL != "" {
			urls[svc.Name] = svc.URL
		}
		if svc.Internal {
			internal[svc.Name] = true
		}
	}
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
				URL:         urls[unit.Name],
				Internal:    internal[unit.Name],
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
	return s.Execute(s.ctx, name, scope, enableUnit)
}

func (s *SystemdBackend) DisableService(name string, scope UnitScope) error {
	logger.Debug("[systemd] disabling service %s/%s", scope, name)
	return s.Execute(s.ctx, name, scope, disableUnit)
}

func (s *SystemdBackend) StartService(name string, scope UnitScope) error {
	logger.Debug("[systemd] starting service %s/%s", scope, name)
	return s.Execute(s.ctx, name, scope, startUnit)
}

// TriggerUserUnit starts a user unit without waiting for completion; callers
// observe the run through service.updated events instead (see the upgrade
// backend). User scope only.
func (s *SystemdBackend) TriggerUserUnit(ctx context.Context, name string) error {
	if err := s.canExecute(name, ScopeUser); err != nil {
		return err
	}
	// nil channel: enqueue the start job and return without awaiting the result.
	_, err := s.userConn.StartUnitContext(ctx, name, "replace", nil)
	return err
}

func (s *SystemdBackend) StopService(name string, scope UnitScope) error {
	logger.Debug("[systemd] stopping service %s/%s", scope, name)
	return s.Execute(s.ctx, name, scope, stopUnit)
}

func (s *SystemdBackend) RestartService(name string, scope UnitScope) error {
	logger.Debug("[systemd] restarting service %s/%s", scope, name)
	return s.Execute(s.ctx, name, scope, restartUnit)
}

func (s *SystemdBackend) connForScope(scope UnitScope) *dbus.Conn {
	if scope == ScopeUser {
		return s.userConn
	}
	return s.sysConn
}

// CacheUpdatedAt returns the last time the service cache was written to.
func (s *SystemdBackend) CacheUpdatedAt() time.Time {
	return s.cache.UpdatedAt()
}

// invalidateCache invalidates the entire cache (used if need to reload everything)
func (s *SystemdBackend) invalidateCache() {
	s.cache.Delete(cacheKey)
}

// InvalidateCache is the public API to invalidate the cache if necessary
func (s *SystemdBackend) InvalidateCache() {
	s.invalidateCache()
}
