package input

import (
	"os"
	"path/filepath"
	"strings"
)

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
		return name, KindIR, true
	case isGPIOKeys(linkBase(filepath.Join(parent, "driver"))):
		return name, KindGPIO, true
	}
	return name, "", false
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
