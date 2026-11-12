package pulseaudio

import (
	"net"

	"github.com/b0bbywan/go-odio-api/logger"
)

// deriveACL builds the network sink's auth-ip-acl: loopback first, then the
// private IPv4 subnet of each address in order, then extra, deduplicated.
// Only loopback left means no LAN client could connect.
func deriveACL(addrs []net.Addr, extra []string) []string {
	acl := []string{"127.0.0.1"}
	seen := map[string]bool{"127.0.0.1": true}
	add := func(entry string) {
		if !seen[entry] {
			seen[entry] = true
			acl = append(acl, entry)
		}
	}

	for _, addr := range addrs {
		ipnet, ok := addr.(*net.IPNet)
		if !ok {
			continue
		}
		ip := ipnet.IP.To4()
		if ip == nil || !ip.IsPrivate() {
			continue
		}
		subnet := net.IPNet{IP: ip.Mask(ipnet.Mask), Mask: ipnet.Mask}
		add(subnet.String())
	}
	for _, entry := range extra {
		add(entry)
	}
	return acl
}

// interfaceAddrs reads the current addresses of ifaces, in order.
func interfaceAddrs(ifaces []net.Interface) []net.Addr {
	var addrs []net.Addr
	for _, iface := range ifaces {
		a, err := iface.Addrs()
		if err != nil {
			logger.Warn("[pulseaudio] reading addresses of %s: %v", iface.Name, err)
			continue
		}
		addrs = append(addrs, a...)
	}
	return addrs
}
