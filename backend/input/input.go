package input

import (
	"context"
	"sort"
	"strings"

	"github.com/b0bbywan/go-odio-api/backend/action"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
	"github.com/b0bbywan/go-odio-api/logger"
)

func New(ctx context.Context, cfg *config.InputConfig, actions *action.Targets) (*InputBackend, error) {
	if cfg == nil || !cfg.Enabled {
		return nil, nil
	}

	ctx, cancel := context.WithCancel(ctx)
	b := &InputBackend{
		ctx:     ctx,
		cancel:  cancel,
		host:    linuxHost,
		forced:  make(map[string]bool, len(cfg.Devices)),
		cec:     cfg.CEC,
		actions: actions,
		devices: make(map[string]*device),
		presses: make(chan binding, 4),
		events:  make(chan events.Event, 32),
	}
	for _, name := range cfg.Devices {
		b.forced[strings.ToLower(name)] = true
	}
	b.keymap = b.buildKeymap(cfg.Bindings)

	logger.Info("[input] configured: %d bindings, %d forced devices", len(b.keymap), len(b.forced))
	return b, nil
}

// Start handles the remotes already plugged in and watches for new ones. It
// never fails: missing devices or permissions only log.
func (b *InputBackend) Start() error {
	b.wg.Add(1)
	go b.dispatch()

	// Watch before scanning so a node created in between is not missed; attach
	// ignores the duplicate.
	b.startWatcher()
	b.scan()

	logger.Info("[input] backend started successfully")
	return nil
}

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
	if b.watcher != nil {
		if err := b.watcher.Close(); err != nil {
			logger.Warn("[input] closing watcher: %v", err)
		}
	}

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
