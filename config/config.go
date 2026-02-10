package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	AppName     = "odio-api"
	AppVersion  = "0.5.0"
	serviceType = "_http._tcp"
	domain      = "local."
)

type Config struct {
	Api        *ApiConfig
	Bluetooth  *BluetoothConfig
	MPRIS      *MPRISConfig
	Pulseaudio *PulseAudioConfig
	Systemd    *SystemdConfig
	Zeroconf   *ZeroConfig
	LogLevel   logger.Level
}

type ApiConfig struct {
	Enabled bool
	Port    int
	Listen  string
}

type SystemdConfig struct {
	Enabled        bool
	SystemServices []string
	UserServices   []string
	SupportsUTMP   bool
	XDGRuntimeDir  string
}

type PulseAudioConfig struct {
	Enabled       bool
	XDGRuntimeDir string
}

type MPRISConfig struct {
	Enabled bool
	Timeout time.Duration
}

type ZeroConfig struct {
	Enabled      bool
	InstanceName string
	ServiceType  string
	Domain       string
	Port         int
	TxtRecords   []string
	Listen       []net.Interface
}

type BluetoothConfig struct {
	Enabled        bool
	PairingTimeout time.Duration
	Timeout        time.Duration
}

// parseLogLevel converts a string to a logger.Level
func parseLogLevel(levelStr string) logger.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return logger.DEBUG
	case "INFO":
		return logger.INFO
	case "WARN":
		return logger.WARN
	case "ERROR":
		return logger.ERROR
	case "FATAL":
		return logger.FATAL
	default:
		return logger.WARN // default
	}
}

func interfaceForIP(ip string) (*net.Interface, error) {
	if ip == "127.0.0.1" {
		return nil, nil
	}
	listenIP := net.ParseIP(ip)
	if listenIP == nil {
		return nil, fmt.Errorf("invalid bind: %s", ip)
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	for _, iface := range ifaces {
		addrs, _ := iface.Addrs()
		for _, addr := range addrs {
			var ifaceIP net.IP

			switch v := addr.(type) {
			case *net.IPNet:
				ifaceIP = v.IP
			case *net.IPAddr:
				ifaceIP = v.IP
			}

			if ifaceIP != nil && ifaceIP.Equal(listenIP) {
				return &iface, nil
			}
		}
	}

	return nil, fmt.Errorf("no interface found for IP %s", ip)
}

func systemdHasUTMP() bool {
	_, err := os.Stat("/run/utmp")
	return err == nil
}

func validateConfigPath(path string) error {
	// Check file extension
	ext := filepath.Ext(path)
	if ext != ".yaml" && ext != ".yml" {
		return fmt.Errorf("config file must be .yaml or .yml, got: %s", ext)
	}

	// Check file exists and is readable
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("cannot access config file: %w", err)
	}

	// Check it's not a directory
	if info.IsDir() {
		return fmt.Errorf("config path is a directory, not a file: %s", path)
	}

	return nil
}

func readConfig(cfgFile *string) error {
	if cfgFile != nil && *cfgFile != "" {
		if err := validateConfigPath(*cfgFile); err != nil {
			return err
		}
		viper.SetConfigFile(*cfgFile)
		return viper.ReadInConfig()
	}

	viper.SetConfigName("config")                       // name of config file (without extension)
	viper.AddConfigPath(filepath.Join("/etc", AppName)) // Global configuration path
	if home, err := os.UserHomeDir(); err == nil {
		viper.AddConfigPath(filepath.Join(home, ".config", AppName)) // User config path
	}
	return viper.ReadInConfig()
}

func New(cfgFile *string) (*Config, error) {
	viper.SetDefault("bind", "127.0.0.1")
	viper.SetDefault("LogLevel", "WARN")

	viper.SetDefault("api.enabled", true)
	viper.SetDefault("api.port", 8018)

	viper.SetDefault("mpris.enabled", true)
	viper.SetDefault("mpris.timeout", "5s")

	viper.SetDefault("pulseaudio.enabled", true)

	viper.SetDefault("systemd.enabled", false)
	viper.SetDefault("systemd.system", []string{})
	viper.SetDefault("systemd.user", []string{})

	viper.SetDefault("zeroconf.enabled", true)

	// Load from configuration file, environment variables, and CLI flags
	viper.SetConfigType("yaml") // config file format

	if err := readConfig(cfgFile); err != nil {
		// If user explicitly provided a config file, fail hard on any error
		if cfgFile != nil && *cfgFile != "" {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		// Otherwise, only warn for non-file-not-found errors
		if _, isNotFound := err.(viper.ConfigFileNotFoundError); !isNotFound {
			logger.Warn("failed to read config: %v", err)
		}
	}

	port := viper.GetInt("api.port")
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}

	bind := viper.GetString("bind")
	var interfaces []net.Interface
	inet, err := interfaceForIP(bind)
	if err == nil && inet != nil {
		interfaces = append(interfaces, *inet)
	}

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	apiCfg := ApiConfig{
		Enabled: viper.GetBool("api.enabled"),
		Listen:  fmt.Sprintf("%s:%d", bind, port),
		Port:    port,
	}

	syscfg := SystemdConfig{
		Enabled:        viper.GetBool("systemd.enabled"),
		SystemServices: viper.GetStringSlice("systemd.system"),
		UserServices:   viper.GetStringSlice("systemd.user"),
		SupportsUTMP:   systemdHasUTMP(),
		XDGRuntimeDir:  xdgRuntimeDir,
	}

	pulsecfg := PulseAudioConfig{
		Enabled:       viper.GetBool("pulseaudio.enabled"),
		XDGRuntimeDir: xdgRuntimeDir,
	}

	mprisTimeout := viper.GetDuration("mpris.timeout")
	if mprisTimeout <= 0 {
		mprisTimeout = 5 * time.Second
	}

	mpriscfg := MPRISConfig{
		Enabled: viper.GetBool("mpris.enabled"),
		Timeout: mprisTimeout,
	}

	zerocfg := ZeroConfig{
		Enabled:      viper.GetBool("zeroconf.enabled"),
		InstanceName: AppName,
		ServiceType:  serviceType,
		Port:         port,
		Domain:       domain,
		TxtRecords:   []string{"version=" + AppVersion},
		Listen:       interfaces,
	}

	bluetoothTimeout := viper.GetDuration("bluetooth.timeout")
	if bluetoothTimeout <= 0 {
		bluetoothTimeout = 5 * time.Second
	}

	bluetoothPairingTimeout := viper.GetDuration("bluetooth.pairingtimeout")
	if bluetoothPairingTimeout <= 0 {
		bluetoothPairingTimeout = 60 * time.Second
	}

	bluetoothcfg := BluetoothConfig{
		Enabled:        viper.GetBool("bluetooth.enabled"),
		Timeout:        bluetoothTimeout,
		PairingTimeout: bluetoothPairingTimeout,
	}

	cfg := Config{
		Api:        &apiCfg,
		Bluetooth:  &bluetoothcfg,
		MPRIS:      &mpriscfg,
		Pulseaudio: &pulsecfg,
		Systemd:    &syscfg,
		Zeroconf:   &zerocfg,
		LogLevel:   parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
