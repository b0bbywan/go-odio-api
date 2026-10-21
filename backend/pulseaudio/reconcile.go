package pulseaudio

import (
	"maps"
	"slices"
	"strings"

	"github.com/jfreymuth/pulse/proto"
)

// moduleAction loads a module, or unloads the loaded one at index.
type moduleAction struct {
	load     bool
	index    uint32
	module   string
	args     string
	optional bool
}

// plan compares the loaded modules with f and returns what to unload and load,
// unloads first. Disabled or in error, f ends with none of its modules loaded.
func (f feature) plan(loaded []*proto.GetModuleInfoReply) []moduleAction {
	want := f.enabled && f.err == nil
	var actions []moduleAction
	for _, s := range f.modules {
		actions = append(actions, s.actions(loaded, want)...)
	}
	return actions
}

// actions keeps the first matching instance without drift when wanted, and
// unloads every other match: drifted ones are reloaded.
func (s moduleSpec) actions(loaded []*proto.GetModuleInfoReply, want bool) []moduleAction {
	var actions []moduleAction
	kept := false
	for _, m := range loaded {
		args, ok := s.match(m)
		if !ok {
			continue
		}
		if want && !kept && !s.drifted(args) {
			kept = true
			continue
		}
		actions = append(actions, moduleAction{index: m.ModuleIndex, module: m.ModuleName})
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

// match reports whether m is odio's instance, with its parsed args. Args that
// don't parse can't be odio's, unless any instance is.
func (s moduleSpec) match(m *proto.GetModuleInfoReply) (map[string]string, bool) {
	if m.ModuleName != s.name {
		return nil, false
	}
	args, err := parseModuleArgs(m.ModuleArgs)
	if err != nil {
		return nil, len(s.identity) == 0
	}
	got, want := s.normalize(args), s.normalize(s.args)
	for _, k := range s.identity {
		if got[k] != want[k] {
			return nil, false
		}
	}
	return args, true
}

// drifted reports whether loaded args differ from the expected ones once
// normalized; unparsed (nil) args always do.
func (s moduleSpec) drifted(args map[string]string) bool {
	return args == nil || !maps.Equal(s.normalize(args), s.normalize(s.args))
}

// normalize returns args as compared: defaults filled in, ignored keys left
// out, sets sorted.
func (s moduleSpec) normalize(args map[string]string) map[string]string {
	n := make(map[string]string, len(s.defaults)+len(args))
	maps.Copy(n, s.defaults)
	maps.Copy(n, args)
	for _, k := range s.ignore {
		delete(n, k)
	}
	for _, k := range s.sets {
		if v, ok := n[k]; ok {
			n[k] = sortedSet(v)
		}
	}
	return n
}

func sortedSet(v string) string {
	var entries []string
	for _, e := range strings.Split(v, ";") {
		if e = strings.TrimSpace(e); e != "" {
			entries = append(entries, e)
		}
	}
	slices.Sort(entries)
	return strings.Join(slices.Compact(entries), ";")
}
