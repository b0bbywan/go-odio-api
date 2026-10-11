package input

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"testing"
	"time"

	evdev "github.com/holoplot/go-evdev"

	"github.com/b0bbywan/go-odio-api/backend/action"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

// fakeSource is a device whose events the test feeds; Close, or unplug,
// makes ReadOne fail like a vanished node.
type fakeSource struct {
	events chan evdev.InputEvent
	gone   chan struct{}
	once   sync.Once
}

func newFakeSource() *fakeSource {
	return &fakeSource{events: make(chan evdev.InputEvent, 8), gone: make(chan struct{})}
}

func (f *fakeSource) ReadOne() (*evdev.InputEvent, error) {
	select {
	case ev := <-f.events:
		return &ev, nil
	case <-f.gone:
		return nil, errors.New("no such device")
	}
}

func (f *fakeSource) Close() error {
	f.once.Do(func() { close(f.gone) })
	return nil
}

func (f *fakeSource) key(code evdev.EvCode, value int32) {
	f.events <- evdev.InputEvent{Type: evdev.EV_KEY, Code: code, Value: value}
}

func newBackend(t *testing.T) *InputBackend {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	root := t.TempDir()
	b := &InputBackend{
		ctx:    ctx,
		cancel: cancel,
		host: host{
			devRoot:    filepath.Join(root, "dev"),
			sysRoot:    filepath.Join(root, "sys"),
			retryDelay: time.Millisecond,
			retryFor:   20 * time.Millisecond,
		},
		forced:  map[string]bool{},
		actions: &fakeActions{},
		devices: make(map[string]*device),
		presses: make(chan binding, 4),
		events:  make(chan events.Event, 32),
	}
	t.Cleanup(b.Close)
	return b
}

// addNode fakes the sysfs entry of an event node; parentLink ("subsystem" or
// "driver") and its target make it classify as IR or GPIO.
func addNode(t *testing.T, b *InputBackend, node, name, parentLink, target string) {
	t.Helper()
	parent := filepath.Join(b.sysRoot, node, "device", "device")
	if err := os.MkdirAll(parent, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(b.sysRoot, node, "device", "name"), []byte(name+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if parentLink != "" {
		if err := os.Symlink(target, filepath.Join(parent, parentLink)); err != nil {
			t.Fatal(err)
		}
	}
}

func recv(t *testing.T, b *InputBackend, eventType string) events.Event {
	t.Helper()
	timeout := time.After(2 * time.Second)
	for {
		select {
		case e := <-b.events:
			if e.Type == eventType {
				return e
			}
		case <-timeout:
			t.Fatalf("timed out waiting for %s", eventType)
		}
	}
}

func waitDevices(t *testing.T, b *InputBackend, want []Device) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got := b.Devices()
		if reflect.DeepEqual(got, want) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("devices = %+v, want %+v", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestDeviceLifecycle(t *testing.T) {
	b := newBackend(t)
	actions := &fakeActions{}
	b.actions = actions
	b.keymap = b.buildKeymap(nil)
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")

	sources := make(chan *fakeSource, 2)
	b.open = func(string) (keySource, error) {
		src := newFakeSource()
		sources <- src
		return src, nil
	}
	b.wg.Add(1)
	go b.dispatch()

	path := filepath.Join(b.devRoot, "event3")
	active := []Device{{Path: path, Name: "gpio_ir_recv", Kind: KindIR, State: StateActive}}

	b.attach("event3")
	waitDevices(t, b, active)
	b.attach("event3") // a duplicate Create must not open a second reader
	src := <-sources

	src.key(evdev.KEY_NEXTSONG, keyPressed)
	want := KeyEvent{Device: "gpio_ir_recv", Key: "KEY_NEXTSONG", Code: evdev.KEY_NEXTSONG, State: KeyPressed, Action: "player.next"}
	if got := recv(t, b, events.TypeInputKey).Data; got != want {
		t.Errorf("key event = %+v, want %+v", got, want)
	}
	wantRuns := []string{"player.next"}
	for deadline := time.Now().Add(2 * time.Second); !reflect.DeepEqual(actions.recorded(), wantRuns); {
		if time.Now().After(deadline) {
			t.Fatalf("actions run = %v, want %v", actions.recorded(), wantRuns)
		}
		time.Sleep(time.Millisecond)
	}

	// Unplug, then replug: the node is dropped and handled afresh.
	_ = src.Close()
	waitDevices(t, b, []Device{})
	b.attach("event3")
	waitDevices(t, b, active)

	select {
	case <-sources:
	case <-time.After(time.Second):
		t.Fatal("replugged device was not reopened")
	}
	select {
	case extra := <-sources:
		t.Fatalf("unexpected extra open: %v", extra)
	default:
	}
}

func TestAttachIgnoresUnclassifiedNode(t *testing.T) {
	b := newBackend(t)
	addNode(t, b, "event0", "AT Translated Set 2 keyboard", "", "")
	b.open = func(string) (keySource, error) {
		t.Error("a keyboard must never be opened")
		return nil, fs.ErrPermission
	}

	b.attach("event0")
	b.Close()

	if got := b.Devices(); len(got) != 0 {
		t.Errorf("devices = %+v, want none", got)
	}
}

func TestDeniedDevice(t *testing.T) {
	b := newBackend(t)
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")
	path := filepath.Join(b.devRoot, "event3")

	var mu sync.Mutex
	allowed := false
	b.open = func(p string) (keySource, error) {
		mu.Lock()
		defer mu.Unlock()
		if !allowed {
			return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrPermission}
		}
		return newFakeSource(), nil
	}

	b.attach("event3")
	waitDevices(t, b, []Device{{Path: path, Name: "gpio_ir_recv", Kind: KindIR, State: StateDenied}})

	// A new Create on a denied node is a fresh attempt.
	mu.Lock()
	allowed = true
	mu.Unlock()
	b.attach("event3")
	waitDevices(t, b, []Device{{Path: path, Name: "gpio_ir_recv", Kind: KindIR, State: StateActive}})
}

func TestDetachDropsDeniedDevice(t *testing.T) {
	b := newBackend(t)
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")
	b.open = func(string) (keySource, error) { return nil, fs.ErrPermission }

	b.attach("event3")
	waitDevices(t, b, []Device{{
		Path: filepath.Join(b.devRoot, "event3"), Name: "gpio_ir_recv", Kind: KindIR, State: StateDenied,
	}})

	b.detach("event3")
	if got := b.Devices(); len(got) != 0 {
		t.Errorf("devices = %+v, want none", got)
	}
}

// The node exists before udev grants access to it: the open is retried.
func TestOpenRetriedUntilReadable(t *testing.T) {
	b := newBackend(t)
	b.retryFor = time.Second
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")

	var mu sync.Mutex
	attempts := 0
	b.open = func(string) (keySource, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts < 3 {
			return nil, fs.ErrPermission
		}
		return newFakeSource(), nil
	}

	b.attach("event3")
	waitDevices(t, b, []Device{{
		Path: filepath.Join(b.devRoot, "event3"), Name: "gpio_ir_recv", Kind: KindIR, State: StateActive,
	}})
}

func TestCloseInterruptsReaders(t *testing.T) {
	b := newBackend(t)
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")
	b.open = func(string) (keySource, error) { return newFakeSource(), nil }

	b.attach("event3")
	waitDevices(t, b, []Device{{
		Path: filepath.Join(b.devRoot, "event3"), Name: "gpio_ir_recv", Kind: KindIR, State: StateActive,
	}})

	done := make(chan struct{})
	go func() {
		b.Close()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Close did not return")
	}
}

func TestOnKey(t *testing.T) {
	b := newBackend(t)
	b.actions = &fakeActions{repeat: map[string]bool{"volume.up": true}}
	b.keymap = b.buildKeymap(nil)

	next := Binding{Key: "KEY_NEXTSONG", Action: "player.next"}
	volumeUp := Binding{Key: "KEY_VOLUMEUP", Action: "volume.up"}

	tests := []struct {
		name      string
		code      evdev.EvCode
		value     int32
		wantPress *Binding
		wantState string
	}{
		{name: "press", code: evdev.KEY_NEXTSONG, value: keyPressed, wantPress: &next, wantState: KeyPressed},
		{name: "release", code: evdev.KEY_NEXTSONG, value: keyReleased, wantState: KeyReleased},
		{name: "repeat of a one-shot action", code: evdev.KEY_NEXTSONG, value: keyRepeated},
		{name: "repeat of a repeatable action", code: evdev.KEY_VOLUMEUP, value: keyRepeated, wantPress: &volumeUp},
		{name: "unbound press", code: evdev.KEY_RED, value: keyPressed},
		{name: "unbound release", code: evdev.KEY_RED, value: keyReleased},
		{name: "unbound repeat", code: evdev.KEY_RED, value: keyRepeated},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b.onKey("remote", tt.code, tt.value)

			select {
			case got := <-b.presses:
				if tt.wantPress == nil || got.Binding != *tt.wantPress {
					t.Errorf("queued %+v, want %+v", got.Binding, tt.wantPress)
				}
			default:
				if tt.wantPress != nil {
					t.Errorf("nothing queued, want %+v", *tt.wantPress)
				}
			}

			select {
			case e := <-b.events:
				if got := e.Data.(KeyEvent).State; got != tt.wantState {
					t.Errorf("published state %q, want %q", got, tt.wantState)
				}
			default:
				if tt.wantState != "" {
					t.Errorf("nothing published, want %q", tt.wantState)
				}
			}
		})
	}
}

func TestOnKeyDropsWhenDispatcherBusy(t *testing.T) {
	b := newBackend(t)
	b.actions = &fakeActions{repeat: map[string]bool{"volume.up": true}}
	b.keymap = b.buildKeymap(nil)

	for i := 0; i < cap(b.presses)+3; i++ {
		b.onKey("remote", evdev.KEY_VOLUMEUP, keyRepeated)
	}
	if got := len(b.presses); got != cap(b.presses) {
		t.Errorf("queued %d presses, want %d", got, cap(b.presses))
	}
}

func TestNewDisabled(t *testing.T) {
	for name, cfg := range map[string]*config.InputConfig{
		"nil":      nil,
		"disabled": {Enabled: false},
	} {
		t.Run(name, func(t *testing.T) {
			b, err := New(context.Background(), cfg, nil)
			if b != nil || err != nil {
				t.Errorf("New() = (%v, %v), want (nil, nil)", b, err)
			}
		})
	}
}

// With every target disabled no default binding resolves: none must be kept,
// or it would panic on a key press.
func TestNewWithoutTargets(t *testing.T) {
	b, err := New(context.Background(), &config.InputConfig{Enabled: true}, action.New(nil, nil, nil, nil, nil))
	if err != nil || b == nil {
		t.Fatalf("New() = (%v, %v), want a backend", b, err)
	}
	defer b.Close()
	if len(b.keymap) != 0 {
		t.Errorf("keymap = %v, want empty", b.keymap)
	}
}

func TestStartScansAndWatches(t *testing.T) {
	b := newBackend(t)
	if err := os.MkdirAll(b.devRoot, 0755); err != nil {
		t.Fatal(err)
	}
	b.open = func(string) (keySource, error) { return newFakeSource(), nil }
	touch := func(node string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(b.devRoot, node), nil, 0644); err != nil {
			t.Fatal(err)
		}
	}
	device := func(node, name string, kind Kind) Device {
		return Device{Path: filepath.Join(b.devRoot, node), Name: name, Kind: kind, State: StateActive}
	}

	addNode(t, b, "event0", "AT Translated Set 2 keyboard", "", "")
	addNode(t, b, "event3", "gpio_ir_recv", "subsystem", "../../../class/rc")
	touch("event0")
	touch("event3")
	touch("mice")

	if err := b.Start(); err != nil {
		t.Fatalf("Start() = %v", err)
	}
	waitDevices(t, b, []Device{device("event3", "gpio_ir_recv", KindIR)})

	addNode(t, b, "event7", "gpio-keys", "driver", "../../../bus/platform/drivers/gpio-keys")
	touch("event7")
	waitDevices(t, b, []Device{
		device("event3", "gpio_ir_recv", KindIR),
		device("event7", "gpio-keys", KindGPIO),
	})
}

func TestStartWithoutDevRoot(t *testing.T) {
	b := newBackend(t)
	if err := b.Start(); err != nil {
		t.Errorf("Start() = %v, want nil", err)
	}
}
