package pulseaudio

import (
	"maps"
	"slices"
)

type actionKind int

const (
	actionLoad actionKind = iota
	actionUnload
	actionSetDefault
)

// moduleAction is one step on the server: load module with args, unload the
// module at index, or make sink the default.
type moduleAction struct {
	kind     actionKind
	module   string
	args     string
	index    uint32
	sink     string
	optional bool // a failure only warns
}

// plan returns what to unload then load so that, among the modules odio
// owns, f's are loaded as specified when wanted, and none otherwise.
// defaultSink is the server's current one.
func (f feature) plan(owned []record, want bool, defaultSink string) []moduleAction {
	var actions []moduleAction
	for _, s := range f.modules {
		actions = append(actions, s.actions(owned, want, defaultSink)...)
	}
	return actions
}

// actions keeps the first owned instance without drift when wanted, and
// unloads every other one: drifted ones are reloaded. A sink taking the
// default takes it after load.
func (s moduleSpec) actions(owned []record, want bool, defaultSink string) []moduleAction {
	var actions []moduleAction
	kept := false
	for _, r := range owned {
		if r.Name != s.name {
			continue
		}
		if want && !kept && !s.drifted(r.Args) {
			kept = true
			continue
		}
		var unload []moduleAction
		unload, defaultSink = unloadActions(r, defaultSink)
		actions = append(actions, unload...)
	}
	if !want || kept {
		return actions
	}

	args := s.args
	if s.takesDefault && defaultSink != "" {
		args = maps.Clone(args)
		args["master"] = defaultSink
	}
	actions = append(actions, moduleAction{
		kind:     actionLoad,
		module:   s.name,
		args:     formatModuleArgs(args),
		optional: s.optional,
	})
	if s.takesDefault {
		actions = append(actions, moduleAction{kind: actionSetDefault, sink: s.args["sink_name"]})
	}
	return actions
}

// orphanActions unloads the modules of features no longer in the config.
func orphanActions(owned []record, defaultSink string) []moduleAction {
	var actions []moduleAction
	for _, r := range owned {
		var unload []moduleAction
		unload, defaultSink = unloadActions(r, defaultSink)
		actions = append(actions, unload...)
	}
	return actions
}

// unloadActions unloads r. A sink still the default first hands it back to
// the master it was loaded on; the default sink after is returned.
func unloadActions(r record, defaultSink string) ([]moduleAction, string) {
	var actions []moduleAction
	args, _ := parseModuleArgs(r.Args)
	if master := args["master"]; master != "" && defaultSink == args["sink_name"] {
		actions = append(actions, moduleAction{kind: actionSetDefault, sink: master, optional: true})
		defaultSink = master
	}
	actions = append(actions, moduleAction{kind: actionUnload, module: r.Name, index: r.Index})
	return actions, defaultSink
}

// drifted reports whether loaded args differ from the specified ones;
// unparsable ones always do.
func (s moduleSpec) drifted(loaded string) bool {
	args, err := parseModuleArgs(loaded)
	return err != nil || !maps.Equal(s.withoutIgnored(args), s.withoutIgnored(s.args))
}

func (s moduleSpec) withoutIgnored(args map[string]string) map[string]string {
	n := maps.Clone(args)
	for _, k := range s.ignore {
		delete(n, k)
	}
	return n
}

// present reports whether every required module of f is owned.
func (f feature) present(owned []record) bool {
	for _, s := range f.modules {
		if !s.optional && !slices.ContainsFunc(owned, func(r record) bool { return r.Name == s.name }) {
			return false
		}
	}
	return true
}
