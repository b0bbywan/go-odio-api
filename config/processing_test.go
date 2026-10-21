package config

import (
	"reflect"
	"testing"

	"github.com/spf13/viper"
)

func TestNew_ProcessingDefaults(t *testing.T) {
	isolate(t)

	cfg := mustNew(t, nil)

	want := &ProcessingConfig{
		Mono:        &MonoConfig{},
		NetworkSink: &NetworkSinkConfig{Port: 4713, ACL: true},
	}
	if !reflect.DeepEqual(cfg.Pulseaudio.Processing, want) {
		t.Errorf("Processing = %+v, want %+v", cfg.Pulseaudio.Processing, want)
	}
}

func TestNew_ProcessingFromConfigFile(t *testing.T) {
	isolate(t)

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
      extraAcl: "10.8.0.0/24; 192.168.2.5;"
`)

	cfg := mustNew(t, &configFile)

	want := &ProcessingConfig{
		Mono: &MonoConfig{Enabled: true, OnBoot: true},
		NetworkSink: &NetworkSinkConfig{
			Enabled:  true,
			Port:     4714,
			ExtraACL: []string{"10.8.0.0/24", "192.168.2.5"},
		},
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
		{"extraAcl not an address", "pulseaudio.processing.networkSink.extraAcl", "10.8.0.0/24;lan"},
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

func TestParseACL(t *testing.T) {
	valid := []struct {
		name string
		raw  string
		want []string
	}{
		{"empty", "", nil},
		{"only separators", " ; ;", nil},
		{"single IP", "192.168.1.5", []string{"192.168.1.5"}},
		{"CIDRs trimmed", " 10.8.0.0/24 ;192.168.0.0/16 ", []string{"10.8.0.0/24", "192.168.0.0/16"}},
		{"IPv6", "fd00::/8;::1", []string{"fd00::/8", "::1"}},
	}
	for _, tt := range valid {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseACL(tt.raw)
			if err != nil {
				t.Fatalf("parseACL(%q) error: %v", tt.raw, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("parseACL(%q) = %v, want %v", tt.raw, got, tt.want)
			}
		})
	}

	invalid := []string{"lan", "10.8.0.0/33", "192.168.1.0/24,10.0.0.0/8", "300.1.1.1"}
	for _, raw := range invalid {
		t.Run("invalid "+raw, func(t *testing.T) {
			if _, err := parseACL(raw); err == nil {
				t.Errorf("parseACL(%q) should fail", raw)
			}
		})
	}
}
