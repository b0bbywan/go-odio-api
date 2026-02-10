package systemd

import (
	"testing"

	"github.com/b0bbywan/go-odio-api/cache"
)

func TestGetService(t *testing.T) {
	backend := &SystemdBackend{
		cache: cache.New[[]Service](0),
	}

	// Populate cache with test services
	services := []Service{
		{
			Name:        "test1.service",
			Scope:       ScopeSystem,
			ActiveState: "active",
			Running:     true,
			Enabled:     true,
			Exists:      true,
			Description: "Test Service 1",
		},
		{
			Name:        "test2.service",
			Scope:       ScopeUser,
			ActiveState: "inactive",
			Running:     false,
			Enabled:     false,
			Exists:      true,
			Description: "Test Service 2",
		},
	}
	backend.cache.Set(cacheKey, services)

	tests := []struct {
		name      string
		unitName  string
		scope     UnitScope
		wantFound bool
		wantSvc   *Service
	}{
		{
			name:      "find system service",
			unitName:  "test1.service",
			scope:     ScopeSystem,
			wantFound: true,
			wantSvc: &Service{
				Name:        "test1.service",
				Scope:       ScopeSystem,
				ActiveState: "active",
				Running:     true,
				Enabled:     true,
				Exists:      true,
				Description: "Test Service 1",
			},
		},
		{
			name:      "find user service",
			unitName:  "test2.service",
			scope:     ScopeUser,
			wantFound: true,
			wantSvc: &Service{
				Name:        "test2.service",
				Scope:       ScopeUser,
				ActiveState: "inactive",
				Running:     false,
				Enabled:     false,
				Exists:      true,
				Description: "Test Service 2",
			},
		},
		{
			name:      "service not found",
			unitName:  "nonexistent.service",
			scope:     ScopeSystem,
			wantFound: false,
			wantSvc:   nil,
		},
		{
			name:      "wrong scope",
			unitName:  "test1.service",
			scope:     ScopeUser,
			wantFound: false,
			wantSvc:   nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, found := backend.GetService(tt.unitName, tt.scope)
			if found != tt.wantFound {
				t.Errorf("GetService(%q, %q) found = %v, want %v", tt.unitName, tt.scope, found, tt.wantFound)
			}
			if tt.wantSvc != nil && svc != nil {
				if svc.Name != tt.wantSvc.Name {
					t.Errorf("Name = %q, want %q", svc.Name, tt.wantSvc.Name)
				}
				if svc.Scope != tt.wantSvc.Scope {
					t.Errorf("Scope = %q, want %q", svc.Scope, tt.wantSvc.Scope)
				}
				if svc.Running != tt.wantSvc.Running {
					t.Errorf("Running = %v, want %v", svc.Running, tt.wantSvc.Running)
				}
				if svc.Enabled != tt.wantSvc.Enabled {
					t.Errorf("Enabled = %v, want %v", svc.Enabled, tt.wantSvc.Enabled)
				}
			}
		})
	}
}

func TestGetServiceEmptyCache(t *testing.T) {
	backend := &SystemdBackend{
		cache: cache.New[[]Service](0),
	}

	svc, found := backend.GetService("test.service", ScopeSystem)
	if found {
		t.Error("GetService should return false when cache is empty")
	}
	if svc != nil {
		t.Error("GetService should return nil when cache is empty")
	}
}

func TestUpdateService(t *testing.T) {
	backend := &SystemdBackend{
		cache: cache.New[[]Service](0),
	}

	// Initial cache state
	initialServices := []Service{
		{
			Name:    "test1.service",
			Scope:   ScopeSystem,
			Running: false,
			Enabled: false,
		},
		{
			Name:    "test2.service",
			Scope:   ScopeUser,
			Running: false,
			Enabled: false,
		},
	}
	backend.cache.Set(cacheKey, initialServices)

	// Update an existing service
	updatedService := Service{
		Name:        "test1.service",
		Scope:       ScopeSystem,
		ActiveState: "active",
		Running:     true,
		Enabled:     true,
		Description: "Updated Service",
	}

	err := backend.UpdateService(updatedService)
	if err != nil {
		t.Fatalf("UpdateService failed: %v", err)
	}

	// Verify the service was updated
	svc, found := backend.GetService("test1.service", ScopeSystem)
	if !found {
		t.Fatal("Updated service should be found in cache")
	}
	if !svc.Running {
		t.Error("Service should be running after update")
	}
	if !svc.Enabled {
		t.Error("Service should be enabled after update")
	}
	if svc.Description != "Updated Service" {
		t.Errorf("Description = %q, want %q", svc.Description, "Updated Service")
	}

	// Verify other service wasn't affected
	svc2, found := backend.GetService("test2.service", ScopeUser)
	if !found {
		t.Fatal("Other service should still be in cache")
	}
	if svc2.Running {
		t.Error("Other service should not be affected by update")
	}
}

func TestUpdateServiceAddNew(t *testing.T) {
	backend := &SystemdBackend{
		cache: cache.New[[]Service](0),
	}

	// Initial cache with one service
	initialServices := []Service{
		{
			Name:  "test1.service",
			Scope: ScopeSystem,
		},
	}
	backend.cache.Set(cacheKey, initialServices)

	// Add a new service
	newService := Service{
		Name:    "test2.service",
		Scope:   ScopeUser,
		Running: true,
		Enabled: true,
	}

	err := backend.UpdateService(newService)
	if err != nil {
		t.Fatalf("UpdateService failed: %v", err)
	}

	// Verify the new service was added
	svc, found := backend.GetService("test2.service", ScopeUser)
	if !found {
		t.Fatal("New service should be found in cache")
	}
	if !svc.Running {
		t.Error("New service should be running")
	}

	// Verify we now have 2 services in cache
	services, _ := backend.cache.Get(cacheKey)
	if len(services) != 2 {
		t.Errorf("Cache should contain 2 services, got %d", len(services))
	}
}

func TestInvalidateCache(t *testing.T) {
	backend := &SystemdBackend{
		cache: cache.New[[]Service](0),
	}

	// Populate cache
	services := []Service{
		{Name: "test.service", Scope: ScopeSystem},
	}
	backend.cache.Set(cacheKey, services)

	// Verify cache is populated
	_, found := backend.GetService("test.service", ScopeSystem)
	if !found {
		t.Fatal("Cache should be populated")
	}

	// Invalidate cache
	backend.InvalidateCache()

	// Verify cache is empty
	_, found = backend.GetService("test.service", ScopeSystem)
	if found {
		t.Error("Cache should be empty after invalidation")
	}
}

func TestListenerWatched(t *testing.T) {
	listener := &Listener{
		sysWatched: map[string]bool{
			"system-service.service": true,
			"another.service":        true,
		},
		userWatched: map[string]bool{
			"user-service.service": true,
		},
	}

	tests := []struct {
		name     string
		unitName string
		scope    UnitScope
		expected bool
	}{
		{
			name:     "watched system service",
			unitName: "system-service.service",
			scope:    ScopeSystem,
			expected: true,
		},
		{
			name:     "watched user service",
			unitName: "user-service.service",
			scope:    ScopeUser,
			expected: true,
		},
		{
			name:     "unwatched system service",
			unitName: "unwatched.service",
			scope:    ScopeSystem,
			expected: false,
		},
		{
			name:     "unwatched user service",
			unitName: "unwatched.service",
			scope:    ScopeUser,
			expected: false,
		},
		{
			name:     "wrong scope",
			unitName: "system-service.service",
			scope:    ScopeUser,
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := listener.Watched(tt.unitName, tt.scope)
			if result != tt.expected {
				t.Errorf("Watched(%q, %q) = %v, want %v", tt.unitName, tt.scope, result, tt.expected)
			}
		})
	}
}

// Security Tests

func TestPermissionSystemError(t *testing.T) {
	err := &PermissionSystemError{Unit: "test.service"}
	expected := "can not act on system units: test.service"
	if err.Error() != expected {
		t.Errorf("PermissionSystemError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestPermissionUserError(t *testing.T) {
	err := &PermissionUserError{Unit: "unmanaged.service"}
	expected := "cannot act on unmanaged user unit: unmanaged.service"
	if err.Error() != expected {
		t.Errorf("PermissionUserError.Error() = %q, want %q", err.Error(), expected)
	}
}

func TestCanExecute_SystemScope(t *testing.T) {
	backend := &SystemdBackend{
		listener: &Listener{
			sysWatched: map[string]bool{
				"watched.service": true,
			},
			userWatched: map[string]bool{},
		},
	}

	tests := []struct {
		name      string
		unitName  string
		scope     UnitScope
		wantError bool
		errorType string
	}{
		{
			name:      "system scope always blocked",
			unitName:  "watched.service",
			scope:     ScopeSystem,
			wantError: true,
			errorType: "*systemd.PermissionSystemError",
		},
		{
			name:      "system scope unmanaged unit",
			unitName:  "unwatched.service",
			scope:     ScopeSystem,
			wantError: true,
			errorType: "*systemd.PermissionSystemError",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backend.canExecute(tt.unitName, tt.scope)
			if tt.wantError {
				if err == nil {
					t.Errorf("canExecute(%q, %q) should return error", tt.unitName, tt.scope)
					return
				}
				// Check error type
				switch err.(type) {
				case *PermissionSystemError:
					if tt.errorType != "*systemd.PermissionSystemError" {
						t.Errorf("canExecute error type = %T, want %s", err, tt.errorType)
					}
				case *PermissionUserError:
					if tt.errorType != "*systemd.PermissionUserError" {
						t.Errorf("canExecute error type = %T, want %s", err, tt.errorType)
					}
				default:
					t.Errorf("unexpected error type: %T", err)
				}
			} else {
				if err != nil {
					t.Errorf("canExecute(%q, %q) unexpected error: %v", tt.unitName, tt.scope, err)
				}
			}
		})
	}
}

func TestCanExecute_UserScope(t *testing.T) {
	backend := &SystemdBackend{
		listener: &Listener{
			sysWatched: map[string]bool{},
			userWatched: map[string]bool{
				"managed.service": true,
			},
		},
	}

	tests := []struct {
		name      string
		unitName  string
		wantError bool
	}{
		{
			name:      "managed user unit allowed",
			unitName:  "managed.service",
			wantError: false,
		},
		{
			name:      "unmanaged user unit blocked",
			unitName:  "unmanaged.service",
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := backend.canExecute(tt.unitName, ScopeUser)
			if tt.wantError {
				if err == nil {
					t.Errorf("canExecute(%q, ScopeUser) should return error for unmanaged unit", tt.unitName)
					return
				}
				// Should be PermissionUserError
				if _, ok := err.(*PermissionUserError); !ok {
					t.Errorf("canExecute error type = %T, want *PermissionUserError", err)
				}
			} else {
				if err != nil {
					t.Errorf("canExecute(%q, ScopeUser) unexpected error: %v", tt.unitName, err)
				}
			}
		})
	}
}

func TestCanExecute_WhitelistEnforcement(t *testing.T) {
	// Test that ONLY explicitly whitelisted user units can be executed
	backend := &SystemdBackend{
		listener: &Listener{
			sysWatched: map[string]bool{},
			userWatched: map[string]bool{
				"allowed.service": true,
			},
		},
	}

	// Try common service names that should be blocked
	dangerousServices := []string{
		"ssh.service",
		"sshd.service",
		"systemd-logind.service",
		"dbus.service",
		"polkit.service",
		"../../../etc/passwd",
		"../../systemd/system/dangerous.service",
	}

	for _, service := range dangerousServices {
		t.Run(service, func(t *testing.T) {
			err := backend.canExecute(service, ScopeUser)
			if err == nil {
				t.Errorf("canExecute(%q, ScopeUser) should block non-whitelisted service", service)
			}
			if _, ok := err.(*PermissionUserError); !ok {
				t.Errorf("canExecute should return PermissionUserError, got: %T", err)
			}
		})
	}
}
