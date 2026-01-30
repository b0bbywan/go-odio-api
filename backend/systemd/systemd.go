package systemd

import (
	"context"

	"github.com/coreos/go-systemd/v22/dbus"
)

type UnitScope string

const (
	ScopeSystem UnitScope = "system"
	ScopeUser   UnitScope = "user"
)

type SystemdBackend struct {
	sysConn  *dbus.Conn
	userConn *dbus.Conn
	ctx      context.Context
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

func New(ctx context.Context) (*SystemdBackend, error) {
	sysC, err := dbus.NewSystemConnectionContext(ctx)
	if err != nil {
		return nil, err
	}
	userC, err := dbus.NewUserConnectionContext(ctx)
	if err != nil {
		return nil, err
	}

	return &SystemdBackend{sysConn: sysC, userConn: userC, ctx: ctx}, nil
}

func (s *SystemdBackend) ListServices() ([]Service, error) {
	services := []string{
		"mpd.service",
		"mpd-discplayer.service",
		"pipewire-pulse.service",
		"pulseaudio.service",
		"shairport-sync.service",
		"snapclient.service",
		"spotifyd.service",
		"upmpdcli.service",
	}
	out := make([]Service, 0, len(services)*2)

	sysSvcs, _ := s.listServices(s.ctx, s.sysConn, ScopeSystem, services)
	userSvcs, _ := s.listServices(s.ctx, s.userConn, ScopeUser, services)

	out = append(out, sysSvcs...)
	out = append(out, userSvcs...)

	return out, nil
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
		
		// unit existante
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

	return startUnit(s.ctx, conn, name)
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
	return conn.ReloadContext(s.ctx)
}

func (s *SystemdBackend) RestartService(name string, scope UnitScope) error {
	return restartUnit(s.ctx, s.connForScope(scope), name)
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
