package systemd

import (
	"testing"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
)

func TestAddInternalUserUnits(t *testing.T) {
	backend := &SystemdBackend{
		config: &config.SystemdConfig{
			UserServices: []config.SystemdService{{Name: "existing.service"}},
		},
	}

	// Empty names are skipped; duplicates (against config and within the call)
	// are not re-added; an already-configured unit keeps its original flags.
	backend.AddInternalUserUnits("upgrade.service", "", "existing.service", "upgrade.service")

	got := backend.config.UserServices
	if len(got) != 2 {
		t.Fatalf("UserServices = %+v, want 2 entries", got)
	}
	byName := map[string]config.SystemdService{}
	for _, svc := range got {
		byName[svc.Name] = svc
	}
	if svc, ok := byName["upgrade.service"]; !ok || !svc.Internal {
		t.Errorf("upgrade.service = %+v, want present and internal", svc)
	}
	if svc := byName["existing.service"]; svc.Internal {
		t.Errorf("existing.service marked internal, want left untouched")
	}
}

func TestConfiguredInternal(t *testing.T) {
	backend := &SystemdBackend{
		config: &config.SystemdConfig{
			SystemServices: []config.SystemdService{{Name: "sys.service", Internal: true}},
			UserServices: []config.SystemdService{
				{Name: "user.service", Internal: true},
				{Name: "public.service"},
			},
		},
	}

	cases := []struct {
		name  string
		unit  string
		scope UnitScope
		want  bool
	}{
		{"internal user unit", "user.service", ScopeUser, true},
		{"public user unit", "public.service", ScopeUser, false},
		{"internal system unit", "sys.service", ScopeSystem, true},
		{"wrong scope hides internal", "user.service", ScopeSystem, false},
		{"unknown unit", "nope.service", ScopeUser, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := backend.IsInternal(tc.unit, tc.scope); got != tc.want {
				t.Errorf("IsInternal(%q, %q) = %v, want %v", tc.unit, tc.scope, got, tc.want)
			}
		})
	}
}

func TestIsInternalNilBackend(t *testing.T) {
	var backend *SystemdBackend
	if backend.IsInternal("x.service", ScopeUser) {
		t.Error("IsInternal on nil backend = true, want false")
	}
}

func TestPublicServicesFiltersInternal(t *testing.T) {
	backend := &SystemdBackend{cache: cache.New[[]Service](0)}
	backend.cache.Set(cacheKey, []Service{
		{Name: "public.service", Scope: ScopeUser},
		{Name: "upgrade.service", Scope: ScopeUser, Internal: true},
		{Name: "mpd.service", Scope: ScopeUser},
	})

	public, err := backend.PublicServices()
	if err != nil {
		t.Fatalf("PublicServices: %v", err)
	}
	if len(public) != 2 {
		t.Fatalf("PublicServices returned %d services, want 2: %+v", len(public), public)
	}
	for _, svc := range public {
		if svc.Internal {
			t.Errorf("PublicServices leaked internal unit %q", svc.Name)
		}
	}
}
