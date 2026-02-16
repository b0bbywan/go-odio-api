package backend

import (
	"bufio"
	"io"
	"os"
	"runtime"
	"strings"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	UNKNOWN         = "unknown"
	OS_RELEASE_FILE = "/etc/os-release"
)

var osVersion string

type ServerDeviceInfo struct {
	Hostname   string   `json:"hostname"`
	OSPlatform string   `json:"os_platform"`
	OSVersion  string   `json:"os_version"`
	APISW      string   `json:"api_sw"`
	APIVersion string   `json:"api_version"`
	Backends   Backends `json:"backends"`
}

type Backends struct {
	Power      bool `json:"power"`
	MPRIS      bool `json:"mpris"`
	PulseAudio bool `json:"pulseaudio"`
	Systemd    bool `json:"systemd"`
	Zeroconf   bool `json:"zeroconf"`
}

func init() {
	osVersion = readOSRelease()
}

func parseKeyValue(r io.Reader) (map[string]string, error) {
	out := make(map[string]string)

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), "=")
		if !ok {
			continue
		}
		out[key] = strings.Trim(value, `"`)
	}

	return out, scanner.Err()
}

func readOSRelease() string {
	file, err := os.Open(OS_RELEASE_FILE)
	if err != nil {
		return UNKNOWN
	}
	defer func() {
		if err := file.Close(); err != nil {
			logger.Warn("[backend] failed to close %s: %v", OS_RELEASE_FILE, err)
		}
	}()

	var content map[string]string
	content, err = parseKeyValue(file)
	if err != nil {
		logger.Debug("[backend] failed to parse %s: %v", OS_RELEASE_FILE, err)
	}

	switch {
	case content["PRETTY_NAME"] != "":
		return content["PRETTY_NAME"]
	case content["NAME"] != "":
		return content["NAME"]
	default:
		return UNKNOWN
	}
}

func (b *Backend) GetServerDeviceInfo() (ServerDeviceInfo, error) {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Debug("[backend] failed to get hostname: %v", err)
		hostname = UNKNOWN
	}

	platform := runtime.GOOS + "/" + runtime.GOARCH

	return ServerDeviceInfo{
		Hostname:   hostname,
		OSPlatform: platform,
		OSVersion:  osVersion,
		APISW:      config.AppName,
		APIVersion: config.AppVersion,
		Backends: Backends{
			Power:      b.Login1 != nil,
			MPRIS:      b.MPRIS != nil,
			PulseAudio: b.Pulse != nil,
			Systemd:    b.Systemd != nil,
			Zeroconf:   b.Zeroconf != nil,
		},
	}, nil
}
