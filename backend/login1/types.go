package login1

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/godbus/dbus/v5"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/events"
)

// Login1Backend manages reboot and shutdown
type Login1Backend struct {
	conn    *dbus.Conn
	ctx     context.Context
	timeout time.Duration

	CanReboot   bool
	CanPoweroff bool

	eventsC chan events.Event

	rebootFile string
	rebootPkgs string
	reboot     cache.Value[*RebootRequired]
	watcher    *fsnotify.Watcher
	wg         sync.WaitGroup
}

type RebootRequired struct {
	Required bool
	Packages []string // packages listed in the .pkgs file, in hook order
}

// PowerActionData is the payload of a power.action event.
type PowerActionData struct {
	Action string `json:"action"`
}

// PowerStatus is the GET /power payload and the power.updated event data.
type PowerStatus struct {
	Reboot           bool     `json:"reboot"`
	PowerOff         bool     `json:"power_off"`
	RebootRequired   bool     `json:"reboot_required"`
	RebootRequiredBy []string `json:"reboot_required_by"`
}
