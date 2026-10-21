package pulseaudio

import "maps"

// moduleAction loads a module, or unloads the loaded one at index.
type moduleAction struct {
	load     bool
	index    uint32
	module   string
	args     string
	optional bool
}

// plan returns what to unload then load so that, among the modules odio
// owns, f's are loaded as specified when wanted, and none otherwise.
func (f feature) plan(owned []record, want bool) []moduleAction {
	var actions []moduleAction
	for _, s := range f.modules {
		actions = append(actions, s.actions(owned, want)...)
	}
	return actions
}

// actions keeps the first owned instance without drift when wanted, and
// unloads every other one: drifted ones are reloaded.
func (s moduleSpec) actions(owned []record, want bool) []moduleAction {
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
		actions = append(actions, moduleAction{index: r.Index, module: r.Name})
	}
	if want && !kept {
		actions = append(actions, moduleAction{
			load:     true,
			module:   s.name,
			args:     formatModuleArgs(s.args),
			optional: s.optional,
		})
	}
	return actions
}

// orphanActions unloads the modules of features no longer in the config.
func orphanActions(owned []record) []moduleAction {
	var actions []moduleAction
	for _, r := range owned {
		actions = append(actions, moduleAction{index: r.Index, module: r.Name})
	}
	return actions
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
