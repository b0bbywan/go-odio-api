package pulseaudio

import (
	"net"
	"reflect"
	"testing"
)

// hostAddrs builds interface addresses from host CIDRs ("192.168.1.42/24"), as
// net.Interface.Addrs returns them.
func hostAddrs(t *testing.T, cidrs ...string) []net.Addr {
	t.Helper()
	var addrs []net.Addr
	for _, c := range cidrs {
		ip, ipnet, err := net.ParseCIDR(c)
		if err != nil {
			t.Fatalf("ParseCIDR(%q): %v", c, err)
		}
		addrs = append(addrs, &net.IPNet{IP: ip, Mask: ipnet.Mask})
	}
	return addrs
}

func TestDeriveACL(t *testing.T) {
	tests := []struct {
		name  string
		addrs []string
		extra []string
		want  []string
	}{
		{"no address", nil, nil, []string{"127.0.0.1"}},
		{
			// same acl as the module odios pulse-tcp.sh loads on a 192.168.1.x LAN
			"single LAN address",
			[]string{"192.168.1.42/24"}, nil,
			[]string{"127.0.0.1", "192.168.1.0/24"},
		},
		{
			"wired and wifi on the same subnet",
			[]string{"192.168.1.42/24", "192.168.1.43/24"}, nil,
			[]string{"127.0.0.1", "192.168.1.0/24"},
		},
		{
			"interface order kept",
			[]string{"192.168.1.42/24", "10.1.2.3/8", "172.20.5.6/16"}, nil,
			[]string{"127.0.0.1", "192.168.1.0/24", "10.0.0.0/8", "172.20.0.0/16"},
		},
		{
			"public, loopback and IPv6 skipped",
			[]string{"82.64.1.2/24", "127.0.0.1/8", "fd00::1/64", "2a01:e0a::1/64", "192.168.1.42/24"}, nil,
			[]string{"127.0.0.1", "192.168.1.0/24"},
		},
		{
			"172.16/12 boundaries",
			[]string{"172.15.0.1/16", "172.16.0.1/16", "172.31.0.1/16", "172.32.0.1/16"}, nil,
			[]string{"127.0.0.1", "172.16.0.0/16", "172.31.0.0/16"},
		},
		{
			"extra appended after derived subnets, deduplicated",
			[]string{"192.168.1.42/24"}, []string{"10.8.0.0/24", "192.168.1.0/24", "127.0.0.1"},
			[]string{"127.0.0.1", "192.168.1.0/24", "10.8.0.0/24"},
		},
		{
			"extra without LAN subnet",
			nil, []string{"10.8.0.0/24"},
			[]string{"127.0.0.1", "10.8.0.0/24"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveACL(hostAddrs(t, tt.addrs...), tt.extra)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("deriveACL(%v, %v) = %v, want %v", tt.addrs, tt.extra, got, tt.want)
			}
		})
	}
}

func TestDeriveACL_IgnoresNonIPNetAddrs(t *testing.T) {
	addrs := []net.Addr{&net.IPAddr{IP: net.ParseIP("192.168.1.42")}}

	want := []string{"127.0.0.1"}
	if got := deriveACL(addrs, nil); !reflect.DeepEqual(got, want) {
		t.Errorf("deriveACL = %v, want %v", got, want)
	}
}
