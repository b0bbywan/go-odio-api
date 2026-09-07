package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

// isolate resets viper and points HOME at a scratch directory so New never
// picks up the developer's own config files.
func isolate(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
}

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// mustNew builds a Config from cfgFile and fails the test on error.
func mustNew(t *testing.T, cfgFile *string) *Config {
	t.Helper()
	cfg, err := New(cfgFile)
	if err != nil {
		t.Fatalf("New() returned error: %v", err)
	}
	return cfg
}
