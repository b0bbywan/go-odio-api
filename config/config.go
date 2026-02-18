package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	AppName     = "odio-api"
	AppVersion  = "0.5.1"
	serviceType = "_http._tcp"
	domain      = "local."
)

type Config struct {
	Api        *ApiConfig
	Bluetooth  *BluetoothConfig
	Login1     *Login1Config
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

type Login1Capabilities struct {
	CanPoweroff bool
	CanReboot   bool
}

type Login1Config struct {
	Enabled      bool
	Capabilities *Login1Capabilities
}

type MPRISConfig struct {
	Enabled bool
	Timeout time.Duration
}

type PulseAudioConfig struct {
	Enabled       bool
	XDGRuntimeDir string
}

type SystemdConfig struct {
	Enabled        bool
	SystemServices []string
	UserServices   []string
	SupportsUTMP   bool
	XDGRuntimeDir  string
}

type BluetoothConfig struct {
	Enabled        bool
	PairingTimeout time.Duration
	Timeout        time.Duration
	IdleTimeout    time.Duration
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

// resolveBindToIP convertit bind (interface name ou "all") en IP pour l'API
func resolveBindToIP(bind string) (string, error) {
	if bind == "all" {
		return "0.0.0.0", nil
	}

	iface, err := net.InterfaceByName(bind)
	if err != nil {
		return "", fmt.Errorf("interface %q not found", bind)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 on interface %s", bind)
}

// getZeroconfInterfaces retourne les interfaces pour zeroconf
func getZeroconfInterfaces(bind string) []net.Interface {
	if bind == "all" {
		return getAllActiveNonLoopback()
	}

	iface, err := net.InterfaceByName(bind)
	if err != nil {
		logger.Warn("[config] interface %q not found: %v", bind, err)
		return nil
	}
	if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
		return nil
	}
	return []net.Interface{*iface}
}

// getAllActiveNonLoopback retourne toutes interfaces UP sauf loopback
func getAllActiveNonLoopback() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var result []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			result = append(result, iface)
		}
	}
	return result
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

	viper.SetDefault("bind", "lo")
	viper.SetDefault("LogLevel", "INFO")

	viper.SetDefault("api.enabled", true)
	viper.SetDefault("api.port", 8018)

	viper.SetDefault("bluetooth.enabled", true)
	viper.SetDefault("bluetooth.timeout", "5s")
	viper.SetDefault("bluetooth.pairingtimeout", "60s")
	viper.SetDefault("bluetooth.idletimeout", "30m")

	viper.SetDefault("power.enabled", false)
	viper.SetDefault("power.capabilities.reboot", false)
	viper.SetDefault("power.capabilities.poweroff", false)

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

	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		xdgRuntimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}

	port := viper.GetInt("api.port")
	if port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", port)
	}
	bind := viper.GetString("bind")
	listenIP, err := resolveBindToIP(bind)
	if err != nil {
		return nil, err
	}

	apiCfg := ApiConfig{
		Enabled: viper.GetBool("api.enabled"),
		Listen:  net.JoinHostPort(listenIP, strconv.Itoa(port)),
		Port:    port,
	}

	loginCapabilities := Login1Capabilities{
		CanReboot:   viper.GetBool("power.capabilities.reboot"),
		CanPoweroff: viper.GetBool("power.capabilities.poweroff"),
	}

	logincfg := Login1Config{
		Enabled:      viper.GetBool("power.enabled"),
		Capabilities: &loginCapabilities,
	}

	mprisTimeout := viper.GetDuration("mpris.timeout")
	if mprisTimeout <= 0 {
		mprisTimeout = 5 * time.Second
	}

	mpriscfg := MPRISConfig{
		Enabled: viper.GetBool("mpris.enabled"),
		Timeout: mprisTimeout,
	}

	bluetoothTimeout := viper.GetDuration("bluetooth.timeout")
	if bluetoothTimeout <= 0 {
		bluetoothTimeout = 5 * time.Second
	}

	bluetoothPairingTimeout := viper.GetDuration("bluetooth.pairingtimeout")
	if bluetoothPairingTimeout <= 0 {
		bluetoothPairingTimeout = 60 * time.Second
	}

	bluetoothIdleTimeout := viper.GetDuration("bluetooth.idletimeout")
	if bluetoothIdleTimeout <= 0 {
		bluetoothIdleTimeout = 30 * time.Minute
	}

	bluetoothcfg := BluetoothConfig{
		Enabled:        viper.GetBool("bluetooth.enabled"),
		Timeout:        bluetoothTimeout,
		PairingTimeout: bluetoothPairingTimeout,
		IdleTimeout:    bluetoothIdleTimeout,
	}

	pulsecfg := PulseAudioConfig{
		Enabled:       viper.GetBool("pulseaudio.enabled"),
		XDGRuntimeDir: xdgRuntimeDir,
	}

	syscfg := SystemdConfig{
		Enabled:        viper.GetBool("systemd.enabled"),
		SystemServices: viper.GetStringSlice("systemd.system"),
		UserServices:   viper.GetStringSlice("systemd.user"),
		SupportsUTMP:   systemdHasUTMP(),
		XDGRuntimeDir:  xdgRuntimeDir,
	}

	interfaces := getZeroconfInterfaces(bind)
	zerocfg := ZeroConfig{
		Enabled:      viper.GetBool("zeroconf.enabled"),
		InstanceName: AppName,
		ServiceType:  serviceType,
		Port:         port,
		Domain:       domain,
		TxtRecords:   []string{"version=" + AppVersion},
		Listen:       interfaces,
	}

	cfg := Config{
		Api:        &apiCfg,
		Bluetooth:  &bluetoothcfg,
		Login1:     &logincfg,
		MPRIS:      &mpriscfg,
		Pulseaudio: &pulsecfg,
		Systemd:    &syscfg,
		Zeroconf:   &zerocfg,
		LogLevel:   parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
