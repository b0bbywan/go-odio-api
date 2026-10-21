package pulseaudio

import (
	"errors"
	"net"
	"strconv"
	"strings"

	"github.com/b0bbywan/go-odio-api/config"
)

type featureName string

const (
	featureMono        featureName = "mono"
	featureNetworkSink featureName = "network_sink"
)

var (
	errACLUnsupported = errors.New("auth-ip-acl is not supported by PipeWire, set acl: false")
	errNoLANSubnet    = errors.New("no private LAN subnet to allow")
)

// moduleSpec is one server module of a feature.
type moduleSpec struct {
	name     string
	args     map[string]string
	ignore   []string // loaded keys left out of the drift check
	optional bool     // a load failure only warns
	// A sink loaded on top of the default one, which it replaces.
	takesDefault bool
}

// feature is a toggle and the modules it loads. err is set when the config
// cannot be applied on this server: odio then leaves its modules as they are.
type feature struct {
	name    featureName
	onBoot  bool
	modules []moduleSpec
	err     error
}

// buildFeatures resolves the enabled features of the processing config
// against the server kind and the bind interface addresses.
func buildFeatures(cfg *config.ProcessingConfig, kind AudioServerKind, addrs []net.Addr) []feature {
	if cfg == nil {
		return nil
	}
	var features []feature
	if cfg.Mono.Enabled {
		features = append(features, monoFeature(cfg.Mono))
	}
	if cfg.NetworkSink.Enabled {
		features = append(features, networkSinkFeature(cfg.NetworkSink, kind, addrs))
	}
	return features
}

func monoFeature(cfg *config.MonoConfig) feature {
	return feature{
		name:   featureMono,
		onBoot: cfg.OnBoot,
		modules: []moduleSpec{{
			name: "module-remap-sink",
			args: map[string]string{
				"sink_name":       "mono",
				"channels":        "1",
				"channel_map":     "mono",
				"sink_properties": "device.description=Mono",
			},
			ignore:       []string{"master"},
			takesDefault: true,
		}},
	}
}

func networkSinkFeature(cfg *config.NetworkSinkConfig, kind AudioServerKind, addrs []net.Addr) feature {
	f := feature{name: featureNetworkSink, onBoot: cfg.OnBoot}

	tcp := moduleSpec{
		name: "module-native-protocol-tcp",
		args: map[string]string{"port": strconv.Itoa(cfg.Port)},
	}
	switch {
	case !cfg.ACL:
		tcp.args["auth-anonymous"] = "1"
	case kind == ServerPipeWire:
		f.err = errACLUnsupported
	default:
		acl := deriveACL(addrs, cfg.ExtraACL)
		if len(acl) == 1 {
			f.err = errNoLANSubnet
		}
		tcp.args["auth-ip-acl"] = strings.Join(acl, ";")
	}

	f.modules = []moduleSpec{
		tcp,
		{name: "module-zeroconf-publish", args: map[string]string{}, optional: true},
	}
	return f
}
