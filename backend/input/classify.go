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
		return name, rcKind(inputDev), true
	case isGPIOKeys(linkBase(filepath.Join(parent, "driver"))):
		return name, KindGPIO, true
	}
	return name, "", false
}

// rcKind tells an HDMI-CEC adapter from an infrared receiver: the kernel
// exposes both as rc devices, the bus type sets them apart.
func rcKind(inputDev string) Kind {
	raw, err := os.ReadFile(filepath.Join(inputDev, "id", "bustype"))
	if err != nil {
		return KindIR
	}
	if bus, err := strconv.ParseUint(strings.TrimSpace(string(raw)), 16, 16); err == nil && bus == busCEC {
		return KindCEC
	}
	return KindIR
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
