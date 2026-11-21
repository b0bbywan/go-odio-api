package config

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestNew_ProcessingDefaults(t *testing.T) {
	isolate(t)
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")

	cfg := mustNew(t, nil)

	want := &ProcessingConfig{
		Mono:             &MonoConfig{},
		NetworkSink:      &NetworkSinkConfig{Port: 4713, ACL: true},
		ZeroconfDiscover: &ZeroconfDiscoverConfig{},
		LedgerFile:       "/run/user/1000/odio-api/processing.json",
	}
	if !reflect.DeepEqual(cfg.Pulseaudio.Processing, want) {
		t.Errorf("Processing = %+v, want %+v", cfg.Pulseaudio.Processing, want)
	}
}

func TestNew_ProcessingFromConfigFile(t *testing.T) {
	isolate(t)
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")

	configFile := t.TempDir() + "/config.yaml"
	writeFile(t, configFile, `
pulseaudio:
  processing:
    mono:
      enabled: true
      onBoot: true
    networkSink:
      enabled: true
      port: 4714
      acl: false
      extraAcl:
        - 10.8.0.0/24
        - 192.168.2.5
    zeroconfDiscover:
      enabled: true
      onBoot: true
`)

	cfg := mustNew(t, &configFile)

	want := &ProcessingConfig{
		Mono: &MonoConfig{Enabled: true, OnBoot: true},
		NetworkSink: &NetworkSinkConfig{
			Enabled:  true,
			Port:     4714,
			ExtraACL: []string{"10.8.0.0/24", "192.168.2.5"},
		},
		ZeroconfDiscover: &ZeroconfDiscoverConfig{Enabled: true, OnBoot: true},
		LedgerFile:       "/run/user/1000/odio-api/processing.json",
	}
	if !reflect.DeepEqual(cfg.Pulseaudio.Processing, want) {
		t.Errorf("Processing = %+v, want %+v", cfg.Pulseaudio.Processing, want)
	}
}

func TestNew_ProcessingInterfacesFollowBind(t *testing.T) {
	isolate(t)
	viper.Set("bind", "all")

	cfg := mustNew(t, nil)

	if !reflect.DeepEqual(cfg.Pulseaudio.Processing.NetworkSink.Interfaces, cfg.Zeroconf.Listen) {
		t.Errorf("Interfaces = %v, want the zeroconf ones %v",
			cfg.Pulseaudio.Processing.NetworkSink.Interfaces, cfg.Zeroconf.Listen)
	}
}

func TestNew_ProcessingInvalid(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  any
	}{
		{"port zero", "pulseaudio.processing.networkSink.port", 0},
		{"port too high", "pulseaudio.processing.networkSink.port", 65536},
		{"extraAcl not an address", "pulseaudio.processing.networkSink.extraAcl", []string{"10.8.0.0/24", "lan"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			isolate(t)
			viper.Set(tt.key, tt.val)

			if _, err := New(nil); err == nil {
				t.Errorf("expected error for %s=%v", tt.key, tt.val)
			}
		})
	}
}

func TestValidateACL(t *testing.T) {
	valid := [][]string{
		nil,
		{"192.168.1.5"},
		{"10.8.0.0/24", "192.168.0.0/16"},
		{"fd00::/8", "::1"},
	}
	for _, acl := range valid {
		if err := validateACL(acl); err != nil {
			t.Errorf("validateACL(%q) error: %v", acl, err)
		}
	}

	invalid := []string{"lan", "10.8.0.0/33", "192.168.1.0/24,10.0.0.0/8", "10.8.0.0/24;192.168.2.5", "300.1.1.1"}
	for _, entry := range invalid {
		if err := validateACL([]string{"10.8.0.0/24", entry}); err == nil {
			t.Errorf("validateACL(%q) should fail", entry)
		}
	}
}
