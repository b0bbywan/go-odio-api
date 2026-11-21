package pulseaudio

import (
	"encoding/json"
	"errors"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/logger"
)

// ledger is what odio loaded on the server, so a module is odio's only when
// its index, name and args match a record: any other is left alone. It lives
// in the runtime dir, outliving an odio restart but wiped with the session.
type ledger struct {
	path     string
	features map[featureName]*ledgerEntry
}

// ledgerEntry is a feature's modules and its last wanted state, restored when
// the server restarts.
type ledgerEntry struct {
	Loaded  bool     `json:"loaded"`
	Modules []record `json:"modules,omitempty"`
}

type record struct {
	Index uint32 `json:"index"`
	Name  string `json:"name"`
	Args  string `json:"args"`
}

// openLedger reads the ledger at path; missing or unreadable, it starts empty.
func openLedger(path string) *ledger {
	l := &ledger{path: path, features: map[featureName]*ledgerEntry{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return l
	}
	if err == nil {
		err = json.Unmarshal(data, &l.features)
	}
	if err != nil {
		logger.Warn("[pulseaudio] processing: reading %s, starting empty: %v", path, err)
		l.features = map[featureName]*ledgerEntry{}
	}
	return l
}

// save writes the ledger through a rename, so a crash leaves the previous one.
func (l *ledger) save() {
	if err := l.write(); err != nil {
		logger.Warn("[pulseaudio] processing: writing %s: %v", l.path, err)
	}
}

func (l *ledger) write() error {
	data, err := json.Marshal(l.features)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// wanted returns the recorded state of name, recording onBoot when there is none.
func (l *ledger) wanted(name featureName, onBoot bool) bool {
	return l.entry(name, onBoot).Loaded
}

func (l *ledger) entry(name featureName, loaded bool) *ledgerEntry {
	e, ok := l.features[name]
	if !ok {
		e = &ledgerEntry{Loaded: loaded}
		l.features[name] = e
	}
	return e
}

func (l *ledger) modules(name featureName) []record {
	if e, ok := l.features[name]; ok {
		return e.Modules
	}
	return nil
}

// add and remove are saved at once: a module loaded but not recorded would be
// loaded again by the next odio.
func (l *ledger) add(name featureName, r record) {
	e := l.entry(name, false)
	e.Modules = append(e.Modules, r)
	l.save()
}

func (l *ledger) remove(name featureName, index uint32) {
	if e, ok := l.features[name]; ok {
		e.Modules = slices.DeleteFunc(e.Modules, func(r record) bool { return r.Index == index })
		l.save()
	}
}

// prune drops the records whose module is no longer loaded, and reports
// whether any was.
func (l *ledger) prune(loaded []*proto.GetModuleInfoReply) bool {
	changed := false
	for _, e := range l.features {
		kept := slices.DeleteFunc(e.Modules, func(r record) bool {
			return !slices.ContainsFunc(loaded, r.is)
		})
		if len(kept) != len(e.Modules) {
			changed = true
		}
		e.Modules = kept
	}
	return changed
}

// is reports whether m is the recorded module. A server restart reuses
// indexes, hence the args check.
func (r record) is(m *proto.GetModuleInfoReply) bool {
	return m.ModuleIndex == r.Index && m.ModuleName == r.Name && sameArgs(m.ModuleArgs, r.Args)
}

func sameArgs(a, b string) bool {
	if a == b {
		return true
	}
	pa, errA := parseModuleArgs(a)
	pb, errB := parseModuleArgs(b)
	return errA == nil && errB == nil && maps.Equal(pa, pb)
}
