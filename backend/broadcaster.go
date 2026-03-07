package backend

import (
	"context"

	idbus "github.com/b0bbywan/go-odio-api/backend/internal/dbus"
	"github.com/b0bbywan/go-odio-api/events"
)

// Broadcaster fans out events from a single upstream channel to all subscribers.
type Broadcaster = idbus.Broadcaster[events.Event]

// NewBroadcaster starts a broadcaster that reads from upstream and fans out to
// all subscribers. It stops when ctx is cancelled or upstream is closed.
func NewBroadcaster(ctx context.Context, upstream <-chan events.Event) *Broadcaster {
	return idbus.NewBroadcaster[events.Event](ctx, upstream)
}

// newBroadcasterFromBackend wires all enabled sub-backend event channels into
// a single Broadcaster. Called once by Backend.New().
func newBroadcasterFromBackend(ctx context.Context, b *Backend) *Broadcaster {
	var srcs []<-chan events.Event
	if b.Bluetooth != nil {
		srcs = append(srcs, b.Bluetooth.Events())
	}
	if b.Login1 != nil {
		srcs = append(srcs, b.Login1.Events())
	}
	if b.MPRIS != nil {
		srcs = append(srcs, b.MPRIS.Events())
	}
	if b.Pulse != nil {
		srcs = append(srcs, b.Pulse.Events())
	}
	if b.Systemd != nil {
		srcs = append(srcs, b.Systemd.Events())
	}
	return NewBroadcaster(ctx, idbus.FanIn[events.Event](ctx, srcs...))
}
