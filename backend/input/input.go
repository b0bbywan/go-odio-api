package input

import (
	"sort"

	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

func (b *InputBackend) dispatch() {
	defer b.wg.Done()
	for {
		select {
		case <-b.ctx.Done():
			return
		case bd := <-b.presses:
			if err := bd.run(); err != nil {
				logger.Warn("[input] %s (%s) failed: %v", bd.Action, bd.Key, err)
			}
		}
	}
}

// Close interrupts every device read and waits for the goroutines.
func (b *InputBackend) Close() {
	b.cancel()

	b.mu.Lock()
	for _, d := range b.devices {
		if d.src != nil {
			_ = d.src.Close()
		}
	}
	b.mu.Unlock()

	b.wg.Wait()
}

func (b *InputBackend) Events() <-chan events.Event { return b.events }

func (b *InputBackend) Devices() []Device {
	b.mu.Lock()
	defer b.mu.Unlock()

	devices := make([]Device, 0, len(b.devices))
	for _, d := range b.devices {
		devices = append(devices, d.Device)
	}
	sort.Slice(devices, func(i, j int) bool { return devices[i].Path < devices[j].Path })
	return devices
}

func (b *InputBackend) GetStatus() Status {
	bindings := make([]Binding, 0, len(b.keymap))
	for _, bd := range b.keymap {
		bindings = append(bindings, bd.Binding)
	}
	sort.Slice(bindings, func(i, j int) bool { return bindings[i].Key < bindings[j].Key })

	return Status{Devices: b.Devices(), Bindings: bindings}
}
