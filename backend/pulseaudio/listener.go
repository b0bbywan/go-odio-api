package pulseaudio

import (
	"context"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

// eventBufferSize bounds pending subscription events between the protocol
// callback and the listening goroutine.
const eventBufferSize = 64

// Listener listens for pulseaudio changes
type Listener struct {
	backend *PulseAudioBackend
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewListener(backend *PulseAudioBackend) *Listener {
	ctx, cancel := context.WithCancel(backend.ctx)
	return &Listener{
		backend: backend,
		ctx:     ctx,
		cancel:  cancel,
	}
}

// Start starts listening for pulseaudio events
func (l *Listener) Start() error {
	// Forward subscription events to the listening goroutine
	evs := make(chan proto.SubscribeEvent, eventBufferSize)
	l.backend.client.Callback = func(msg interface{}) {
		switch m := msg.(type) {
		case *proto.SubscribeEvent:
			select {
			case evs <- *m:
			default:
				logger.Warn("[pulseaudio] event buffer full, dropping %s #%d", m.Event, m.Index)
			}
		case *proto.ConnectionClosed:
			l.backend.connected.Store(false)
			close(evs)
		}
	}

	// Subscribe to sink, sink input and server changes
	mask := proto.SubscriptionMaskSink | proto.SubscriptionMaskSinkInput | proto.SubscriptionMaskServer
	if err := l.backend.client.Request(&proto.Subscribe{Mask: mask}, nil); err != nil {
		return err
	}

	// Listening goroutine
	go l.listen(evs)

	logger.Info("[pulseaudio] listener started")
	return nil
}

func (l *Listener) listen(evs <-chan proto.SubscribeEvent) {
	for {
		select {
		case <-l.ctx.Done():
			logger.Debug("[pulseaudio] listener context done")
			return

		case ev, ok := <-evs:
			if !ok {
				logger.Warn("[pulseaudio] event channel closed")
				return
			}
			l.handle(ev)
		}
	}
}

// handle routes an event to the cache it affects: removals are applied by
// index without querying the server, everything else reloads that cache.
func (l *Listener) handle(ev proto.SubscribeEvent) {
	logger.Debug("[pulseaudio] event: %s #%d", ev.Event, ev.Index)
	remove := ev.Event.GetType() == proto.EventRemove

	switch ev.Event.GetFacility() {
	case proto.EventSinkSinkInput:
		if remove {
			if c, ok := l.backend.removeClient(ev.Index); ok {
				l.backend.notify(events.Event{Type: events.TypeAudioRemoved, Data: []AudioClient{c}})
			}
			return
		}
		l.refreshClients()

	case proto.EventSink:
		if remove {
			if o, ok := l.backend.removeOutput(ev.Index); ok {
				l.backend.notify(events.Event{Type: events.TypeAudioOutputRemoved, Data: []AudioOutput{o}})
			}
			return
		}
		l.refreshOutputs()

	case proto.EventServer:
		l.refreshOutputs()
	}
}

func (l *Listener) refreshClients() {
	oldClients := l.backend.cache.Load()
	clients, err := l.backend.refreshCache()
	if err != nil {
		logger.Warn("[pulseaudio] failed to refresh clients: %v", err)
		return
	}
	changed, removed := diffClients(oldClients, clients)
	logger.Debug("[pulseaudio] client diff: %d changed, %d removed", len(changed), len(removed))
	if len(changed) > 0 {
		l.backend.notify(events.Event{Type: events.TypeAudioUpdated, Data: changed})
	}
	if len(removed) > 0 {
		l.backend.notify(events.Event{Type: events.TypeAudioRemoved, Data: removed})
	}
}

func (l *Listener) refreshOutputs() {
	oldOutputs := l.backend.outputCache.Load()
	outputs, err := l.backend.refreshOutputCache()
	if err != nil {
		logger.Warn("[pulseaudio] failed to refresh outputs: %v", err)
		return
	}
	changed, removed := diffOutputs(oldOutputs, outputs)
	logger.Debug("[pulseaudio] output diff: %d changed, %d removed", len(changed), len(removed))
	if len(changed) > 0 {
		l.backend.notify(events.Event{Type: events.TypeAudioOutputUpdated, Data: changed})
	}
	if len(removed) > 0 {
		l.backend.notify(events.Event{Type: events.TypeAudioOutputRemoved, Data: removed})
	}
}

// diffClients returns clients that were added/modified and clients that were removed.
func diffClients(old, new []AudioClient) (changed []AudioClient, removed []AudioClient) {
	newByName := make(map[string]struct{}, len(new))
	for _, c := range new {
		newByName[c.Name] = struct{}{}
	}

	oldByName := make(map[string]AudioClient, len(old))
	for _, c := range old {
		oldByName[c.Name] = c
		if _, exists := newByName[c.Name]; !exists {
			removed = append(removed, c)
		}
	}

	for _, c := range new {
		o, exists := oldByName[c.Name]
		if !exists || clientChanged(o, c) {
			changed = append(changed, c)
		}
	}
	return
}

// Stop stops the listener
func (l *Listener) Stop() {
	logger.Info("[pulseaudio] stopping listener")

	// Cancel the context to stop the goroutine
	l.cancel()
}
