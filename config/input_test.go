package config

import (
	"reflect"
	"testing"
)

func TestNew_InputDisabledByDefault(t *testing.T) {
	isolate(t)

	cfg := mustNew(t, nil)

	if cfg.Input == nil {
		t.Fatal("Input config should not be nil")
	}
	if cfg.Input.Enabled {
		t.Error("Input.Enabled should be false by default")
	}
}

func TestNew_InputFromConfigFile(t *testing.T) {
	isolate(t)

	configFile := t.TempDir() + "/config.yaml"
	writeFile(t, configFile, `
input:
  enabled: true
  devices:
    - "flirc.tv flirc"
  bindings:
    - key: KEY_POWER
      action: power.poweroff
    - key: KEY_RED
      action: service.restart
      unit: kodi.service
`)

	cfg := mustNew(t, &configFile)

	want := &InputConfig{
		Enabled: true,
		Devices: []string{"flirc.tv flirc"},
		Bindings: []InputBinding{
			{Key: "KEY_POWER", Action: "power.poweroff"},
			{Key: "KEY_RED", Action: "service.restart", Unit: "kodi.service"},
		},
	}
	if !reflect.DeepEqual(cfg.Input, want) {
		t.Errorf("Input = %+v, want %+v", cfg.Input, want)
	}
}

func TestNew_InputInvalidBindings(t *testing.T) {
	isolate(t)

	configFile := t.TempDir() + "/config.yaml"
	writeFile(t, configFile, `
input:
  bindings:
    - action: player.next
`)

	if _, err := New(&configFile); err == nil {
		t.Error("expected error for a binding without key")
	}
}

func TestNew_ConfDInputBindingsReplacedNotAppended(t *testing.T) {
	isolate(t)

	tmpDir := t.TempDir()
	mainConfig := tmpDir + "/config.yaml"
	writeFile(t, mainConfig, `
input:
  bindings:
    - key: KEY_POWER
      action: power.poweroff
`)
	writeFile(t, tmpDir+"/conf.d/10-input.yaml", `
input:
  bindings:
    - key: KEY_RED
      action: player.next
`)

	cfg := mustNew(t, &mainConfig)

	want := []InputBinding{{Key: "KEY_RED", Action: "player.next"}}
	if !reflect.DeepEqual(cfg.Input.Bindings, want) {
		t.Errorf("Bindings = %+v, want %+v", cfg.Input.Bindings, want)
	}
}

func TestParseInputBindings(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		got, err := parseInputBindings(nil)
		if err != nil || got != nil {
			t.Errorf("parseInputBindings(nil) = (%v, %v), want (nil, nil)", got, err)
		}
	})

	invalid := []struct {
		name string
		raw  any
	}{
		{"top level not a list", "KEY_POWER"},
		{"entry not an object", []any{"KEY_POWER"}},
		{"missing key", []any{map[string]any{"action": "player.next"}}},
		{"missing action", []any{map[string]any{"key": "KEY_NEXT"}}},
		{"key not a string", []any{map[string]any{"key": 116, "action": "power.poweroff"}}},
	}
	for _, tt := range invalid {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := parseInputBindings(tt.raw); err == nil {
				t.Errorf("expected error for %v, got nil", tt.raw)
			}
		})
	}
}
