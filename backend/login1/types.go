package login1

import (
	"context"

	"github.com/godbus/dbus/v5"
)

// Login1Backend manages reboot and shutdown
type Login1Backend struct {
	conn *dbus.Conn
	ctx  context.Context

	CanReboot   bool
	CanPoweroff bool
}
