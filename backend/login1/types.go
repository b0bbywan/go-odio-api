package login1

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
)

// Login1Backend manages reboot and shutdown
type Login1Backend struct {
	conn    *dbus.Conn
	ctx     context.Context
	timeout time.Duration

	CanReboot   bool
	CanPoweroff bool
}
