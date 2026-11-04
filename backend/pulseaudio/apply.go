package pulseaudio

import (
	"errors"
	"fmt"
	"net"
	"slices"
	"sync"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/cache"
	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/logger"
)

// moduleServer is what processing needs from the audio server.
type moduleServer interface {
	modules() ([]*proto.GetModuleInfoReply, error)
	loadModule(name, args string) (uint32, error)
	unloadModule(index uint32) error
	setDefaultSink(name string) error
}

var _ moduleServer = (*PulseAudioBackend)(nil)

type featureState string

const (
	stateDisabled featureState = "disabled"
	stateActive   featureState = "active"
	stateError    featureState = "error"
)

// ProcessingFeature is a feature's runtime state; Error is the reason of an
// error.
type ProcessingFeature struct {
	Name  featureName  `json:"name"`
	State featureState `json:"state"`
	Error string       `json:"error,omitempty"`
}

// processing keeps the server's modules in line with what odio wants of each
// feature, and the state it got.
type processing struct {
	mu       sync.Mutex
	srv      moduleServer
	cfg      *config.ProcessingConfig
	ledger   *ledger
	features []feature                        // as last reconciled
	state    cache.Value[[]ProcessingFeature] // in features order
}

// newProcessing is nil when no feature is enabled.
func newProcessing(cfg *config.ProcessingConfig, srv moduleServer) *processing {
	if cfg == nil || !cfg.Mono.Enabled && !cfg.NetworkSink.Enabled {
		return nil
	}
	return &processing{srv: srv, cfg: cfg, ledger: openLedger(cfg.LedgerFile)}
}

// reconcile brings the server in line with the wanted features and returns
// those whose state changed. defaultSink is the server's current one. A
// feature is wanted as last recorded, so as before a server or odio restart,
// or as onBoot for a new session.
func (p *processing) reconcile(kind AudioServerKind, defaultSink string, addrs []net.Addr) []ProcessingFeature {
	p.mu.Lock()
	defer p.mu.Unlock()

	changed, err := p.reconcileLocked(buildFeatures(p.cfg, kind, addrs), defaultSink)
	if err != nil {
		logger.Warn("[pulseaudio] processing: listing modules: %v", err)
	}
	return changed
}

// set turns name on or off, reconciles and returns the features whose state
// changed; it fails if name ends in error.
func (p *processing) set(name featureName, enabled bool, kind AudioServerKind, defaultSink string, addrs []net.Addr) ([]ProcessingFeature, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	features := buildFeatures(p.cfg, kind, addrs)
	if !slices.ContainsFunc(features, func(f feature) bool { return f.name == name }) {
		return nil, &NotFoundError{Resource: "processing feature", Name: string(name)}
	}
	e := p.ledger.entry(name, enabled)
	prev := e.Loaded
	e.Loaded = enabled
	changed, err := p.reconcileLocked(features, defaultSink)
	if err != nil {
		e.Loaded = prev
		return nil, err
	}

	for _, st := range p.state.Load() {
		if st.Name == name && st.State == stateError {
			return changed, errors.New(st.Error)
		}
	}
	return changed, nil
}

// reconcileLocked applies features; p.mu is held.
func (p *processing) reconcileLocked(features []feature, defaultSink string) ([]ProcessingFeature, error) {
	loaded, err := p.srv.modules()
	if err != nil {
		return nil, err
	}
	p.ledger.prune(loaded)
	p.dropOrphans(features, defaultSink)

	next := make([]ProcessingFeature, 0, len(features))
	for _, f := range features {
		next = append(next, p.reconcileFeature(f, defaultSink))
	}
	p.features = features
	p.ledger.save()
	return p.store(next), nil
}

// dropOrphans unloads the modules of features no longer in the config.
func (p *processing) dropOrphans(features []feature, defaultSink string) {
	for name, e := range p.ledger.features {
		if slices.ContainsFunc(features, func(f feature) bool { return f.name == name }) {
			continue
		}
		logger.Info("[pulseaudio] %s no longer configured, unloading its modules", name)
		for _, a := range orphanActions(e.Modules, defaultSink) {
			if err := p.run(name, a); err != nil {
				logger.Warn("[pulseaudio] %s: %v", name, err)
			}
		}
		delete(p.ledger.features, name)
	}
}

func (p *processing) reconcileFeature(f feature, defaultSink string) ProcessingFeature {
	want := p.ledger.wanted(f.name, f.onBoot)
	switch {
	case f.err != nil && want:
		logger.Error("[pulseaudio] %s: %v", f.name, f.err)
		return ProcessingFeature{Name: f.name, State: stateError, Error: f.err.Error()}
	case f.err != nil:
		return ProcessingFeature{Name: f.name, State: stateDisabled}
	}
	if err := p.apply(f.name, f.plan(p.ledger.modules(f.name), want, defaultSink)); err != nil {
		return ProcessingFeature{Name: f.name, State: stateError, Error: err.Error()}
	}
	if !want {
		return ProcessingFeature{Name: f.name, State: stateDisabled}
	}
	return ProcessingFeature{Name: f.name, State: stateActive}
}

// refresh follows odio's modules unloaded by someone else: an active feature
// whose required module is gone turns off. Nothing is loaded or unloaded.
func (p *processing) refresh() []ProcessingFeature {
	p.mu.Lock()
	defer p.mu.Unlock()

	loaded, err := p.srv.modules()
	if err != nil {
		logger.Debug("[pulseaudio] processing: listing modules: %v", err)
		return nil
	}
	if !p.ledger.prune(loaded) {
		return nil
	}

	next := slices.Clone(p.state.Load())
	for i, f := range p.features {
		if next[i].State == stateActive && !f.present(p.ledger.modules(f.name)) {
			logger.Info("[pulseaudio] %s unloaded by someone else, turned off", f.name)
			p.ledger.features[f.name].Loaded = false
			next[i] = ProcessingFeature{Name: f.name, State: stateDisabled}
		}
	}
	p.ledger.save()
	return p.store(next)
}

// apply runs the actions of feature name, stopping at the first failure but
// for optional ones.
func (p *processing) apply(name featureName, actions []moduleAction) error {
	for _, a := range actions {
		err := p.run(name, a)
		switch {
		case err == nil:
		case a.optional:
			logger.Warn("[pulseaudio] %s: %v", name, err)
		default:
			logger.Error("[pulseaudio] %s: %v", name, err)
			return err
		}
	}
	return nil
}

// store publishes next and returns the features whose state changed.
func (p *processing) store(next []ProcessingFeature) []ProcessingFeature {
	prev := p.state.Load()
	var changed []ProcessingFeature
	for i, st := range next {
		if i >= len(prev) || prev[i] != st {
			changed = append(changed, st)
		}
	}
	if changed != nil {
		p.state.Store(next)
	}
	return changed
}

// run runs a, recording in the ledger the modules of feature name it loads
// or unloads.
func (p *processing) run(name featureName, a moduleAction) error {
	switch a.kind {
	case actionLoad:
		logger.Info("[pulseaudio] loading %s %s", a.module, a.args)
		index, err := p.srv.loadModule(a.module, a.args)
		if err == nil {
			p.ledger.add(name, record{Index: index, Name: a.module, Args: a.args})
		}
		return err
	case actionUnload:
		logger.Info("[pulseaudio] unloading %s #%d", a.module, a.index)
		err := p.srv.unloadModule(a.index)
		if err == nil {
			p.ledger.remove(name, a.index)
		}
		return err
	case actionSetDefault:
		logger.Info("[pulseaudio] setting default sink %s", a.sink)
		return p.srv.setDefaultSink(a.sink)
	default:
		return fmt.Errorf("unknown action %d", a.kind)
	}
}
