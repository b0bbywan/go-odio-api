package systemd

import (
	"context"
	"log"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/coreos/go-systemd/v22/dbus"
)

type UnitScope string

const (
	ScopeSystem UnitScope = "system"
	ScopeUser   UnitScope = "user"
)

type SystemdBackend struct {
	sysConn      *dbus.Conn
	userConn     *dbus.Conn
	ctx          context.Context
	serviceNames []string

	// cache permanent (pas d'expiration)
	cache *cache.Cache[[]Service]

	// listener pour les changements systemd
	listener *Listener
}

type Service struct {
	Name        string 		`json:"name"`
	Scope       UnitScope 	`json:"scope"`
	ActiveState string 		`json:"active_state,omitempty"`
	Running     bool   		`json:"running"`
	Enabled     bool 		`json:"enabled"`
	Exists      bool      	`json:"exists"`
	Description string 		`json:"description,omitempty"`
}

func New(ctx context.Context, serviceNames []string) (*SystemdBackend, error) {
	sysC, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	userC, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}

	backend := &SystemdBackend{
		sysConn:      sysC,
		userConn:     userC,
		ctx:          ctx,
		serviceNames: serviceNames,
		cache:        cache.New[[]Service](0), // TTL=0 = pas d'expiration
	}

	// Charger le cache au démarrage
	if _, err := backend.ListServices(); err != nil {
		return nil, err
	}

	// Démarrer le listener pour les changements systemd
	backend.listener = NewListener(backend)
	if err := backend.listener.Start(); err != nil {
		return nil, err
	}

	return backend, nil
}

const cacheKey = "services"

func (s *SystemdBackend) ListServices() ([]Service, error) {
	// Vérifier le cache
	if cached, ok := s.cache.Get(cacheKey); ok {
		return cached, nil
	}

	// Charger depuis systemd
	out := make([]Service, 0, len(s.serviceNames)*2)

	sysSvcs, err := s.listServices(s.ctx, s.sysConn, ScopeSystem, s.serviceNames)
	if err != nil {
		log.Printf("Warning: failed to list system services: %v", err)
	}
	userSvcs, err := s.listServices(s.ctx, s.userConn, ScopeUser, s.serviceNames)
	if err != nil {
		log.Printf("Warning: failed to list user services: %v", err)
	}

	out = append(out, sysSvcs...)
	out = append(out, userSvcs...)

	// Mettre en cache
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

	svc := Service{
		Name:  name,
		Scope: scope,
	}

	props, err := conn.GetUnitPropertiesContext(s.ctx, name)
	if err != nil || props["UnitFileState"] == nil || props["UnitFileState"] == "" {
		svc.Exists = false
		svc.Enabled = false
	} else {
		svc.Exists = true
		svc.Enabled = props["UnitFileState"] == "enabled"
		svc.ActiveState, _ = props["ActiveState"].(string)
		subState, _ := props["SubState"].(string)
		svc.Running = svc.ActiveState == "active" && subState == "running"
		if desc, ok := props["Description"].(string); ok {
			svc.Description = desc
		}
	}

	// Mettre à jour dans le cache
	if err := s.UpdateService(svc); err != nil {
		return nil, err
	}

	return &svc, nil
}

func (s *SystemdBackend) listServices(ctx context.Context, conn *dbus.Conn, scope UnitScope, names []string) ([]Service, error) {
	services := make([]Service, 0, len(names))

	for _, name := range names {
		svc := Service{
			Name:  name,
			Scope: scope,
		}

		props, err := conn.GetUnitPropertiesContext(ctx, name)
		if err != nil || props["UnitFileState"] == nil || props["UnitFileState"] == "" {
			// unit inexistante
			svc.Exists = false
			svc.Enabled = false
			services = append(services, svc)
			continue
		}

		svc.Exists = true
		svc.Enabled = props["UnitFileState"] == "enabled"
		svc.ActiveState, _ = props["ActiveState"].(string)
		subState, _ := props["SubState"].(string)
		svc.Running = svc.ActiveState == "active" && subState == "running"

		if desc, ok := props["Description"].(string); ok {
			svc.Description = desc
		}

		services = append(services, svc)
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
		log.Printf("Warning: failed to refresh service %q in cache: %v", name, err)
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
		log.Printf("Warning: failed to refresh service %q in cache: %v", name, err)
	}
	return nil
}

func (s *SystemdBackend) RestartService(name string, scope UnitScope) error {
	if err := restartUnit(s.ctx, s.connForScope(scope), name); err != nil {
		return err
	}

	// Rafraîchir uniquement ce service dans le cache
	if _, err := s.RefreshService(name, scope); err != nil {
		log.Printf("Warning: failed to refresh service %q in cache: %v", name, err)
	}
	return nil
}

func startUnit(ctx context.Context, conn *dbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.StartUnitContext(ctx, name, "replace", ch)
	})
}

func stopUnit(ctx context.Context, conn *dbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.StopUnitContext(ctx, name, "replace", ch)
	})
}

func restartUnit(ctx context.Context, conn *dbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.RestartUnitContext(ctx, name, "replace", ch)
	})
}

func doUnitJob(
	ctx context.Context,
	f func(chan<- string) (int, error),
) error {
	ch := make(chan string, 1)

	if _, err := f(ch); err != nil {
		return err
	}

	<-ch
	return nil
}

func ParseUnitScope(v string) (UnitScope, bool) {
	switch UnitScope(v) {
	case ScopeSystem, ScopeUser:
		return UnitScope(v), true
	default:
		return "", false
	}
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
	}
	if s.sysConn != nil {
		s.sysConn.Close()
	}
	if s.userConn != nil {
		s.userConn.Close()
	}
}
