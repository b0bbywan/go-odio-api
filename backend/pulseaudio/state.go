package pulseaudio

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"

	"github.com/b0bbywan/go-odio-api/logger"
)

// featureKeys are the features' config keys under pulseaudio.processing.
var featureKeys = map[featureName]string{
	featureMono:        "mono",
	featureNetworkSink: "networkSink",
}

// stateDropIn is the conf.d drop-in holding the API toggles.
type stateDropIn struct {
	PulseAudio struct {
		Processing map[string]toggle `yaml:"processing"`
	} `yaml:"pulseaudio"`
}

type toggle struct {
	Enabled bool `yaml:"enabled"`
}

// saveToggle records enabled for name in the state drop-in at path, keeping
// the other toggles; unwritten, the change lasts until restart.
func saveToggle(path string, name featureName, enabled bool) {
	if path == "" {
		logger.Warn("[pulseaudio] no config file loaded, %s change lasts until restart", name)
		return
	}
	var st stateDropIn
	data, err := os.ReadFile(path)
	switch {
	case err == nil:
		if err := yaml.Unmarshal(data, &st); err != nil {
			logger.Warn("[pulseaudio] rewriting unreadable %s: %v", path, err)
			st = stateDropIn{}
		}
	case !errors.Is(err, fs.ErrNotExist):
		logger.Warn("[pulseaudio] reading %s: %v", path, err)
	}
	if st.PulseAudio.Processing == nil {
		st.PulseAudio.Processing = map[string]toggle{}
	}
	st.PulseAudio.Processing[featureKeys[name]] = toggle{Enabled: enabled}

	if err := writeAtomic(path, st); err != nil {
		logger.Warn("[pulseaudio] %s change lasts until restart: %v", name, err)
		return
	}
	logger.Info("[pulseaudio] saved %s enabled=%t to %s", name, enabled, path)
}

func writeAtomic(path string, v any) error {
	data, err := yaml.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
