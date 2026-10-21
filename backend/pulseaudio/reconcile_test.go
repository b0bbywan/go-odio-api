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

	tcpArgs = "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713"
	// as loaded by odios pulse-tcp.sh on a 192.168.1.x LAN
	legacyTCPArgs = "auth-ip-acl=127.0.0.1;192.168.1.0/24"
	// as loaded by the pulse-mono.sh workaround of odios discussion #86
	workaroundMonoArgs = "master=" + workaroundMaster + " sink_name=mono sink_properties=device.description=Mono channels=1 channel_map=mono"
)

const workaroundMaster = "alsa_output.platform-3f00b840.mailbox.stereo-fallback"

var (
	loadTCP      = moduleAction{kind: actionLoad, module: tcpModule, args: tcpArgs}
	loadZeroconf = moduleAction{kind: actionLoad, module: zeroconfModule, optional: true}
	// on the default sink, which it then takes
	loadMono = []moduleAction{
		{kind: actionLoad, module: remapModule, args: monoArgs},
		{kind: actionSetDefault, sink: "mono"},
	}
	restoreMaster = moduleAction{kind: actionSetDefault, sink: workaroundMaster, optional: true}

	ownedTCP      = record{Index: 21, Name: tcpModule, Args: tcpArgs}
	ownedZeroconf = record{Index: 22, Name: zeroconfModule}
	ownedMono     = record{Index: 30, Name: remapModule, Args: monoArgs}
)

func TestPlan(t *testing.T) {
	tests := []struct {
		name        string
		edit        func(*config.ProcessingConfig)
		unwanted    bool
		def         string // default sink, the workaround master if empty
		owned       []record
		wantMono    []moduleAction
		wantNetwork []moduleAction
	}{
		{
			name:        "nothing owned",
			wantMono:    loadMono,
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name:  "owned as specified",
			def:   "mono",
			owned: []record{ownedTCP, ownedZeroconf, ownedMono},
		},
		{
			name:  "mono on another master",
			owned: []record{ownedTCP, ownedZeroconf, {Index: 30, Name: remapModule, Args: "channel_map=mono channels=1 master=alsa_output.hdmi sink_name=mono sink_properties=device.description=Mono"}},
		},
		{
			name:        "config changed reloads",
			edit:        func(c *config.ProcessingConfig) { c.NetworkSink.Port = 4714 },
			owned:       []record{ownedTCP, ownedZeroconf, ownedMono},
			wantNetwork: []moduleAction{unload(21, tcpModule), {kind: actionLoad, module: tcpModule, args: "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4714"}},
		},
		{
			name:        "acl drift reloads",
			owned:       []record{{Index: 21, Name: tcpModule, Args: "auth-ip-acl=127.0.0.1;192.168.0.0/24 port=4713"}, ownedZeroconf, ownedMono},
			wantNetwork: []moduleAction{unload(21, tcpModule), loadTCP},
		},
		{
			name:        "mono drift reloads on its master",
			def:         "mono",
			owned:       []record{{Index: 30, Name: remapModule, Args: "master=" + workaroundMaster + " sink_name=mono channels=2"}},
			wantMono:    slices.Concat([]moduleAction{restoreMaster, unload(30, remapModule)}, loadMono),
			wantNetwork: []moduleAction{loadTCP, loadZeroconf},
		},
		{
			name:        "unwanted unloaded",
			unwanted:    true,
			def:         "mono",
			owned:       []record{ownedTCP, ownedZeroconf, ownedMono},
			wantMono:    []moduleAction{restoreMaster, unload(30, remapModule)},
			wantNetwork: []moduleAction{unload(21, tcpModule), unload(22, zeroconfModule)},
		},
		{
			name:     "unwanted mono, another default left alone",
			unwanted: true,
			def:      "bluez_sink.00_11_22_33_44_55.a2dp_sink",
			owned:    []record{ownedMono},
			wantMono: []moduleAction{unload(30, remapModule)},
		},
		{
			name:        "duplicate owned instance unloaded",
			owned:       []record{ownedTCP, ownedZeroconf, {Index: 23, Name: zeroconfModule}},
			wantMono:    loadMono,
			wantNetwork: []moduleAction{unload(23, zeroconfModule)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := processingCfg()
			if tt.edit != nil {
				tt.edit(cfg)
			}
			def := tt.def
			if def == "" {
				def = workaroundMaster
			}

			got := map[featureName][]moduleAction{}
			for _, f := range buildFeatures(cfg, ServerPulse, hostAddrs(t, "192.168.1.42/24")) {
				got[f.name] = f.plan(tt.owned, !tt.unwanted, def)
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

func TestOrphanActions(t *testing.T) {
	got := orphanActions([]record{ownedMono, ownedTCP}, "mono")

	want := []moduleAction{restoreMaster, unload(30, remapModule), unload(21, tcpModule)}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("orphanActions =\n  %+v\nwant\n  %+v", got, want)
	}
}
