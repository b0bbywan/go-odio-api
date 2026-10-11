package input

import (
	"errors"
	"io/fs"
	"path/filepath"
	"time"

	evdev "github.com/holoplot/go-evdev"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

const (
	keyReleased = 0
	keyPressed  = 1
	keyRepeated = 2
)

// attach starts handling an event node if it classifies as a remote. It is a
// no-op for a node already opening or active, and a fresh attempt for a
// denied one.
func (b *InputBackend) attach(node string) {
	name, kind, ok := classify(filepath.Join(b.sysRoot, node), b.forced)
	if !ok || (kind == KindCEC && !b.cec) {
		return
	}
	path := filepath.Join(b.devRoot, node)

	b.mu.Lock()
	defer b.mu.Unlock()
	if d, exists := b.devices[path]; exists && d.State != StateDenied {
		return
	}
	if b.ctx.Err() != nil {
		return
	}
	d := &device{Device: Device{Path: path, Name: name, Kind: kind, State: StateOpening}}
	b.devices[path] = d

	b.wg.Add(1)
	go b.handle(d)
}

// detach drops a denied device whose node is gone; an active one leaves by
// itself when its read fails.
func (b *InputBackend) detach(node string) {
	path := filepath.Join(b.devRoot, node)

	b.mu.Lock()
	d, exists := b.devices[path]
	dropped := exists && d.State == StateDenied
	if dropped {
		delete(b.devices, path)
	}
	b.mu.Unlock()

	if dropped {
		b.notifyUpdated()
	}
}

// handle runs a device from open to unplug. One that stays unreadable is
// kept, as denied; any other leaves the list on the way out.
func (b *InputBackend) handle(d *device) {
	defer b.wg.Done()

	src, err := b.openRetrying(d.Path)
	if errors.Is(err, fs.ErrPermission) {
		logger.Warn("[input] cannot read %s (%s): %v; is the user in the 'input' group (re-login needed after adding it)?",
			d.Path, d.Name, err)
		b.setState(d, StateDenied, nil)
		return
	}
	defer b.remove(d)
	if err != nil {
		if b.ctx.Err() == nil {
			logger.Warn("[input] cannot open %s (%s): %v", d.Path, d.Name, err)
		}
		return
	}
	defer func() { _ = src.Close() }()

	if !b.setState(d, StateActive, src) {
		return
	}
	logger.Info("[input] handling %s (%s, %s)", d.Path, d.Name, d.Kind)
	b.readKeys(d, src)
}

// readKeys forwards the device's key events until its read fails: the device
// is gone, or the backend is closing.
func (b *InputBackend) readKeys(d *device, src keySource) {
	for {
		ev, err := src.ReadOne()
		if err != nil {
			if b.ctx.Err() == nil {
				logger.Info("[input] %s (%s) is gone: %v", d.Path, d.Name, err)
			}
			return
		}
		if ev.Type == evdev.EV_KEY {
			b.onKey(d.Name, ev.Code, ev.Value)
		}
	}
}

func (b *InputBackend) openRetrying(path string) (keySource, error) {
	delay := b.retryDelay
	deadline := time.Now().Add(b.retryFor)
	for {
		src, err := b.open(path)
		if err == nil {
			return src, nil
		}
		retryable := errors.Is(err, fs.ErrPermission) || errors.Is(err, fs.ErrNotExist)
		if !retryable || !time.Now().Add(delay).Before(deadline) {
			return nil, err
		}

		timer := time.NewTimer(delay)
		select {
		case <-b.ctx.Done():
			timer.Stop()
			return nil, b.ctx.Err()
		case <-timer.C:
		}
		delay *= 2
	}
}

// setState moves d to state and reports false, leaving d untouched, when the
// backend is closing: Close only interrupts the sources it can see, so a
// source must not become active behind its back.
func (b *InputBackend) setState(d *device, state State, src keySource) bool {
	b.mu.Lock()
	if b.ctx.Err() != nil {
		b.mu.Unlock()
		return false
	}
	d.State = state
	d.src = src
	b.mu.Unlock()

	b.notifyUpdated()
	return true
}

func (b *InputBackend) remove(d *device) {
	b.mu.Lock()
	if b.devices[d.Path] == d {
		delete(b.devices, d.Path)
	}
	b.mu.Unlock()

	if b.ctx.Err() == nil {
		b.notifyUpdated()
	}
}

// onKey publishes presses and releases of bound keys and queues their action;
// unbound keys are only logged, so a handled keyboard leaks nothing typed.
// Kernel auto-repeat only re-fires repeatable actions and is not published.
func (b *InputBackend) onKey(deviceName string, code evdev.EvCode, value int32) {
	bd, bound := b.keymap[code]
	if !bound {
		if value == keyPressed {
			logger.Debug("[input] unmapped %s (%s)", keyName(code), deviceName)
		}
		return
	}

	switch value {
	case keyPressed:
		b.notifyKey(deviceName, code, KeyPressed, bd.Action)
	case keyReleased:
		b.notifyKey(deviceName, code, KeyReleased, bd.Action)
		return
	case keyRepeated:
		if !bd.repeat {
			return
		}
	default:
		return
	}

	select {
	case b.presses <- bd:
	default:
		logger.Debug("[input] dispatcher busy, dropping %s", bd.Key)
	}
}

func (b *InputBackend) notifyKey(deviceName string, code evdev.EvCode, state, action string) {
	b.notify(events.Event{Type: events.TypeInputKey, Data: KeyEvent{
		Device: deviceName,
		Key:    keyName(code),
		Code:   uint16(code),
		State:  state,
		Action: action,
	}})
}

func (b *InputBackend) notifyUpdated() {
	b.notify(events.Event{Type: events.TypeInputUpdated, Data: b.Devices()})
}

func (b *InputBackend) notify(e events.Event) {
	select {
	case b.events <- e:
	default:
		logger.Warn("[input] event channel full, dropping %s event", e.Type)
	}
}
