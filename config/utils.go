package config

import (
	"fmt"
	"net"
	"os"
	"reflect"
	"strings"
	"time"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"

	"github.com/b0bbywan/go-odio-api/logger"
)

// parseLogLevel converts a string to a logger.Level
func parseLogLevel(levelStr string) logger.Level {
	switch strings.ToUpper(levelStr) {
	case "DEBUG":
		return logger.DEBUG
	case "INFO":
		return logger.INFO
	case "WARN":
		return logger.WARN
	case "ERROR":
		return logger.ERROR
	case "FATAL":
		return logger.FATAL
	default:
		return logger.WARN // default
	}
}

func getDuration(key string, fallback time.Duration) time.Duration {
	if d := viper.GetDuration(key); d >= 0 {
		return d
	}
	return fallback
}

// parseSystemdServices accepts viper's raw value for a service list and
// supports two YAML shapes interchangeably within the same list:
//   - bare string  →  SystemdService{Name: s}
//   - object       →  SystemdService{Name: name, URL: url, Open: open}
//
// A mapstructure DecodeHook routes both shapes to SystemdService in one pass;
// the post-decode loop enforces what the hook can't: a non-empty Name, and a
// known Open mode, and only where there is a URL to open.
func parseSystemdServices(raw any) ([]SystemdService, error) {
	if raw == nil {
		return nil, nil
	}

	var services []SystemdService
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:     &services,
		DecodeHook: stringToSystemdServiceHook,
	})
	if err != nil {
		return nil, err
	}
	if err := decoder.Decode(raw); err != nil {
		return nil, err
	}
	for i, s := range services {
		if s.Name == "" {
			return nil, fmt.Errorf("entry %d: missing or empty 'name' field", i)
		}
		switch s.Open {
		case OpenTab, OpenPanel, OpenSelf:
		case "tab":
			services[i].Open = OpenTab
		default:
			logger.Warn("[config] %s: unknown 'open' %q, opening in a new tab", s.Name, s.Open)
			services[i].Open = OpenTab
		}
		if services[i].Open != OpenTab && s.URL == "" {
			logger.Warn("[config] %s: 'open' without 'url', ignored", s.Name)
			services[i].Open = OpenTab
		}
	}
	return services, nil
}

var systemdServiceType = reflect.TypeOf(SystemdService{})

// stringToSystemdServiceHook lets a YAML scalar stand in for a {name, url}
// object inside a []SystemdService. mapstructure handles the map → struct case
// natively; this hook only patches the string → struct edge.
func stringToSystemdServiceHook(from, to reflect.Type, data any) (any, error) {
	if to != systemdServiceType || from.Kind() != reflect.String {
		return data, nil
	}
	return SystemdService{Name: data.(string)}, nil
}

// resolveIfaceToIP returns the IPv4 address of a single named interface.
func resolveIfaceToIP(bind string) (string, error) {
	iface, err := net.InterfaceByName(bind)
	if err != nil {
		return "", fmt.Errorf("interface %q not found", bind)
	}

	addrs, err := iface.Addrs()
	if err != nil {
		return "", err
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok {
			if ip4 := ipnet.IP.To4(); ip4 != nil {
				return ip4.String(), nil
			}
		}
	}

	return "", fmt.Errorf("no IPv4 on interface %s", bind)
}

// resolveBindsToListens converts a list of bind names to host:port listen addresses.
// "all" expands to 0.0.0.0. No implicit addresses are added.
func resolveBindsToListens(binds []string, port string) ([]string, error) {
	for _, b := range binds {
		if b == "all" {
			return []string{net.JoinHostPort("0.0.0.0", port)}, nil
		}
	}

	seen := map[string]bool{}
	var addrs []string

	for _, bind := range binds {
		ip, err := resolveIfaceToIP(bind)
		if err != nil {
			return nil, err
		}
		addr := net.JoinHostPort(ip, port)
		if !seen[addr] {
			seen[addr] = true
			addrs = append(addrs, addr)
		}
	}

	return addrs, nil
}

// hasLoopback returns true if listens contains 127.0.0.1:port or 0.0.0.0:port.
func hasLoopback(listens []string, port string) bool {
	loopback := net.JoinHostPort("127.0.0.1", port)
	wildcard := net.JoinHostPort("0.0.0.0", port)
	for _, l := range listens {
		if l == loopback || l == wildcard {
			return true
		}
	}
	return false
}

// getZeroconfInterfaces returns the network interfaces on which mDNS should be announced.
func getZeroconfInterfaces(binds []string) []net.Interface {
	for _, b := range binds {
		if b == "all" {
			return getAllActiveNonLoopback()
		}
	}

	var result []net.Interface
	for _, bind := range binds {
		if bind == "lo" {
			continue
		}
		iface, err := net.InterfaceByName(bind)
		if err != nil {
			logger.Warn("[config] interface %q not found: %v", bind, err)
			continue
		}
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		result = append(result, *iface)
	}
	return result
}

// getAllActiveNonLoopback returns every UP interface except loopback
func getAllActiveNonLoopback() []net.Interface {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}

	var result []net.Interface
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp != 0 && iface.Flags&net.FlagLoopback == 0 {
			result = append(result, iface)
		}
	}
	return result
}

func systemdHasUTMP() bool {
	_, err := os.Stat("/run/utmp")
	return err == nil
}

// isUnixSocket reports whether path names an existing Unix socket.
func isUnixSocket(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.Mode()&os.ModeSocket != 0
}
