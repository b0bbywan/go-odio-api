package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	AppName     = "odio-api"
	serviceType = "_http._tcp"
	domain      = "local."
)

// AppVersion is set at build time via -ldflags "-X github.com/b0bbywan/go-odio-api/config.AppVersion=x.y.z"
var AppVersion = "dev"

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

type UIConfig struct {
	Enabled bool
}

type SSEConfig struct {
	Enabled bool
}

type CORSConfig struct {
	Origins []string // allowed origins; ["*"] for wildcard
}

type ApiConfig struct {
	Enabled bool
	Listens []string
	Port    int

	UI   *UIConfig
	SSE  *SSEConfig
	CORS *CORSConfig // nil = CORS disabled
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
	ServeCookie   bool
}

type SystemdService struct {
	Name string
	URL  string
}

type SystemdConfig struct {
	Enabled        bool
	SystemServices []SystemdService
	UserServices   []SystemdService
	SupportsUTMP   bool
	XDGRuntimeDir  string
	Timeout        time.Duration
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

// resolveIfaceToIP returns the IPv4 address of a single named interface.
func resolveIfaceToIP(bind string) (string, error) {
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

// resolveBindsToListens converts a list of bind names to host:port listen addresses.
// "all" expands to 0.0.0.0. No implicit addresses are added.
func resolveBindsToListens(binds []string, port string) ([]string, error) {
	for _, b := range binds {
		if b == "all" {
			return []string{net.JoinHostPort("0.0.0.0", port)}, nil
		}
	}

	seen := map[string]bool{}
	var addrs []string

	for _, bind := range binds {
		ip, err := resolveIfaceToIP(bind)
		if err != nil {
			return nil, err
		}
		addr := net.JoinHostPort(ip, port)
		if !seen[addr] {
			seen[addr] = true
			addrs = append(addrs, addr)
		}
	}

	return addrs, nil
}

// hasLoopback returns true if listens contains 127.0.0.1:port or 0.0.0.0:port.
func hasLoopback(listens []string, port string) bool {
	loopback := net.JoinHostPort("127.0.0.1", port)
	wildcard := net.JoinHostPort("0.0.0.0", port)
	for _, l := range listens {
		if l == loopback || l == wildcard {
			return true
		}
	}
	return false
}

// getZeroconfInterfaces returns the network interfaces on which mDNS should be announced.
func getZeroconfInterfaces(binds []string) []net.Interface {
	for _, b := range binds {
		if b == "all" {
			return getAllActiveNonLoopback()
		}
	}

	var result []net.Interface
	for _, bind := range binds {
		if bind == "lo" {
			continue
		}
		iface, err := net.InterfaceByName(bind)
		if err != nil {
			logger.Warn("[config] interface %q not found: %v", bind, err)
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		result = append(result, *iface)
	}
	return result
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

func getDuration(key string, fallback time.Duration) time.Duration {
	if d := viper.GetDuration(key); d > 0 {
		return d
	}
	return fallback
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

// parseSystemdServices accepts viper's raw value for a service list and
// supports two YAML shapes interchangeably within the same list:
//   - bare string  →  SystemdService{Name: s}
//   - object       →  SystemdService{Name: name, URL: url}
//
// Tolerates the slice flavors viper may return (after a YAML parse: []any with
// string and map[string]any entries; after viper.Set in tests: []string).
func parseSystemdServices(raw any) ([]SystemdService, error) {
	if raw == nil {
		return nil, nil
	}

	switch slice := raw.(type) {
	case []string:
		out := make([]SystemdService, 0, len(slice))
		for i, name := range slice {
			if name == "" {
				return nil, fmt.Errorf("entry %d: empty service name", i)
			}
			out = append(out, SystemdService{Name: name})
		}
		return out, nil
	case []any:
		out := make([]SystemdService, 0, len(slice))
		for i, e := range slice {
			svc, err := parseSystemdServiceEntry(e)
			if err != nil {
				return nil, fmt.Errorf("entry %d: %w", i, err)
			}
			out = append(out, svc)
		}
		return out, nil
	default:
		return nil, fmt.Errorf("expected list, got %T", raw)
	}
}

func parseSystemdServiceEntry(e any) (SystemdService, error) {
	switch v := e.(type) {
	case string:
		if v == "" {
			return SystemdService{}, fmt.Errorf("empty service name")
		}
		return SystemdService{Name: v}, nil
	case map[string]any:
		var svc SystemdService
		if err := mapstructure.Decode(v, &svc); err != nil {
			return SystemdService{}, err
		}
		if svc.Name == "" {
			return SystemdService{}, fmt.Errorf("missing or empty 'name' field")
		}
		return svc, nil
	default:
		return SystemdService{}, fmt.Errorf("expected string or {name, url} object, got %T", e)
	}
}

func mergeConfDir(mainConfigPath string) error {
	confDir := filepath.Join(filepath.Dir(mainConfigPath), "conf.d")
	entries, err := os.ReadDir(confDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("cannot read conf.d directory %s: %w", confDir, err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".yaml" && ext != ".yml" {
			continue
		}
		files = append(files, filepath.Join(confDir, e.Name()))
	}
	sort.Strings(files)

	for _, path := range files {
		f, err := os.Open(path)
		if err != nil {
			return fmt.Errorf("cannot open conf.d snippet %s: %w", path, err)
		}
		mergeErr := viper.MergeConfig(f)
		if closeErr := f.Close(); closeErr != nil {
			logger.Warn("[%s] failed to close conf.d snippet %s: %v", AppName, path, closeErr)
		}
		if mergeErr != nil {
			return fmt.Errorf("failed to merge conf.d snippet %s: %w", path, mergeErr)
		}
		logger.Info("[%s] merged config snippet: %s", AppName, path)
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
	viper.SetDefault("api.cors.origins", []string{"https://odio-pwa.vercel.app", "https://pwa.odio.love"})
	viper.SetDefault("api.ui.enabled", true)
	viper.SetDefault("api.sse.enabled", true)

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
	viper.SetDefault("pulseaudio.serve_cookie", false)

	viper.SetDefault("systemd.enabled", false)
	viper.SetDefault("systemd.system", []string{})
	viper.SetDefault("systemd.user", []string{})
	viper.SetDefault("systemd.timeout", "90s")

	viper.SetDefault("zeroconf.enabled", true)

	// Load from configuration file, environment variables, and CLI flags
	viper.SetConfigType("yaml") // config file format

	if err := readConfig(cfgFile); err != nil {
		if _, isNotFound := err.(viper.ConfigFileNotFoundError); !isNotFound {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
	}

	if used := viper.ConfigFileUsed(); used != "" {
		if err := mergeConfDir(used); err != nil {
			return nil, err
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

	// bind accepts a single interface name or a list: "enp2s0", ["enp2s0","wlan0"], "all"
	binds := viper.GetStringSlice("bind")
	portStr := strconv.Itoa(port)
	listens, err := resolveBindsToListens(binds, portStr)
	if err != nil {
		return nil, err
	}

	uiCfg := UIConfig{
		Enabled: viper.GetBool("api.ui.enabled"),
	}

	if uiCfg.Enabled && !hasLoopback(listens, portStr) {
		logger.Error("[config] UI is enabled but 'lo' is not in bind config — UI disabled")
		uiCfg.Enabled = false
	}

	sseCfg := SSEConfig{
		Enabled: viper.GetBool("api.sse.enabled"),
	}

	apiCfg := ApiConfig{
		Enabled: viper.GetBool("api.enabled"),
		Listens: listens,
		Port:    port,
		UI:      &uiCfg,
		SSE:     &sseCfg,
	}

	if origins := viper.GetStringSlice("api.cors.origins"); len(origins) > 0 {
		apiCfg.CORS = &CORSConfig{Origins: origins}
	}

	loginCapabilities := Login1Capabilities{
		CanReboot:   viper.GetBool("power.capabilities.reboot"),
		CanPoweroff: viper.GetBool("power.capabilities.poweroff"),
	}

	logincfg := Login1Config{
		Enabled:      viper.GetBool("power.enabled"),
		Capabilities: &loginCapabilities,
	}

	mpriscfg := MPRISConfig{
		Enabled: viper.GetBool("mpris.enabled"),
		Timeout: getDuration("mpris.timeout", 5*time.Second),
	}

	bluetoothcfg := BluetoothConfig{
		Enabled:        viper.GetBool("bluetooth.enabled"),
		Timeout:        getDuration("bluetooth.timeout", 5*time.Second),
		PairingTimeout: getDuration("bluetooth.pairingtimeout", 60*time.Second),
		IdleTimeout:    getDuration("bluetooth.idletimeout", 30*time.Minute),
	}

	pulsecfg := PulseAudioConfig{
		Enabled:       viper.GetBool("pulseaudio.enabled"),
		XDGRuntimeDir: xdgRuntimeDir,
		ServeCookie:   viper.GetBool("pulseaudio.serve_cookie"),
	}

	sysServices, err := parseSystemdServices(viper.Get("systemd.system"))
	if err != nil {
		return nil, fmt.Errorf("invalid systemd.system: %w", err)
	}
	userServices, err := parseSystemdServices(viper.Get("systemd.user"))
	if err != nil {
		return nil, fmt.Errorf("invalid systemd.user: %w", err)
	}
	syscfg := SystemdConfig{
		Enabled:        viper.GetBool("systemd.enabled"),
		SystemServices: sysServices,
		UserServices:   userServices,
		SupportsUTMP:   systemdHasUTMP(),
		XDGRuntimeDir:  xdgRuntimeDir,
		Timeout:        getDuration("systemd.timeout", 90*time.Second),
	}

	interfaces := getZeroconfInterfaces(binds)
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
