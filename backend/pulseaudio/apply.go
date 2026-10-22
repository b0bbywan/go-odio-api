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
	loadModule(name, args string) error
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

// processing holds what odio wants of each feature, from the config then from
// external loads and unloads, and what it got on the server.
type processing struct {
	mu       sync.Mutex
	srv      moduleServer
	cfg      *config.ProcessingConfig
	enabled  map[featureName]bool
	features []feature                        // as last reconciled
	state    cache.Value[[]ProcessingFeature] // in features order
}

func newProcessing(cfg *config.ProcessingConfig, srv moduleServer) *processing {
	if cfg == nil || !cfg.Enabled {
		return nil
	}
	return &processing{
		srv: srv,
		cfg: cfg,
		enabled: map[featureName]bool{
			featureMono:        cfg.Mono.Enabled,
			featureNetworkSink: cfg.NetworkSink.Enabled,
		},
	}
}

// reconcile brings the server in line with the wanted features and returns
// those whose state changed. defaultSink is the server's current one.
func (p *processing) reconcile(kind AudioServerKind, defaultSink string, addrs []net.Addr) []ProcessingFeature {
	p.mu.Lock()
	defer p.mu.Unlock()

	changed, err := p.reconcileLocked(kind, defaultSink, addrs)
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

	prev, ok := p.enabled[name]
	if !ok {
		return nil, &NotFoundError{Resource: "processing feature", Name: string(name)}
	}
	p.enabled[name] = enabled
	changed, err := p.reconcileLocked(kind, defaultSink, addrs)
	if err != nil {
		p.enabled[name] = prev
		return nil, err
	}

	for _, st := range p.state.Load() {
		if st.Name == name && st.State == stateError {
			return changed, errors.New(st.Error)
		}
	}
	return changed, nil
}

// reconcileLocked builds the features from the config and what odio wants,
// and applies them; p.mu is held.
func (p *processing) reconcileLocked(kind AudioServerKind, defaultSink string, addrs []net.Addr) ([]ProcessingFeature, error) {
	features := buildFeatures(p.cfg, kind, addrs)
	for i := range features {
		features[i].enabled = p.enabled[features[i].name]
	}
	loaded, err := p.srv.modules()
	if err != nil {
		return nil, err
	}
	next := make([]ProcessingFeature, 0, len(features))
	for _, f := range features {
		next = append(next, p.apply(f, f.plan(loaded, defaultSink)))
	}
	p.features = features
	return p.store(next), nil
}

// refresh follows modules loaded or unloaded by someone else: an active
// feature whose module is gone turns off, a disabled one whose module showed
// up turns on. Nothing is loaded or unloaded; features in error are left.
func (p *processing) refresh() []ProcessingFeature {
	p.mu.Lock()
	defer p.mu.Unlock()

	loaded, err := p.srv.modules()
	if err != nil {
		logger.Debug("[pulseaudio] processing: listing modules: %v", err)
		return nil
	}

	next := slices.Clone(p.state.Load())
	for i, f := range p.features {
		if f.err != nil {
			continue
		}
		present := f.present(loaded)
		switch {
		case next[i].State == stateActive && !present:
			logger.Info("[pulseaudio] %s unloaded by someone else, turned off", f.name)
			p.enabled[f.name] = false
			next[i] = ProcessingFeature{Name: f.name, State: stateDisabled}
		case next[i].State == stateDisabled && present:
			logger.Info("[pulseaudio] %s loaded by someone else, turned on", f.name)
			p.enabled[f.name] = true
			next[i] = ProcessingFeature{Name: f.name, State: stateActive}
		}
	}
	return p.store(next)
}

// apply runs f's actions, stopping at the first failure but for optional
// ones, and returns f's state.
func (p *processing) apply(f feature, actions []moduleAction) ProcessingFeature {
	for _, a := range actions {
		err := a.run(p.srv)
		switch {
		case err == nil:
		case a.optional:
			logger.Warn("[pulseaudio] %s: %v", f.name, err)
		default:
			logger.Error("[pulseaudio] %s: %v", f.name, err)
			return ProcessingFeature{Name: f.name, State: stateError, Error: err.Error()}
		}
	}
	if !f.enabled {
		return ProcessingFeature{Name: f.name, State: stateDisabled}
	}
	if f.err != nil {
		logger.Error("[pulseaudio] %s: %v", f.name, f.err)
		return ProcessingFeature{Name: f.name, State: stateError, Error: f.err.Error()}
	}
	return ProcessingFeature{Name: f.name, State: stateActive}
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

func (a moduleAction) run(srv moduleServer) error {
	switch a.kind {
	case actionLoad:
		logger.Info("[pulseaudio] loading %s %s", a.module, a.args)
		return srv.loadModule(a.module, a.args)
	case actionUnload:
		logger.Info("[pulseaudio] unloading %s #%d", a.module, a.index)
		return srv.unloadModule(a.index)
	case actionSetDefault:
		logger.Info("[pulseaudio] setting default sink %s", a.sink)
		return srv.setDefaultSink(a.sink)
	default:
		return fmt.Errorf("unknown action %d", a.kind)
	}
}
