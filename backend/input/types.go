package input

import (
	"context"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	evdev "github.com/holoplot/go-evdev"

	"github.com/b0bbywan/go-odio-api/backend/action"
	"github.com/b0bbywan/go-odio-api/events"
)

type Kind string

const (
	KindIR     Kind = "ir"
	KindGPIO   Kind = "gpio"
	KindForced Kind = "forced"
)

// State is a handled device's lifecycle: opening → active, or opening → denied
// when the node stays unreadable. A device that goes away leaves the list.
type State string

const (
	StateOpening State = "opening"
	StateActive  State = "active"
	StateDenied  State = "denied"
)

type Device struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Kind  Kind   `json:"kind"`
	State State  `json:"state"`
}

type Binding struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Unit   string `json:"unit,omitempty"`
}

type Status struct {
	Devices  []Device  `json:"devices"`
	Bindings []Binding `json:"bindings"`
}

const (
	KeyPressed  = "pressed"
	KeyReleased = "released"
)

type KeyEvent struct {
	Device string `json:"device"`
	Key    string `json:"key"`
	Code   uint16 `json:"code"`
	State  string `json:"state"`
	Action string `json:"action,omitempty"`
}

// binding is a Binding with its action resolved: repeat lets a held key fire
// it again on kernel auto-repeat.
type binding struct {
	Binding
	repeat bool
	run    func() error
}

// actionResolver is the slice of action.Targets the keymap needs.
type actionResolver interface {
	Resolve(name, unit string) (action.Action, error)
}

// keySource is the slice of an evdev device the reader needs; Close must
// interrupt a pending ReadOne.
type keySource interface {
	ReadOne() (*evdev.InputEvent, error)
	Close() error
}

type device struct {
	Device
	src keySource // set while active
}

// host is how the backend reaches the machine's input devices; tests point it
// at a fake tree.
type host struct {
	devRoot string
	sysRoot string
	open    func(path string) (keySource, error)

	// A hotplugged node shows up before udev has applied its group, so a
	// failed open is retried from retryDelay, doubling, for up to retryFor.
	retryDelay time.Duration
	retryFor   time.Duration
}

type InputBackend struct {
	ctx    context.Context
	cancel context.CancelFunc

	host
	forced map[string]bool // lower-cased device names from config

	actions actionResolver
	keymap  map[evdev.EvCode]binding

	mu      sync.Mutex
	devices map[string]*device // by node path

	// Readers hand key presses to a single dispatcher so a slow target call
	// never stalls a device read.
	presses chan binding
	events  chan events.Event
	watcher *fsnotify.Watcher
	wg      sync.WaitGroup
}
