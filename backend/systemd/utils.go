package systemd

import (
	"context"
	"strings"

	sysdbus "github.com/coreos/go-systemd/v22/dbus"
	"github.com/godbus/dbus/v5"
)

// unitNameFromPath extrait le nom de l'unité depuis le path D-Bus
// Ex: /org/freedesktop/systemd1/unit/spotifyd_2eservice -> spotifyd.service
func unitNameFromPath(path dbus.ObjectPath) string {
	s := string(path)
	const prefix = "/org/freedesktop/systemd1/unit/"
	if !strings.HasPrefix(s, prefix) {
		return ""
	}
	encoded := s[len(prefix):]
	// Décoder les caractères échappés (ex: _2e -> .)
	return decodeUnitName(encoded)
}

// decodeUnitName décode le nom d'unité échappé par systemd
func decodeUnitName(encoded string) string {
	var result strings.Builder
	for i := 0; i < len(encoded); i++ {
		if encoded[i] == '_' && i+2 < len(encoded) {
			// Séquence d'échappement _XX (hex)
			hex := encoded[i+1 : i+3]
			var b byte
			if _, err := parseHexByte(hex, &b); err == nil {
				result.WriteByte(b)
				i += 2
				continue
			}
		}
		result.WriteByte(encoded[i])
	}
	return result.String()
}

func parseHexByte(s string, b *byte) (bool, error) {
	if len(s) != 2 {
		return false, nil
	}
	val := 0
	for _, c := range s {
		val <<= 4
		switch {
		case c >= '0' && c <= '9':
			val |= int(c - '0')
		case c >= 'a' && c <= 'f':
			val |= int(c - 'a' + 10)
		case c >= 'A' && c <= 'F':
			val |= int(c - 'A' + 10)
		default:
			return false, nil
		}
	}
	*b = byte(val)
	return true, nil
}

// stateKey génère une clé unique pour le couple service/scope
func stateKey(name string, scope UnitScope) string {
	return string(scope) + "/" + name
}

func serviceFromProps(name string, scope UnitScope, props map[string]interface{}) Service {
	svc := Service{
		Name:  name,
		Scope: scope,
	}

	if props == nil || props["UnitFileState"] == nil || props["UnitFileState"] == "" {
		svc.Exists = false
		svc.Enabled = false
		return svc
	}

	svc.Exists = true
	svc.Enabled = props["UnitFileState"] == "enabled"
	svc.ActiveState, _ = props["ActiveState"].(string)

	subState, _ := props["SubState"].(string)
	svc.Running = svc.ActiveState == "active" && subState == "running"

	if desc, ok := props["Description"].(string); ok {
		svc.Description = desc
	}

	return svc
}

func startUnit(ctx context.Context, conn *sysdbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.StartUnitContext(ctx, name, "replace", ch)
	})
}

func stopUnit(ctx context.Context, conn *sysdbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.StopUnitContext(ctx, name, "replace", ch)
	})
}

func restartUnit(ctx context.Context, conn *sysdbus.Conn, name string) error {
	return doUnitJob(ctx, func(ch chan<- string) (int, error) {
		return conn.RestartUnitContext(ctx, name, "replace", ch)
	})
}

func doUnitJob(
	ctx context.Context,
	f func(chan<- string) (int, error),
) error {
	ch := make(chan string, 1)

	if _, err := f(ch); err != nil {
		return err
	}

	<-ch
	return nil
}

func ParseUnitScope(v string) (UnitScope, bool) {
	switch UnitScope(v) {
	case ScopeSystem, ScopeUser:
		return UnitScope(v), true
	default:
		return "", false
	}
}