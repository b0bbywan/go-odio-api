package config

import (
	"net"
	"os"
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
		SystemServices: []string{"service1", "service2"},
		UserServices:   []string{"user-service1"},
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

func TestNew_UIDisabledByDefault(t *testing.T) {
	viper.Reset()
	t.Setenv("HOME", t.TempDir())

	cfg, err := New(nil)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	if cfg.Api.UI == nil {
		t.Fatal("Api.UI should not be nil")
	}
	if cfg.Api.UI.Enabled {
		t.Error("Api.UI.Enabled should be false by default")
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

	if len(cfg.Systemd.UserServices) != 1 || cfg.Systemd.UserServices[0] != "test.service" {
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
