package pulseaudio

import (
	"reflect"
	"slices"
	"testing"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/config"
)

func module(index uint32, name, args string) *proto.GetModuleInfoReply {
	return &proto.GetModuleInfoReply{ModuleIndex: index, ModuleName: name, ModuleArgs: args}
}

func unload(index uint32, name string) moduleAction {
	return moduleAction{kind: actionUnload, module: name, index: index}
}

const (
	tcpModule      = "module-native-protocol-tcp"
	zeroconfModule = "module-zeroconf-publish"
	remapModule    = "module-remap-sink"

	// as loaded by odios pulse-tcp.sh on a 192.168.1.x LAN
	legacyTCPArgs = "auth-ip-acl=127.0.0.1;192.168.1.0/24"
	// as loaded by the pulse-mono.sh workaround of odios discussion #86
	workaroundMonoArgs = "master=" + workaroundMaster + " sink_name=mono sink_properties=device.description=Mono channels=1 channel_map=mono"
)

const workaroundMaster = "alsa_output.platform-3f00b840.mailbox.stereo-fallback"

var (
	loadTCP = moduleAction{
		kind:   actionLoad,
		module: tcpModule,
		args:   "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713",
	}
	loadZeroconf = moduleAction{kind: actionLoad, module: zeroconfModule, optional: true}
	// on the default sink, which it then takes
	loadMono = []moduleAction{
		{kind: actionLoad, module: remapModule, args: monoArgs},
		{kind: actionSetDefault, sink: "mono"},
	}
	restoreMaster = moduleAction{kind: actionSetDefault, sink: workaroundMaster, optional: true}
)

// infrastructure modules from a PulseAudio 17.0 module list, never odio's
var infraModules = []*proto.GetModuleInfoReply{
	module(1, "module-stream-restore", "restore_device=false"),
	module(6, "module-alsa-card", `device_id="0" name="platform-3f00b840.mailbox" card_name="alsa_card.platform-3f00b840.mailbox" namereg_fail=false tsched=no card_properties="module-udev-detect.discovered=1"`),
	module(11, "module-native-protocol-unix", ""),
}

func TestPlan(t *testing.T) {
	tests := []struct {
		name        string
		edit        func(*config.ProcessingConfig)
		kind        AudioServerKind
		def         string // default sink, the workaround master if empty
		loaded      []*proto.GetModuleInfoReply
		wantMono    []moduleAction
		wantNetwork []moduleAction
	}{
		{
			name:        "nothing loaded",
			wantMono:    loadMono,
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name: "legacy modules adopted",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, legacyTCPArgs),
				module(22, zeroconfModule, ""),
				module(30, remapModule, workaroundMonoArgs),
			},
		},
		{
			name: "acl in another order",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "auth-ip-acl=192.168.1.0/24;127.0.0.1"),
				module(22, zeroconfModule, ""),
			},
			wantMono: loadMono,
		},
		{
			name: "explicit default port",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "port=4713 "+legacyTCPArgs),
				module(22, zeroconfModule, ""),
			},
			wantMono: loadMono,
		},
		{
			name: "acl drift reloads",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "auth-ip-acl=127.0.0.1;192.168.0.0/24"),
				module(22, zeroconfModule, ""),
			},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{unload(21, tcpModule), loadTCP},
		},
		{
			name: "unknown key is a drift",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "listen=0.0.0.0 "+legacyTCPArgs),
				module(22, zeroconfModule, ""),
			},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{unload(21, tcpModule), loadTCP},
		},
		{
			name: "mono drift reloads",
			loaded: []*proto.GetModuleInfoReply{
				module(30, remapModule, "sink_name=mono channels=2 channel_map=mono sink_properties=device.description=Mono"),
			},
			wantMono:    slices.Concat([]moduleAction{unload(30, remapModule)}, loadMono),
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name: "mono drift reloads on its master",
			def:  "mono",
			loaded: []*proto.GetModuleInfoReply{
				module(30, remapModule, "master="+workaroundMaster+" sink_name=mono channels=2"),
			},
			wantMono:    slices.Concat([]moduleAction{restoreMaster, unload(30, remapModule)}, loadMono),
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name: "disabled features unloaded",
			def:  "mono",
			edit: func(c *config.ProcessingConfig) {
				c.Mono.Enabled = false
				c.NetworkSink.Enabled = false
			},
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, legacyTCPArgs),
				module(22, zeroconfModule, ""),
				module(30, remapModule, workaroundMonoArgs),
			},
			wantMono:    []moduleAction{restoreMaster, unload(30, remapModule)},
			wantNetwork: []moduleAction{unload(21, tcpModule), unload(22, zeroconfModule)},
		},
		{
			name: "mono unloaded, another default left alone",
			def:  "bluez_sink.00_11_22_33_44_55.a2dp_sink",
			edit: func(c *config.ProcessingConfig) {
				c.Mono.Enabled = false
				c.NetworkSink.Enabled = false
			},
			loaded:   []*proto.GetModuleInfoReply{module(30, remapModule, workaroundMonoArgs)},
			wantMono: []moduleAction{unload(30, remapModule)},
		},
		{
			name: "homonyms left alone",
			edit: func(c *config.ProcessingConfig) {
				c.Mono.Enabled = false
				c.NetworkSink.Enabled = false
			},
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "port=4714 auth-anonymous=1"),
				module(30, remapModule, "sink_name=t master=@DEFAULT_SINK@"),
				// does not parse, so cannot be odio's
				module(31, remapModule, `sink_name=mono sink_properties=device.description="Mono test"`),
			},
		},
		{
			name: "homonyms left alone when enabled",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, "port=4714 auth-anonymous=1"),
				module(30, remapModule, "sink_name=t master=@DEFAULT_SINK@"),
			},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name: "duplicate instance unloaded",
			loaded: []*proto.GetModuleInfoReply{
				module(21, tcpModule, legacyTCPArgs),
				module(22, zeroconfModule, ""),
				module(23, zeroconfModule, ""),
			},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{unload(23, zeroconfModule)},
		},
		{
			name: "feature in error unloaded",
			kind: ServerPipeWire,
			loaded: []*proto.GetModuleInfoReply{
				module(536870917, tcpModule, "auth-anonymous=1"),
				module(536870916, zeroconfModule, ""),
			},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{unload(536870917, tcpModule), unload(536870916, zeroconfModule)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := processingCfg()
			if tt.edit != nil {
				tt.edit(cfg)
			}
			kind := tt.kind
			if kind == "" {
				kind = ServerPulse
			}
			def := tt.def
			if def == "" {
				def = workaroundMaster
			}
			loaded := append(slices.Clone(infraModules), tt.loaded...)

			got := map[featureName][]moduleAction{}
			for _, f := range buildFeatures(cfg, kind, hostAddrs(t, "192.168.1.42/24")) {
				got[f.name] = f.plan(loaded, def)
			}

			want := map[featureName][]moduleAction{
				featureMono:        tt.wantMono,
				featureNetworkSink: tt.wantNetwork,
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("plan =\n  %+v\nwant\n  %+v", got, want)
			}
		})
	}
}
