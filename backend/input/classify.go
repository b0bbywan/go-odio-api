package input

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// busCEC is the kernel's BUS_CEC input bus type.
const busCEC = 0x1e

// classify decides from sysfs alone whether an event node is a remote to
// handle, so nodes that are not (keyboards, touchscreens, …) are never opened.
// sysDir is the node's /sys/class/input/eventN directory.
func classify(sysDir string, forced map[string]bool) (name string, kind Kind, ok bool) {
	inputDev := filepath.Join(sysDir, "device")
	if raw, err := os.ReadFile(filepath.Join(inputDev, "name")); err == nil {
		name = strings.TrimSpace(string(raw))
	}

	parent := filepath.Join(inputDev, "device")
	switch {
	case forced[strings.ToLower(name)]:
		return name, KindForced, true
	case linkBase(filepath.Join(parent, "subsystem")) == "rc":
		kind, ok := rcKind(inputDev)
		return name, kind, ok
	case isGPIOKeys(linkBase(filepath.Join(parent, "driver"))):
		return name, KindGPIO, true
	}
	return name, "", false
}

// rcKind tells an HDMI-CEC adapter from an infrared receiver: the kernel
// exposes both as rc devices, the bus type sets them apart. It reports false
// when the bus type cannot be read, rather than guess a kind.
func rcKind(inputDev string) (Kind, bool) {
	raw, err := os.ReadFile(filepath.Join(inputDev, "id", "bustype"))
	if err != nil {
		return "", false
	}
	bus, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 16, 16)
	if err != nil {
		return "", false
	}
	if bus == busCEC {
		return KindCEC, true
	}
	return KindIR, true
}

func isGPIOKeys(driver string) bool {
	return driver == "gpio-keys" || driver == "gpio-keys-polled"
}

func linkBase(path string) string {
	target, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}
