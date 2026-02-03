package systemd

import (
	"testing"

	"github.com/godbus/dbus/v5"
)

func TestUnitNameFromPath(t *testing.T) {
	tests := []struct {
		name     string
		path     dbus.ObjectPath
		expected string
	}{
		{
			name:     "simple service",
			path:     "/org/freedesktop/systemd1/unit/sshd_2eservice",
			expected: "sshd.service",
		},
		{
			name:     "service with dash",
			path:     "/org/freedesktop/systemd1/unit/my_2dservice_2eservice",
			expected: "my-service.service",
		},
		{
			name:     "spotifyd example",
			path:     "/org/freedesktop/systemd1/unit/spotifyd_2eservice",
			expected: "spotifyd.service",
		},
		{
			name:     "invalid path",
			path:     "/some/other/path",
			expected: "",
		},
		{
			name:     "empty path",
			path:     "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := unitNameFromPath(tt.path)
			if result != tt.expected {
				t.Errorf("unitNameFromPath(%q) = %q, want %q", tt.path, result, tt.expected)
			}
		})
	}
}

func TestDecodeUnitName(t *testing.T) {
	tests := []struct {
		name     string
		encoded  string
		expected string
	}{
		{
			name:     "dot encoded",
			encoded:  "test_2eservice",
			expected: "test.service",
		},
		{
			name:     "dash encoded",
			encoded:  "my_2dservice",
			expected: "my-service",
		},
		{
			name:     "multiple encoded chars",
			encoded:  "test_2dname_2eservice",
			expected: "test-name.service",
		},
		{
			name:     "no encoding",
			encoded:  "simple",
			expected: "simple",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := decodeUnitName(tt.encoded)
			if result != tt.expected {
				t.Errorf("decodeUnitName(%q) = %q, want %q", tt.encoded, result, tt.expected)
			}
		})
	}
}

func TestParseHexByte(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected byte
		wantOk   bool
	}{
		{
			name:     "valid hex 2e (dot)",
			input:    "2e",
			expected: 0x2e,
			wantOk:   true,
		},
		{
			name:     "valid hex 2d (dash)",
			input:    "2d",
			expected: 0x2d,
			wantOk:   true,
		},
		{
			name:     "uppercase hex",
			input:    "2E",
			expected: 0x2e,
			wantOk:   true,
		},
		{
			name:     "invalid - too short",
			input:    "2",
			expected: 0,
			wantOk:   false,
		},
		{
			name:     "invalid - too long",
			input:    "2ef",
			expected: 0,
			wantOk:   false,
		},
		{
			name:     "invalid - non-hex char",
			input:    "2g",
			expected: 0,
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var result byte
			ok, _ := parseHexByte(tt.input, &result)
			if ok != tt.wantOk {
				t.Errorf("parseHexByte(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if ok && result != tt.expected {
				t.Errorf("parseHexByte(%q) = %#x, want %#x", tt.input, result, tt.expected)
			}
		})
	}
}

func TestStateKey(t *testing.T) {
	tests := []struct {
		name     string
		unitName string
		scope    UnitScope
		expected string
	}{
		{
			name:     "system service",
			unitName: "sshd.service",
			scope:    ScopeSystem,
			expected: "system/sshd.service",
		},
		{
			name:     "user service",
			unitName: "spotifyd.service",
			scope:    ScopeUser,
			expected: "user/spotifyd.service",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := stateKey(tt.unitName, tt.scope)
			if result != tt.expected {
				t.Errorf("stateKey(%q, %q) = %q, want %q", tt.unitName, tt.scope, result, tt.expected)
			}
		})
	}
}

func TestServiceFromProps(t *testing.T) {
	tests := []struct {
		name     string
		unitName string
		scope    UnitScope
		props    map[string]interface{}
		expected Service
	}{
		{
			name:     "running enabled service",
			unitName: "test.service",
			scope:    ScopeSystem,
			props: map[string]interface{}{
				"UnitFileState": "enabled",
				"ActiveState":   "active",
				"SubState":      "running",
				"Description":   "Test Service",
			},
			expected: Service{
				Name:        "test.service",
				Scope:       ScopeSystem,
				ActiveState: "active",
				Running:     true,
				Enabled:     true,
				Exists:      true,
				Description: "Test Service",
			},
		},
		{
			name:     "stopped disabled service",
			unitName: "test.service",
			scope:    ScopeUser,
			props: map[string]interface{}{
				"UnitFileState": "disabled",
				"ActiveState":   "inactive",
				"SubState":      "dead",
				"Description":   "Test Service",
			},
			expected: Service{
				Name:        "test.service",
				Scope:       ScopeUser,
				ActiveState: "inactive",
				Running:     false,
				Enabled:     false,
				Exists:      true,
				Description: "Test Service",
			},
		},
		{
			name:     "non-existent service",
			unitName: "missing.service",
			scope:    ScopeSystem,
			props:    nil,
			expected: Service{
				Name:    "missing.service",
				Scope:   ScopeSystem,
				Exists:  false,
				Enabled: false,
			},
		},
		{
			name:     "service without description",
			unitName: "test.service",
			scope:    ScopeSystem,
			props: map[string]interface{}{
				"UnitFileState": "enabled",
				"ActiveState":   "active",
				"SubState":      "running",
			},
			expected: Service{
				Name:        "test.service",
				Scope:       ScopeSystem,
				ActiveState: "active",
				Running:     true,
				Enabled:     true,
				Exists:      true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := serviceFromProps(tt.unitName, tt.scope, tt.props)
			if result.Name != tt.expected.Name {
				t.Errorf("Name = %q, want %q", result.Name, tt.expected.Name)
			}
			if result.Scope != tt.expected.Scope {
				t.Errorf("Scope = %q, want %q", result.Scope, tt.expected.Scope)
			}
			if result.ActiveState != tt.expected.ActiveState {
				t.Errorf("ActiveState = %q, want %q", result.ActiveState, tt.expected.ActiveState)
			}
			if result.Running != tt.expected.Running {
				t.Errorf("Running = %v, want %v", result.Running, tt.expected.Running)
			}
			if result.Enabled != tt.expected.Enabled {
				t.Errorf("Enabled = %v, want %v", result.Enabled, tt.expected.Enabled)
			}
			if result.Exists != tt.expected.Exists {
				t.Errorf("Exists = %v, want %v", result.Exists, tt.expected.Exists)
			}
			if result.Description != tt.expected.Description {
				t.Errorf("Description = %q, want %q", result.Description, tt.expected.Description)
			}
		})
	}
}

func TestParseUnitScope(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected UnitScope
		wantOk   bool
	}{
		{
			name:     "system scope",
			input:    "system",
			expected: ScopeSystem,
			wantOk:   true,
		},
		{
			name:     "user scope",
			input:    "user",
			expected: ScopeUser,
			wantOk:   true,
		},
		{
			name:     "invalid scope",
			input:    "invalid",
			expected: "",
			wantOk:   false,
		},
		{
			name:     "empty scope",
			input:    "",
			expected: "",
			wantOk:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, ok := ParseUnitScope(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseUnitScope(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
			}
			if result != tt.expected {
				t.Errorf("ParseUnitScope(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
