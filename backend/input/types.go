package input

import "github.com/b0bbywan/go-odio-api/backend/action"

type Kind string

const (
	KindIR     Kind = "ir"
	KindCEC    Kind = "cec"
	KindGPIO   Kind = "gpio"
	KindForced Kind = "forced"
)

type Binding struct {
	Key    string `json:"key"`
	Action string `json:"action"`
	Unit   string `json:"unit,omitempty"`
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

type InputBackend struct {
	actions actionResolver
}
