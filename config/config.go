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
	Upgrade    *UpgradeConfig
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
	// Internal units are triggerable but hidden from the /services listing and
	// service.updated events. Set programmatically (e.g. by the upgrade
	// backend), never from user config.
	Internal bool
}

type SystemdConfig struct {
	Enabled        bool
	SystemServices []SystemdService
	UserServices   []SystemdService
	SupportsUTMP   bool
	XDGRuntimeDir  string
	Timeout        time.Duration
}

// UpgradeConfig drives the agnostic upgrade backend: it reads a result file
// written by an external detector and triggers external systemd user units.
type UpgradeConfig struct {
	Enabled        bool
	ResultFile     string // upgrades.json to read and watch
	CheckUnit      string // user unit to (re)run detection; empty disables /upgrade/check
	UpgradeUnit    string // user unit to run the upgrade; empty disables /upgrade/start
	ProgressSocket string // unix socket the upgrade script streams run progress to
	StateFile      string // persisted last-run verdict, in a persistent dir (survives reboot)
}

type BluetoothConfig struct {
	Enabled        bool
	PowerOnStart   bool
	PairingTimeout time.Duration
	Timeout        time.Duration
	IdleTimeout    time.Duration
	ScanTimeout    time.Duration
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
	viper.SetDefault("bluetooth.poweronstart", false)
	viper.SetDefault("bluetooth.timeout", "5s")
	viper.SetDefault("bluetooth.pairingtimeout", "60s")
	viper.SetDefault("bluetooth.idletimeout", "30m")
	viper.SetDefault("bluetooth.scantimeout", "60s")

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

	// No system-wide fallback for the state dir (unlike runtime's /run/user/{uid}); leave it
	// empty when neither XDG_STATE_HOME nor HOME is set, so we never build a relative path.
	stateHome := os.Getenv("XDG_STATE_HOME")
	if stateHome == "" {
		if home := os.Getenv("HOME"); home != "" {
			stateHome = filepath.Join(home, ".local", "state")
		}
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
		PowerOnStart:   viper.GetBool("bluetooth.poweronstart"),
		Timeout:        getDuration("bluetooth.timeout", 5*time.Second),
		PairingTimeout: getDuration("bluetooth.pairingtimeout", 60*time.Second),
		IdleTimeout:    getDuration("bluetooth.idletimeout", 30*time.Minute),
		ScanTimeout:    getDuration("bluetooth.scantimeout", 60*time.Second),
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

	// Progress streams over a socket, not a file, to avoid SD-card writes; default
	// it into the tmpfs runtime dir.
	progressSocket := viper.GetString("upgrade.progressSocket")
	if progressSocket == "" {
		progressSocket = filepath.Join(xdgRuntimeDir, "odio-api", "upgrade.sock")
	}
	// The last-run verdict must survive reboot, so it lives in the persistent state
	// dir rather than the tmpfs runtime dir; one write per run, so SD wear is moot.
	stateFile := viper.GetString("upgrade.stateFile")
	if stateFile == "" && stateHome != "" {
		stateFile = filepath.Join(stateHome, "odio-api", "upgrade-run.json")
	}
	upgradecfg := UpgradeConfig{
		Enabled:        viper.GetBool("upgrade.enabled"),
		ResultFile:     viper.GetString("upgrade.resultFile"),
		CheckUnit:      viper.GetString("upgrade.checkUnit"),
		UpgradeUnit:    viper.GetString("upgrade.upgradeUnit"),
		ProgressSocket: progressSocket,
		StateFile:      stateFile,
	}
	if upgradecfg.Enabled && upgradecfg.StateFile == "" {
		logger.Warn("[config] upgrade.stateFile unset and no state dir (XDG_STATE_HOME/HOME); last-run verdict will not persist")
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
		Upgrade:    &upgradecfg,
		Zeroconf:   &zerocfg,
		LogLevel:   parseLogLevel(viper.GetString("LogLevel")),
	}

	return &cfg, nil
}
