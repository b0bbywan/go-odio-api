package config

import (
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected logger.Level
	}{
		{"debug", logger.DEBUG},
		{"DEBUG", logger.DEBUG},
		{"Debug", logger.DEBUG},
		{"info", logger.INFO},
		{"INFO", logger.INFO},
		{"warn", logger.WARN},
		{"WARN", logger.WARN},
		{"error", logger.ERROR},
		{"ERROR", logger.ERROR},
		{"fatal", logger.FATAL},
		{"FATAL", logger.FATAL},
		{"unknown", logger.WARN}, // default
		{"", logger.WARN},        // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestConfigStructFields(t *testing.T) {
	// Just verify the Config struct has the expected fields
	cfg := &Config{
		Systemd: &SystemdConfig{
			SupportsUTMP: false,
		},
		Api: &ApiConfig{
			Port: 8080,
		},
		LogLevel: logger.INFO,
	}

	if cfg.Api.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Api.Port)
	}
	if cfg.LogLevel != logger.INFO {
		t.Errorf("LogLevel = %d, want %d", cfg.LogLevel, logger.INFO)
	}
	if cfg.Systemd.SupportsUTMP != false {
		t.Errorf("Headless = %v, want false", cfg.Systemd.SupportsUTMP)
	}
	if cfg.Systemd == nil {
		t.Error("Services should not be nil")
	}
}

func TestSystemdConfigStructFields(t *testing.T) {
	sysCfg := &SystemdConfig{
		SystemServices: []SystemdService{{Name: "service1"}, {Name: "service2"}},
		UserServices:   []SystemdService{{Name: "user-service1"}},
	}

	if len(sysCfg.SystemServices) != 2 {
		t.Errorf("SystemServices length = %d, want 2", len(sysCfg.SystemServices))
	}
	if len(sysCfg.UserServices) != 1 {
		t.Errorf("UserServices length = %d, want 1", len(sysCfg.UserServices))
	}
}

func TestNew_UIConfig(t *testing.T) {
	tests := []struct {
		name      string
		uiEnabled bool
	}{
		{"UI disabled by default", false},
		{"UI enabled", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			viper.Set("api.ui.enabled", tt.uiEnabled)
			t.Setenv("HOME", t.TempDir())

			cfg, err := New(nil)
			if err != nil {
				t.Fatalf("New(nil) returned error: %v", err)
			}

			if cfg.Api.UI == nil {
				t.Fatal("Api.UI should not be nil")
			}
			if cfg.Api.UI.Enabled != tt.uiEnabled {
				t.Errorf("Api.UI.Enabled = %v, want %v", cfg.Api.UI.Enabled, tt.uiEnabled)
			}
		})
	}
}

func TestNew_UIEnabledByDefault(t *testing.T) {
	viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if cfg.Api.UI == nil {
		t.Fatal("Api.UI should not be nil")
	}
	if !cfg.Api.UI.Enabled {
		t.Error("Api.UI.Enabled should be true by default")
	}
}

func BenchmarkParseLogLevel(b *testing.B) {
	for i := 0; i < b.N; i++ {
		parseLogLevel("DEBUG")
	}
}

func TestNew_Defaults(t *testing.T) {
	// Reset viper to ensure clean state
	viper.Reset()

	// Isolate from user's config files by using a temp directory
	t.Setenv("HOME", t.TempDir())

	// Set XDG_SESSION_DESKTOP to avoid headless mode detection
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Test default port
	if cfg.Api.Port != 8018 {
		t.Errorf("Api.Port = %d, want 8018", cfg.Api.Port)
	}

	// Test default enabled flags
	if !cfg.Api.Enabled {
		t.Error("Api.Enabled should be true by default")
	}
	if cfg.Systemd.Enabled {
		t.Error("Systemd.Enabled should be false by default")
	}
	if !cfg.Pulseaudio.Enabled {
		t.Error("Pulseaudio.Enabled should be true by default")
	}
	if !cfg.MPRIS.Enabled {
		t.Error("MPRIS.Enabled should be true by default")
	}

	// Test default log level
	if cfg.LogLevel != logger.INFO {
		t.Errorf("LogLevel = %d, want %d (INFO)", cfg.LogLevel, logger.INFO)
	}
}

// The upgrade run-state file defaults to the persistent state dir: $XDG_STATE_HOME when
// set, else $HOME/.local/state — never the tmpfs runtime dir used for the progress socket.
func TestNew_UpgradeStateFileDefault(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	t.Setenv("XDG_STATE_HOME", "") // fallback path
	viper.Reset()
	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	if got, want := cfg.Upgrade.StateFile, filepath.Join(home, ".local", "state", "odio-api", "upgrade-run.json"); got != want {
		t.Errorf("Upgrade.StateFile = %q, want %q", got, want)
	}

	stateHome := t.TempDir()
	t.Setenv("XDG_STATE_HOME", stateHome)
	viper.Reset()
	cfg, err = New(nil)
	if err != nil {
		t.Fatalf("New(nil): %v", err)
	}
	if got, want := cfg.Upgrade.StateFile, filepath.Join(stateHome, "odio-api", "upgrade-run.json"); got != want {
		t.Errorf("Upgrade.StateFile = %q, want %q", got, want)
	}
}

func TestNew_CustomPort(t *testing.T) {
	// Reset viper to ensure clean state
	viper.Reset()

	// Set custom port
	viper.Set("api.port", 9090)

	// Isolate from user's config files by using a temp directory
	t.Setenv("HOME", t.TempDir())

	// Set XDG_SESSION_DESKTOP to avoid headless mode detection
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Api.Port != 9090 {
		t.Errorf("Api.Port = %d, want 9090", cfg.Api.Port)
	}
}

func TestNew_InvalidPort(t *testing.T) {
	tests := []struct {
		name string
		port int
	}{
		{"zero port", 0},
		{"negative port", -1},
		{"port too high", 65536},
		{"port way too high", 100000},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Reset viper to ensure clean state
			viper.Reset()

			// Set invalid port
			viper.Set("api.port", tt.port)

			// Isolate from user's config files by using a temp directory
			t.Setenv("HOME", t.TempDir())

			// Set XDG_SESSION_DESKTOP to avoid headless mode detection
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err == nil {
				t.Errorf("New(nil) with port %d should return error, got config: %+v", tt.port, cfg)
			}
			if cfg != nil {
				t.Errorf("New(nil) with invalid port should return nil config, got: %+v", cfg)
			}
		})
	}
}

func TestNew_CustomLogLevel(t *testing.T) {
	tests := []struct {
		level    string
		expected logger.Level
	}{
		{"DEBUG", logger.DEBUG},
		{"INFO", logger.INFO},
		{"WARN", logger.WARN},
		{"ERROR", logger.ERROR},
		{"FATAL", logger.FATAL},
	}

	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			// Reset viper to ensure clean state
			viper.Reset()

			viper.Set("LogLevel", tt.level)

			// Isolate from user's config files by using a temp directory
			t.Setenv("HOME", t.TempDir())

			// Set XDG_SESSION_DESKTOP to avoid headless mode detection
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err != nil {
				t.Fatalf("New(nil) returned error: %v", err)
			}

			if cfg.LogLevel != tt.expected {
				t.Errorf("LogLevel = %d, want %d (%s)", cfg.LogLevel, tt.expected, tt.level)
			}
		})
	}
}

func TestValidateConfigPath_ValidFiles(t *testing.T) {
	tests := []struct {
		name      string
		extension string
	}{
		{"yaml extension", ".yaml"},
		{"yml extension", ".yml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the given extension
			tmpDir := t.TempDir()
			tmpFile := tmpDir + "/config" + tt.extension
			if err := os.WriteFile(tmpFile, []byte("test: value"), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Validate the file
			err := validateConfigPath(tmpFile)
			if err != nil {
				t.Errorf("validateConfigPath(%q) returned error: %v, want nil", tmpFile, err)
			}
		})
	}
}

func TestValidateConfigPath_InvalidExtensions(t *testing.T) {
	tests := []struct {
		name      string
		extension string
	}{
		{"no extension", ""},
		{"txt extension", ".txt"},
		{"json extension", ".json"},
		{"toml extension", ".toml"},
		{"conf extension", ".conf"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with invalid extension
			tmpDir := t.TempDir()
			tmpFile := tmpDir + "/config" + tt.extension
			if err := os.WriteFile(tmpFile, []byte("test: value"), 0644); err != nil {
				t.Fatalf("Failed to create test file: %v", err)
			}

			// Validate should fail
			err := validateConfigPath(tmpFile)
			if err == nil {
				t.Errorf("validateConfigPath(%q) should return error for invalid extension", tmpFile)
			}
		})
	}
}

func TestValidateConfigPath_FileNotExists(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := tmpDir + "/nonexistent.yaml"

	err := validateConfigPath(nonExistentFile)
	if err == nil {
		t.Error("validateConfigPath should return error for non-existent file")
	}
}

func TestValidateConfigPath_Directory(t *testing.T) {
	// Create a directory with .yaml extension
	tmpDir := t.TempDir()
	configDir := tmpDir + "/config.yaml"
	if err := os.Mkdir(configDir, 0755); err != nil {
		t.Fatalf("Failed to create test directory: %v", err)
	}

	err := validateConfigPath(configDir)
	if err == nil {
		t.Error("validateConfigPath should return error for directory")
	}
}

func TestValidateConfigPath_PathTraversal(t *testing.T) {
	tests := []struct {
		name string
		path string
	}{
		{"etc passwd", "/etc/passwd"},
		{"etc shadow", "/etc/shadow"},
		{"etc hosts", "/etc/hosts"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// These files don't have .yaml extension, should fail
			err := validateConfigPath(tt.path)
			if err == nil {
				t.Errorf("validateConfigPath(%q) should return error for system file", tt.path)
			}
		})
	}
}

func TestNew_InvalidConfigFile(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	invalidFile := tmpDir + "/invalid.txt"
	if err := os.WriteFile(invalidFile, []byte("test"), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&invalidFile)
	if err == nil {
		t.Error("New() should return error for invalid config file extension")
	}
	if cfg != nil {
		t.Errorf("New() should return nil config for invalid file, got: %+v", cfg)
	}
}

func TestNew_ValidConfigFile(t *testing.T) {
	viper.Reset()

	// Create a valid YAML config file
	tmpDir := t.TempDir()
	validFile := tmpDir + "/config.yaml"
	configContent := `
api:
  port: 9999
  enabled: true
logLevel: DEBUG
`
	if err := os.WriteFile(validFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&validFile)
	if err != nil {
		t.Fatalf("New() with valid config file returned error: %v", err)
	}
	if cfg.Api.Port != 9999 {
		t.Errorf("Api.Port = %d, want 9999", cfg.Api.Port)
	}
	if cfg.LogLevel != logger.DEBUG {
		t.Errorf("LogLevel = %d, want %d (DEBUG)", cfg.LogLevel, logger.DEBUG)
	}
}

// --- Tests Login1Config ---

func TestLogin1ConfigStructFields(t *testing.T) {
	cfg := &Login1Config{
		Enabled:      true,
		Capabilities: &Login1Capabilities{CanReboot: true, CanPoweroff: false},
	}
	if !cfg.Enabled {
		t.Error("Login1Config.Enabled should be true")
	}
	if cfg.Capabilities == nil {
		t.Fatal("Login1Config.Capabilities should not be nil")
	}
	if !cfg.Capabilities.CanReboot {
		t.Error("Login1Capabilities.CanReboot should be true")
	}
	if cfg.Capabilities.CanPoweroff {
		t.Error("Login1Capabilities.CanPoweroff should be false")
	}
}

func TestLogin1CapabilitiesStructFields(t *testing.T) {
	tests := []struct {
		name        string
		canReboot   bool
		canPoweroff bool
	}{
		{"both disabled", false, false},
		{"reboot only", true, false},
		{"poweroff only", false, true},
		{"both enabled", true, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := Login1Capabilities{CanReboot: tt.canReboot, CanPoweroff: tt.canPoweroff}
			if caps.CanReboot != tt.canReboot {
				t.Errorf("CanReboot = %v, want %v", caps.CanReboot, tt.canReboot)
			}
			if caps.CanPoweroff != tt.canPoweroff {
				t.Errorf("CanPoweroff = %v, want %v", caps.CanPoweroff, tt.canPoweroff)
			}
		})
	}
}

func TestNew_Login1DisabledByDefault(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Login1 == nil {
		t.Fatal("Login1 config should not be nil")
	}
	// Power/login1 must be DISABLED by default for security
	if cfg.Login1.Enabled {
		t.Error("Login1.Enabled should be false by default for security")
	}
}

func TestNew_Login1CapabilitiesDisabledByDefault(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Login1.Capabilities == nil {
		t.Fatal("Login1.Capabilities should not be nil")
	}
	if cfg.Login1.Capabilities.CanReboot {
		t.Error("Login1.Capabilities.CanReboot should be false by default")
	}
	if cfg.Login1.Capabilities.CanPoweroff {
		t.Error("Login1.Capabilities.CanPoweroff should be false by default")
	}
}

func TestNew_Login1ExplicitlyEnabled(t *testing.T) {
	viper.Reset()
	viper.Set("power.enabled", true)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if !cfg.Login1.Enabled {
		t.Error("Login1.Enabled should be true when explicitly enabled")
	}
}

func TestNew_Login1CapabilitiesFromViper(t *testing.T) {
	tests := []struct {
		name     string
		reboot   bool
		poweroff bool
	}{
		{"reboot only", true, false},
		{"poweroff only", false, true},
		{"both capabilities", true, true},
		{"no capability", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			viper.Set("power.enabled", true)
			viper.Set("power.capabilities.reboot", tt.reboot)
			viper.Set("power.capabilities.poweroff", tt.poweroff)

			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err != nil {
				t.Fatalf("New(nil) returned error: %v", err)
			}

			if cfg.Login1.Capabilities.CanReboot != tt.reboot {
				t.Errorf("CanReboot = %v, want %v", cfg.Login1.Capabilities.CanReboot, tt.reboot)
			}
			if cfg.Login1.Capabilities.CanPoweroff != tt.poweroff {
				t.Errorf("CanPoweroff = %v, want %v", cfg.Login1.Capabilities.CanPoweroff, tt.poweroff)
			}
		})
	}
}

func TestNew_Login1FromConfigFile(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	configContent := `
power:
  enabled: true
  capabilities:
    reboot: true
    poweroff: false
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&configFile)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if !cfg.Login1.Enabled {
		t.Error("Login1.Enabled should be true from config file")
	}
	if cfg.Login1.Capabilities == nil {
		t.Fatal("Login1.Capabilities should not be nil")
	}
	if !cfg.Login1.Capabilities.CanReboot {
		t.Error("Login1.Capabilities.CanReboot should be true from config file")
	}
	if cfg.Login1.Capabilities.CanPoweroff {
		t.Error("Login1.Capabilities.CanPoweroff should be false from config file")
	}
}

func TestNew_Login1SecurityDefaults(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Verify all login1 security defaults
	securityTests := []struct {
		name     string
		got      interface{}
		want     interface{}
		errorMsg string
	}{
		{
			name:     "power disabled",
			got:      cfg.Login1.Enabled,
			want:     false,
			errorMsg: "Login1 (power management) should be disabled by default for security",
		},
		{
			name:     "reboot disabled",
			got:      cfg.Login1.Capabilities.CanReboot,
			want:     false,
			errorMsg: "Login1 CanReboot should be disabled by default for security",
		},
		{
			name:     "poweroff disabled",
			got:      cfg.Login1.Capabilities.CanPoweroff,
			want:     false,
			errorMsg: "Login1 CanPoweroff should be disabled by default for security",
		},
	}

	for _, tt := range securityTests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %v, want %v", tt.errorMsg, tt.got, tt.want)
			}
		})
	}
}

// Security-focused API and Zeroconf tests

func TestNew_DefaultBindLocalhost(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Should always include localhost for security
	loopback := "127.0.0.1:8018"
	found := false
	for _, l := range cfg.Api.Listens {
		if l == loopback {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Api.Listens = %v, want to contain %q (localhost by default)", cfg.Api.Listens, loopback)
	}
}

func TestNew_CustomBindAddress(t *testing.T) {
	tests := []struct {
		name          string
		bind          string
		port          int
		expectContain string // address that must appear in Listens
	}{
		{
			name:          "explicit localhost",
			bind:          "lo",
			port:          8080,
			expectContain: "127.0.0.1:8080",
		},
		{
			name:          "all interfaces",
			bind:          "all",
			port:          8018,
			expectContain: "0.0.0.0:8018",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			viper.Reset()
			viper.Set("bind", tt.bind)
			viper.Set("api.port", tt.port)

			t.Setenv("HOME", t.TempDir())
			t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

			cfg, err := New(nil)
			if err != nil {
				t.Fatalf("New(nil) returned error: %v", err)
			}

			found := false
			for _, l := range cfg.Api.Listens {
				if l == tt.expectContain {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Api.Listens = %v, want to contain %q", cfg.Api.Listens, tt.expectContain)
			}
		})
	}
}

func TestNew_ZeroconfDisabledOnLocalhost(t *testing.T) {
	viper.Reset()
	viper.Set("bind", "lo")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Zeroconf should be enabled by default
	if !cfg.Zeroconf.Enabled {
		t.Error("Zeroconf.Enabled should be true by default")
	}

	// But Listen interfaces should be empty on localhost for security
	if len(cfg.Zeroconf.Listen) != 0 {
		t.Errorf("Zeroconf.Listen should be empty on localhost, got: %v", cfg.Zeroconf.Listen)
	}
}

func TestNew_ZeroconfExplicitlyDisabled(t *testing.T) {
	viper.Reset()
	viper.Set("zeroconf.enabled", false)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Zeroconf.Enabled {
		t.Error("Zeroconf.Enabled should be false when explicitly disabled")
	}
}

func TestNew_SystemdDisabledByDefault(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Systemd should be DISABLED by default for security
	if cfg.Systemd.Enabled {
		t.Error("Systemd.Enabled should be false by default for security")
	}

	// Services lists should be empty by default
	if len(cfg.Systemd.SystemServices) != 0 {
		t.Errorf("Systemd.SystemServices should be empty by default, got: %v", cfg.Systemd.SystemServices)
	}
	if len(cfg.Systemd.UserServices) != 0 {
		t.Errorf("Systemd.UserServices should be empty by default, got: %v", cfg.Systemd.UserServices)
	}
}

func TestNew_SystemdExplicitlyEnabled(t *testing.T) {
	viper.Reset()
	viper.Set("systemd.enabled", true)
	viper.Set("systemd.user", []string{"test.service"})

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if !cfg.Systemd.Enabled {
		t.Error("Systemd.Enabled should be true when explicitly enabled")
	}

	if len(cfg.Systemd.UserServices) != 1 || cfg.Systemd.UserServices[0].Name != "test.service" {
		t.Errorf("Systemd.UserServices = %v, want [test.service]", cfg.Systemd.UserServices)
	}
}

func TestNew_SecurityDefaults(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	// Verify all security-critical defaults
	tests := []struct {
		name     string
		got      interface{}
		want     interface{}
		errorMsg string
	}{
		{
			name:     "bind localhost",
			got:      cfg.Api.Listens[0],
			want:     "127.0.0.1:8018",
			errorMsg: "API should include localhost first by default",
		},
		{
			name:     "systemd disabled",
			got:      cfg.Systemd.Enabled,
			want:     false,
			errorMsg: "Systemd should be disabled by default",
		},
		{
			name:     "non-standard port",
			got:      cfg.Api.Port,
			want:     8018,
			errorMsg: "API should use non-standard port 8018 by default",
		},
		{
			name:     "empty systemd system services",
			got:      len(cfg.Systemd.SystemServices),
			want:     0,
			errorMsg: "No system services should be configured by default",
		},
		{
			name:     "empty systemd user services",
			got:      len(cfg.Systemd.UserServices),
			want:     0,
			errorMsg: "No user services should be configured by default",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("%s: got %v, want %v", tt.errorMsg, tt.got, tt.want)
			}
		})
	}
}

// Tests for network interface helpers
func TestGetZeroconfInterfaces_Localhost(t *testing.T) {
	// Localhost should return nil (no zeroconf on loopback)
	interfaces := getZeroconfInterfaces([]string{"lo"})

	if interfaces != nil {
		t.Errorf("getZeroconfInterfaces(lo) = %v, want nil (no zeroconf on localhost)", interfaces)
	}
}

func TestGetZeroconfInterfaces_AllInterfaces(t *testing.T) {
	// 0.0.0.0 should return all active non-loopback interfaces
	interfaces := getZeroconfInterfaces([]string{"all"})

	// Should call getAllActiveInterfaces() which filters loopback
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			t.Errorf("getZeroconfInterfaces(all) returned loopback interface: %s", iface.Name)
		}
		if iface.Flags&net.FlagUp == 0 {
			t.Errorf("getZeroconfInterfaces(all) returned DOWN interface: %s", iface.Name)
		}
	}
}

func TestGetZeroconfInterfaces_InvalidIP(t *testing.T) {
	tests := []string{
		"not-an-ip",
		"999.999.999.999",
		"192.168.1.999",
		"",
	}

	for _, ip := range tests {
		t.Run(ip, func(t *testing.T) {
			interfaces := getZeroconfInterfaces([]string{ip})

			// Invalid IPs should return nil (with warning logged)
			if interfaces != nil {
				t.Errorf("getZeroconfInterfaces(%q) = %v, want nil for invalid IP", ip, interfaces)
			}
		})
	}
}

func TestGetZeroconfInterfaces_NonexistentIP(t *testing.T) {
	// IP that's valid but doesn't exist on this machine
	nonexistentIP := "192.168.99.99"
	interfaces := getZeroconfInterfaces([]string{nonexistentIP})

	// Should return nil since no interface has this IP
	if interfaces != nil {
		t.Errorf("getZeroconfInterfaces(%q) = %v, want nil for nonexistent IP", nonexistentIP, interfaces)
	}
}

func TestNew_ZeroconfAllInterfaces(t *testing.T) {
	viper.Reset()
	viper.Set("bind", "all")

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if !cfg.Zeroconf.Enabled {
		t.Error("Zeroconf.Enabled should be true by default")
	}

	// With bind=0.0.0.0, should have interfaces populated
	// (may be 0 on minimal test environments with no network)
	// Main assertion: no loopback interfaces
	for _, iface := range cfg.Zeroconf.Listen {
		if iface.Flags&net.FlagLoopback != 0 {
			t.Errorf("Zeroconf.Listen contains loopback interface: %s", iface.Name)
		}
	}
}

// --- Tests CORSConfig ---

func TestNew_CORSDefaultOrigin(t *testing.T) {
	viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}
	if cfg.Api.CORS == nil {
		t.Fatal("Api.CORS should not be nil with default origin")
	}
	want := []string{"https://odio-pwa.vercel.app", "https://pwa.odio.love"}
	if len(cfg.Api.CORS.Origins) != len(want) {
		t.Errorf("Api.CORS.Origins = %v, want %v", cfg.Api.CORS.Origins, want)
	} else {
		for i, o := range want {
			if cfg.Api.CORS.Origins[i] != o {
				t.Errorf("Api.CORS.Origins[%d] = %q, want %q", i, cfg.Api.CORS.Origins[i], o)
			}
		}
	}
}

func TestNew_CORSWildcard(t *testing.T) {
	viper.Reset()
	viper.Set("api.cors.origins", []string{"*"})
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}
	if cfg.Api.CORS == nil {
		t.Fatal("Api.CORS should not be nil when origins are configured")
	}
	if len(cfg.Api.CORS.Origins) != 1 || cfg.Api.CORS.Origins[0] != "*" {
		t.Errorf("Api.CORS.Origins = %v, want [*]", cfg.Api.CORS.Origins)
	}
}

func TestNew_CORSSpecificOrigins(t *testing.T) {
	viper.Reset()
	origins := []string{"https://app.example.com", "https://other.example.com"}
	viper.Set("api.cors.origins", origins)
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}
	if cfg.Api.CORS == nil {
		t.Fatal("Api.CORS should not be nil when origins are configured")
	}
	if len(cfg.Api.CORS.Origins) != len(origins) {
		t.Fatalf("Api.CORS.Origins len = %d, want %d", len(cfg.Api.CORS.Origins), len(origins))
	}
	for i, o := range origins {
		if cfg.Api.CORS.Origins[i] != o {
			t.Errorf("Api.CORS.Origins[%d] = %q, want %q", i, cfg.Api.CORS.Origins[i], o)
		}
	}
}

func TestNew_CORSFromConfigFile(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	configContent := `
api:
  cors:
    origins:
      - "*"
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create test config file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&configFile)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if cfg.Api.CORS == nil {
		t.Fatal("Api.CORS should not be nil when loaded from config file")
	}
	if len(cfg.Api.CORS.Origins) != 1 || cfg.Api.CORS.Origins[0] != "*" {
		t.Errorf("Api.CORS.Origins = %v, want [*]", cfg.Api.CORS.Origins)
	}
}

func TestNew_CORSEmptyOriginsStaysNil(t *testing.T) {
	viper.Reset()
	viper.Set("api.cors.origins", []string{})
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}
	if cfg.Api.CORS != nil {
		t.Errorf("Api.CORS should be nil for empty origins list, got %+v", cfg.Api.CORS)
	}
}

// --- Tests SystemdService parsing (string OR object) ---

func TestParseSystemdServices_Nil(t *testing.T) {
	got, err := parseSystemdServices(nil)
	if err != nil {
		t.Fatalf("parseSystemdServices(nil) error: %v", err)
	}
	if got != nil {
		t.Errorf("parseSystemdServices(nil) = %v, want nil", got)
	}
}

func TestParseSystemdServices_StringSlice(t *testing.T) {
	// Backward-compat path: viper.Set / GetStringSlice may yield []string.
	got, err := parseSystemdServices([]string{"a.service", "b.service"})
	if err != nil {
		t.Fatalf("parseSystemdServices() error: %v", err)
	}
	want := []SystemdService{{Name: "a.service"}, {Name: "b.service"}}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSystemdServices_StringEntries(t *testing.T) {
	// Post-YAML viper shape: []any of strings.
	got, err := parseSystemdServices([]any{"a.service", "b.service"})
	if err != nil {
		t.Fatalf("parseSystemdServices() error: %v", err)
	}
	if len(got) != 2 || got[0].Name != "a.service" || got[1].Name != "b.service" {
		t.Errorf("got %+v", got)
	}
	if got[0].URL != "" || got[1].URL != "" {
		t.Error("string entries must yield empty URLs")
	}
}

func TestParseSystemdServices_ObjectEntries(t *testing.T) {
	got, err := parseSystemdServices([]any{
		map[string]any{"name": "mympd.service", "url": ":8080"},
	})
	if err != nil {
		t.Fatalf("parseSystemdServices() error: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].Name != "mympd.service" {
		t.Errorf("Name = %q, want mympd.service", got[0].Name)
	}
	if got[0].URL != ":8080" {
		t.Errorf("URL = %q, want :8080", got[0].URL)
	}
}

func TestParseSystemdServices_MixedEntries(t *testing.T) {
	got, err := parseSystemdServices([]any{
		"mpd.service",
		map[string]any{"name": "mympd.service", "url": ":8080"},
		"pipewire-pulse.service",
	})
	if err != nil {
		t.Fatalf("parseSystemdServices() error: %v", err)
	}
	want := []SystemdService{
		{Name: "mpd.service"},
		{Name: "mympd.service", URL: ":8080"},
		{Name: "pipewire-pulse.service"},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseSystemdServices_ObjectWithoutURL(t *testing.T) {
	// Object form without a URL is equivalent to the bare-string form.
	got, err := parseSystemdServices([]any{
		map[string]any{"name": "mpd.service"},
	})
	if err != nil {
		t.Fatalf("parseSystemdServices() error: %v", err)
	}
	if len(got) != 1 || got[0].Name != "mpd.service" || got[0].URL != "" {
		t.Errorf("got %+v, want [{mpd.service }]", got)
	}
}

func TestParseSystemdServices_EmptyName(t *testing.T) {
	tests := []struct {
		name  string
		entry any
	}{
		{"empty string", ""},
		{"object missing name", map[string]any{"url": ":8080"}},
		{"object empty name", map[string]any{"name": "", "url": ":8080"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := parseSystemdServices([]any{tt.entry})
			if err == nil {
				t.Errorf("expected error for %v, got nil", tt.entry)
			}
		})
	}
}

func TestParseSystemdServices_InvalidEntryType(t *testing.T) {
	// Numbers / lists / bools should be rejected.
	tests := []any{42, true, []any{"nested"}, 3.14}
	for _, e := range tests {
		_, err := parseSystemdServices([]any{e})
		if err == nil {
			t.Errorf("expected error for entry %T (%v), got nil", e, e)
		}
	}
}

func TestParseSystemdServices_WrongFieldType(t *testing.T) {
	// `url: 8080` is a number, not a string. mapstructure should reject it
	// rather than silently producing an empty URL.
	_, err := parseSystemdServices([]any{
		map[string]any{"name": "mympd.service", "url": 8080},
	})
	if err == nil {
		t.Error("expected error when 'url' is not a string")
	}
}

func TestParseSystemdServices_TopLevelNotAList(t *testing.T) {
	_, err := parseSystemdServices("not a list")
	if err == nil {
		t.Error("expected error when top-level value is not a list")
	}
}

// End-to-end: URL declared in YAML flows through to SystemdConfig.
func TestNew_SystemdServicesWithURLFromConfigFile(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	configContent := `
systemd:
  enabled: true
  user:
    - mpd.service
    - name: mympd.service
      url: ":8080"
    - pipewire-pulse.service
`
	if err := os.WriteFile(configFile, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&configFile)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	want := []SystemdService{
		{Name: "mpd.service"},
		{Name: "mympd.service", URL: ":8080"},
		{Name: "pipewire-pulse.service"},
	}
	if len(cfg.Systemd.UserServices) != len(want) {
		t.Fatalf("UserServices = %+v, want %+v", cfg.Systemd.UserServices, want)
	}
	for i := range want {
		if cfg.Systemd.UserServices[i] != want[i] {
			t.Errorf("[%d] = %+v, want %+v", i, cfg.Systemd.UserServices[i], want[i])
		}
	}
}

// End-to-end: a conf.d snippet may also use the object form.
func TestNew_SystemdServicesWithURLFromConfD(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	if err := os.WriteFile(mainConfig, []byte("systemd:\n  enabled: true\n  user: [mpd.service]\n"), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	override := `
systemd:
  user:
    - name: mympd.service
      url: ":8080"
`
	if err := os.WriteFile(confDir+"/10-override.yaml", []byte(override), 0644); err != nil {
		t.Fatalf("Failed to write override: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if len(cfg.Systemd.UserServices) != 1 {
		t.Fatalf("UserServices len = %d, want 1 (conf.d replaces). Got: %+v",
			len(cfg.Systemd.UserServices), cfg.Systemd.UserServices)
	}
	got := cfg.Systemd.UserServices[0]
	if got.Name != "mympd.service" {
		t.Errorf("Name = %q, want mympd.service", got.Name)
	}
	if got.URL != ":8080" {
		t.Errorf("URL = %q, want :8080", got.URL)
	}
}

// Surface a malformed YAML entry as an error from New() rather than silent drop.
func TestNew_SystemdInvalidEntryType(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	configFile := tmpDir + "/config.yaml"
	if err := os.WriteFile(configFile, []byte("systemd:\n  enabled: true\n  user: [42]\n"), 0644); err != nil {
		t.Fatalf("Failed to create config file: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&configFile)
	if err == nil {
		t.Fatalf("expected error for invalid entry type, got cfg: %+v", cfg)
	}
	if !strings.Contains(err.Error(), "systemd.user") {
		t.Errorf("error should mention systemd.user, got: %v", err)
	}
}

// --- Tests conf.d merging ---

func TestMergeConfDir_NoDirectory(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"

	if err := mergeConfDir(mainConfig); err != nil {
		t.Errorf("mergeConfDir() with no conf.d should return nil, got: %v", err)
	}
}

func TestMergeConfDir_EmptyDirectory(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	tmpDir := t.TempDir()
	if err := os.Mkdir(tmpDir+"/conf.d", 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Errorf("mergeConfDir() with empty conf.d should return nil, got: %v", err)
	}
}

func TestMergeConfDir_AlphabeticalOrder(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}

	if err := os.WriteFile(confDir+"/10-base.yaml", []byte("api:\n  port: 1111\n"), 0644); err != nil {
		t.Fatalf("Failed to write 10-base.yaml: %v", err)
	}
	if err := os.WriteFile(confDir+"/99-final.yaml", []byte("api:\n  port: 7777\n"), 0644); err != nil {
		t.Fatalf("Failed to write 99-final.yaml: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Fatalf("mergeConfDir() returned error: %v", err)
	}

	if got := viper.GetInt("api.port"); got != 7777 {
		t.Errorf("api.port = %d, want 7777 (99-final should win over 10-base)", got)
	}
}

func TestMergeConfDir_OverridesExistingValues(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	// Pre-populate viper at the "config" precedence level (same level as
	// ReadInConfig in production). Using viper.Set here would place values at
	// the "override" level, which sits above MergeConfig and would mask the
	// behavior we want to test.
	mainContent := "api:\n  port: 8018\n  enabled: true\n"
	if err := viper.ReadConfig(strings.NewReader(mainContent)); err != nil {
		t.Fatalf("Failed to seed main config: %v", err)
	}

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	if err := os.WriteFile(confDir+"/override.yaml", []byte("api:\n  port: 9999\n"), 0644); err != nil {
		t.Fatalf("Failed to write override.yaml: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Fatalf("mergeConfDir() returned error: %v", err)
	}

	if got := viper.GetInt("api.port"); got != 9999 {
		t.Errorf("api.port = %d, want 9999 (conf.d should override)", got)
	}
	// Untouched keys must survive the merge.
	if !viper.GetBool("api.enabled") {
		t.Error("api.enabled should remain true after merge")
	}
}

func TestMergeConfDir_IgnoresNonYAMLAndHidden(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")
	viper.Set("api.port", 8018)

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}

	// Files that should be ignored — if any were merged, port would change.
	if err := os.WriteFile(confDir+"/notes.txt", []byte("api:\n  port: 1111\n"), 0644); err != nil {
		t.Fatalf("Failed to write notes.txt: %v", err)
	}
	if err := os.WriteFile(confDir+"/config.json", []byte(`{"api":{"port":2222}}`), 0644); err != nil {
		t.Fatalf("Failed to write config.json: %v", err)
	}
	if err := os.WriteFile(confDir+"/.hidden.yaml", []byte("api:\n  port: 3333\n"), 0644); err != nil {
		t.Fatalf("Failed to write .hidden.yaml: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Fatalf("mergeConfDir() returned error: %v", err)
	}

	if got := viper.GetInt("api.port"); got != 8018 {
		t.Errorf("api.port = %d, want 8018 (non-YAML and hidden files must be ignored)", got)
	}
}

func TestMergeConfDir_AcceptsBothYamlAndYmlExtensions(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}

	if err := os.WriteFile(confDir+"/10-a.yaml", []byte("api:\n  port: 1111\n"), 0644); err != nil {
		t.Fatalf("Failed to write 10-a.yaml: %v", err)
	}
	if err := os.WriteFile(confDir+"/20-b.yml", []byte("logLevel: DEBUG\n"), 0644); err != nil {
		t.Fatalf("Failed to write 20-b.yml: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Fatalf("mergeConfDir() returned error: %v", err)
	}

	if got := viper.GetInt("api.port"); got != 1111 {
		t.Errorf("api.port = %d, want 1111 (.yaml file should be merged)", got)
	}
	if got := viper.GetString("logLevel"); got != "DEBUG" {
		t.Errorf("logLevel = %q, want DEBUG (.yml file should be merged)", got)
	}
}

func TestMergeConfDir_IgnoresSubdirectories(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")
	viper.Set("api.port", 8018)

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	subDir := confDir + "/subdir.yaml" // dir whose name has a YAML extension
	if err := os.MkdirAll(subDir, 0755); err != nil {
		t.Fatalf("Failed to create nested dir: %v", err)
	}

	if err := mergeConfDir(tmpDir + "/config.yaml"); err != nil {
		t.Errorf("mergeConfDir() with only a subdirectory should return nil, got: %v", err)
	}
	if got := viper.GetInt("api.port"); got != 8018 {
		t.Errorf("api.port = %d, want 8018 (subdirectory must not be parsed)", got)
	}
}

func TestMergeConfDir_InvalidYAMLFailsFast(t *testing.T) {
	viper.Reset()
	viper.SetConfigType("yaml")

	tmpDir := t.TempDir()
	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}

	badPath := confDir + "/bad.yaml"
	if err := os.WriteFile(badPath, []byte("api:\n  port: [unclosed\n"), 0644); err != nil {
		t.Fatalf("Failed to write bad.yaml: %v", err)
	}

	err := mergeConfDir(tmpDir + "/config.yaml")
	if err == nil {
		t.Fatal("mergeConfDir() with malformed YAML should return error, got nil")
	}
	if !strings.Contains(err.Error(), "bad.yaml") {
		t.Errorf("error should mention the offending file, got: %v", err)
	}
}

func TestNew_ConfDOverridesMainConfig(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	mainContent := `
api:
  port: 8018
  enabled: true
logLevel: INFO
`
	if err := os.WriteFile(mainConfig, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	override := `
api:
  port: 9999
logLevel: DEBUG
`
	if err := os.WriteFile(confDir+"/10-override.yaml", []byte(override), 0644); err != nil {
		t.Fatalf("Failed to write 10-override.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if cfg.Api.Port != 9999 {
		t.Errorf("Api.Port = %d, want 9999 (conf.d override should win)", cfg.Api.Port)
	}
	if cfg.LogLevel != logger.DEBUG {
		t.Errorf("LogLevel = %d, want %d (DEBUG from conf.d)", cfg.LogLevel, logger.DEBUG)
	}
	// Untouched key from main config must survive.
	if !cfg.Api.Enabled {
		t.Error("Api.Enabled should remain true (not set in conf.d)")
	}
}

// Array merge behavior: viper.MergeConfig replaces a slice value entirely
// rather than appending. These tests pin that contract for the systemd
// services lists so any future viper upgrade that flips the behavior is
// caught immediately.

func TestNew_ConfDSystemdServicesReplacedNotAppended(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	mainContent := `
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
  user:
    - mpd.service
    - pipewire-pulse.service
`
	if err := os.WriteFile(mainConfig, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	override := `
systemd:
  system:
    - shairport-sync.service
`
	if err := os.WriteFile(confDir+"/10-override.yaml", []byte(override), 0644); err != nil {
		t.Fatalf("Failed to write 10-override.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	want := []string{"shairport-sync.service"}
	if len(cfg.Systemd.SystemServices) != len(want) {
		t.Fatalf("SystemServices len = %d, want %d (slices are replaced, not appended). Got: %v",
			len(cfg.Systemd.SystemServices), len(want), cfg.Systemd.SystemServices)
	}
	for i, s := range want {
		if cfg.Systemd.SystemServices[i].Name != s {
			t.Errorf("SystemServices[%d] = %q, want %q", i, cfg.Systemd.SystemServices[i].Name, s)
		}
	}

	// The user services list was NOT mentioned in the override, so it must
	// keep the values from the main config.
	wantUser := []string{"mpd.service", "pipewire-pulse.service"}
	if len(cfg.Systemd.UserServices) != len(wantUser) {
		t.Fatalf("UserServices len = %d, want %d. Got: %v",
			len(cfg.Systemd.UserServices), len(wantUser), cfg.Systemd.UserServices)
	}
	for i, s := range wantUser {
		if cfg.Systemd.UserServices[i].Name != s {
			t.Errorf("UserServices[%d] = %q, want %q", i, cfg.Systemd.UserServices[i].Name, s)
		}
	}
}

func TestNew_ConfDSystemdUserServicesReplacedNotAppended(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	mainContent := `
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
  user:
    - mpd.service
    - pipewire-pulse.service
`
	if err := os.WriteFile(mainConfig, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	override := `
systemd:
  user:
    - shairport-sync.service
`
	if err := os.WriteFile(confDir+"/10-override.yaml", []byte(override), 0644); err != nil {
		t.Fatalf("Failed to write 10-override.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	want := []string{"shairport-sync.service"}
	if len(cfg.Systemd.UserServices) != len(want) {
		t.Fatalf("UserServices len = %d, want %d (slices are replaced, not appended). Got: %v",
			len(cfg.Systemd.UserServices), len(want), cfg.Systemd.UserServices)
	}
	for i, s := range want {
		if cfg.Systemd.UserServices[i].Name != s {
			t.Errorf("UserServices[%d] = %q, want %q", i, cfg.Systemd.UserServices[i].Name, s)
		}
	}

	// Symmetric to the system test: untouched system list survives.
	wantSystem := []string{"bluetooth.service", "upmpdcli.service"}
	if len(cfg.Systemd.SystemServices) != len(wantSystem) {
		t.Fatalf("SystemServices len = %d, want %d. Got: %v",
			len(cfg.Systemd.SystemServices), len(wantSystem), cfg.Systemd.SystemServices)
	}
	for i, s := range wantSystem {
		if cfg.Systemd.SystemServices[i].Name != s {
			t.Errorf("SystemServices[%d] = %q, want %q", i, cfg.Systemd.SystemServices[i].Name, s)
		}
	}
}

func TestNew_ConfDSystemdEmptyArrayClearsList(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	mainContent := `
systemd:
  enabled: true
  system:
    - bluetooth.service
    - upmpdcli.service
`
	if err := os.WriteFile(mainConfig, []byte(mainContent), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	// Explicit empty list — admin wants to wipe the inherited list.
	override := `
systemd:
  system: []
`
	if err := os.WriteFile(confDir+"/99-clear.yaml", []byte(override), 0644); err != nil {
		t.Fatalf("Failed to write 99-clear.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if len(cfg.Systemd.SystemServices) != 0 {
		t.Errorf("SystemServices = %v, want [] (explicit empty list should clear)", cfg.Systemd.SystemServices)
	}
}

func TestNew_ConfDSystemdMultipleSnippetsLastWins(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	if err := os.WriteFile(mainConfig, []byte("systemd:\n  enabled: true\n  system: [a.service]\n"), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	if err := os.WriteFile(confDir+"/10-first.yaml", []byte("systemd:\n  system: [b.service, c.service]\n"), 0644); err != nil {
		t.Fatalf("Failed to write 10-first.yaml: %v", err)
	}
	if err := os.WriteFile(confDir+"/20-second.yaml", []byte("systemd:\n  system: [d.service]\n"), 0644); err != nil {
		t.Fatalf("Failed to write 20-second.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	want := []string{"d.service"}
	if len(cfg.Systemd.SystemServices) != len(want) {
		t.Fatalf("SystemServices len = %d, want %d. Got: %v",
			len(cfg.Systemd.SystemServices), len(want), cfg.Systemd.SystemServices)
	}
	for i, s := range want {
		if cfg.Systemd.SystemServices[i].Name != s {
			t.Errorf("SystemServices[%d] = %q, want %q", i, cfg.Systemd.SystemServices[i].Name, s)
		}
	}
}

func TestNew_ConfDAlphabeticalOrderEndToEnd(t *testing.T) {
	viper.Reset()

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	if err := os.WriteFile(mainConfig, []byte("api:\n  port: 8018\n"), 0644); err != nil {
		t.Fatalf("Failed to write main config: %v", err)
	}

	confDir := tmpDir + "/conf.d"
	if err := os.Mkdir(confDir, 0755); err != nil {
		t.Fatalf("Failed to create conf.d: %v", err)
	}
	if err := os.WriteFile(confDir+"/10-base.yaml", []byte("api:\n  port: 1111\n"), 0644); err != nil {
		t.Fatalf("Failed to write 10-base.yaml: %v", err)
	}
	if err := os.WriteFile(confDir+"/99-final.yaml", []byte("api:\n  port: 7777\n"), 0644); err != nil {
		t.Fatalf("Failed to write 99-final.yaml: %v", err)
	}

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(&mainConfig)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}

	if cfg.Api.Port != 7777 {
		t.Errorf("Api.Port = %d, want 7777 (last alphabetical conf.d wins)", cfg.Api.Port)
	}
}

func TestNew_BluetoothPowerOnStartDisabledByDefault(t *testing.T) {
	viper.Reset()

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if cfg.Bluetooth == nil {
		t.Fatal("Bluetooth config should not be nil")
	}
	if cfg.Bluetooth.PowerOnStart {
		t.Error("Bluetooth.PowerOnStart should be false by default")
	}
}

func TestNew_BluetoothPowerOnStartExplicitlyEnabled(t *testing.T) {
	viper.Reset()
	viper.Set("bluetooth.poweronstart", true)

	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_SESSION_DESKTOP", "test-desktop")

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New(nil) returned error: %v", err)
	}

	if !cfg.Bluetooth.PowerOnStart {
		t.Error("Bluetooth.PowerOnStart should be true when explicitly enabled")
	}
}
