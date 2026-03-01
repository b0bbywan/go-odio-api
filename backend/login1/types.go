package login1

import (
	"context"

	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/events"
)

// Login1Backend manages reboot and shutdown
type Login1Backend struct {
	conn *dbus.Conn
	ctx  context.Context

	CanReboot   bool
	CanPoweroff bool

	eventsC chan events.Event
}

// PowerActionData is the payload of a power.action event.
type PowerActionData struct {
	Action string `json:"action"`
}
