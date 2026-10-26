package pulseaudio

import (
	"reflect"
	"testing"

	"github.com/b0bbywan/go-odio-api/config"
)

// processingCfg is both features enabled and on at boot, the network sink on
// the default port with the derived acl.
func processingCfg() *config.ProcessingConfig {
	return &config.ProcessingConfig{
		Mono:        &config.MonoConfig{Enabled: true, OnBoot: true},
		NetworkSink: &config.NetworkSinkConfig{Enabled: true, OnBoot: true, Port: 4713, ACL: true},
	}
}

func TestBuildFeatures_OnlyEnabled(t *testing.T) {
	cfg := processingCfg()
	cfg.Mono.Enabled = false

	got := buildFeatures(cfg, ServerPulse, nil)
	if len(got) != 1 || got[0].name != featureNetworkSink {
		t.Errorf("buildFeatures = %+v, want the network sink alone", got)
	}

	cfg.NetworkSink.Enabled = false
	if got := buildFeatures(cfg, ServerPulse, nil); got != nil {
		t.Errorf("buildFeatures = %+v, want nil", got)
	}
	if got := buildFeatures(nil, ServerPulse, nil); got != nil {
		t.Errorf("buildFeatures(nil) = %+v, want nil", got)
	}
}

func TestBuildFeatures_Mono(t *testing.T) {
	cfg := processingCfg()
	cfg.Mono.OnBoot = false

	got := buildFeatures(cfg, ServerPulse, nil)[0]

	want := feature{
		name: featureMono,
		modules: []moduleSpec{{
			name: "module-remap-sink",
			args: map[string]string{
				"sink_name":       "mono",
				"channels":        "1",
				"channel_map":     "mono",
				"sink_properties": "device.description=Mono",
			},
			identity:     []string{"sink_name"},
			ignore:       []string{"master"},
			takesDefault: true,
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("mono = %+v, want %+v", got, want)
	}
}

func TestBuildFeatures_NetworkSink(t *testing.T) {
	lan := []string{"192.168.1.42/24"}
	tests := []struct {
		name     string
		kind     AudioServerKind
		addrs    []string
		edit     func(*config.NetworkSinkConfig)
		wantArgs map[string]string
		wantErr  error
	}{
		{
			name:     "derived acl",
			kind:     ServerPulse,
			addrs:    lan,
			wantArgs: map[string]string{"port": "4713", "auth-ip-acl": "127.0.0.1;192.168.1.0/24"},
		},
		{
			name:     "custom port",
			kind:     ServerPulse,
			addrs:    lan,
			edit:     func(c *config.NetworkSinkConfig) { c.Port = 4714 },
			wantArgs: map[string]string{"port": "4714", "auth-ip-acl": "127.0.0.1;192.168.1.0/24"},
		},
		{
			name:     "extra acl",
			kind:     ServerPulse,
			addrs:    lan,
			edit:     func(c *config.NetworkSinkConfig) { c.ExtraACL = []string{"10.8.0.0/24"} },
			wantArgs: map[string]string{"port": "4713", "auth-ip-acl": "127.0.0.1;192.168.1.0/24;10.8.0.0/24"},
		},
		{
			name:     "no acl",
			kind:     ServerPulse,
			addrs:    lan,
			edit:     func(c *config.NetworkSinkConfig) { c.ACL = false },
			wantArgs: map[string]string{"port": "4713", "auth-anonymous": "1"},
		},
		{
			name:     "no acl on pipewire",
			kind:     ServerPipeWire,
			addrs:    lan,
			edit:     func(c *config.NetworkSinkConfig) { c.ACL = false },
			wantArgs: map[string]string{"port": "4713", "auth-anonymous": "1"},
		},
		{
			name:     "acl on pipewire",
			kind:     ServerPipeWire,
			addrs:    lan,
			wantArgs: map[string]string{"port": "4713"},
			wantErr:  errACLUnsupported,
		},
		{
			name:     "no LAN subnet",
			kind:     ServerPulse,
			addrs:    []string{"82.64.1.2/24"},
			wantArgs: map[string]string{"port": "4713", "auth-ip-acl": "127.0.0.1"},
			wantErr:  errNoLANSubnet,
		},
		{
			name:     "extra acl without LAN subnet",
			kind:     ServerPulse,
			edit:     func(c *config.NetworkSinkConfig) { c.ExtraACL = []string{"10.8.0.0/24"} },
			wantArgs: map[string]string{"port": "4713", "auth-ip-acl": "127.0.0.1;10.8.0.0/24"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := processingCfg()
			if tt.edit != nil {
				tt.edit(cfg.NetworkSink)
			}

			got := buildFeatures(cfg, tt.kind, hostAddrs(t, tt.addrs...))[1]

			want := feature{
				name:    featureNetworkSink,
				enabled: true,
				err:     tt.wantErr,
				modules: []moduleSpec{
					{
						name:     "module-native-protocol-tcp",
						args:     tt.wantArgs,
						identity: []string{"port"},
						defaults: map[string]string{"port": "4713"},
						sets:     []string{"auth-ip-acl"},
					},
					{name: "module-zeroconf-publish", args: map[string]string{}, optional: true},
				},
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("network sink = %+v, want %+v", got, want)
			}
		})
	}
}
