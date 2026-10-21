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

const defaultNativeTCPPort = "4713"

var (
	errACLUnsupported = errors.New("auth-ip-acl is not supported by PipeWire, set acl: false")
	errNoLANSubnet    = errors.New("no private LAN subnet to allow")
)

// moduleSpec is one server module of a feature. A loaded module is odio's when
// its name matches and so do the identity keys of args; without identity, any
// instance is.
type moduleSpec struct {
	name     string
	args     map[string]string
	identity []string
	defaults map[string]string // values the module assumes for absent keys
	ignore   []string          // loaded keys left out of the drift check
	sets     []string          // ;-separated values compared regardless of order
	optional bool              // a load failure only warns
}

// feature is a toggle and the modules it loads. err is set when the config
// cannot be applied on this server: nothing of the feature is then loaded.
type feature struct {
	name    featureName
	enabled bool
	modules []moduleSpec
	err     error
}

// buildFeatures resolves the processing config against the server kind and
// the bind interface addresses. Disabled processing manages nothing.
func buildFeatures(cfg *config.ProcessingConfig, kind AudioServerKind, addrs []net.Addr) []feature {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return []feature{
		monoFeature(cfg.Mono),
		networkSinkFeature(cfg.NetworkSink, kind, addrs),
	}
}

func monoFeature(cfg *config.MonoConfig) feature {
	return feature{
		name:    featureMono,
		enabled: cfg.Enabled,
		modules: []moduleSpec{{
			name: "module-remap-sink",
			args: map[string]string{
				"sink_name":       "mono",
				"channels":        "1",
				"channel_map":     "mono",
				"sink_properties": "device.description=Mono",
			},
			identity: []string{"sink_name"},
			ignore:   []string{"master"},
		}},
	}
}

func networkSinkFeature(cfg *config.NetworkSinkConfig, kind AudioServerKind, addrs []net.Addr) feature {
	f := feature{name: featureNetworkSink, enabled: cfg.Enabled}

	tcp := moduleSpec{
		name:     "module-native-protocol-tcp",
		args:     map[string]string{"port": strconv.Itoa(cfg.Port)},
		identity: []string{"port"},
		defaults: map[string]string{"port": defaultNativeTCPPort},
		sets:     []string{"auth-ip-acl"},
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
