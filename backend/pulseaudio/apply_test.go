package pulseaudio

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/jfreymuth/pulse/proto"

	"github.com/b0bbywan/go-odio-api/config"
	"github.com/b0bbywan/go-odio-api/events"
)

// fakeServer keeps a module list and the sinks remap modules add, and records
// every call made to it.
type fakeServer struct {
	mods     []*proto.GetModuleInfoReply
	sinks    []string
	def      string
	next     uint32
	failLoad map[string]error
	failList error
	calls    []string
}

func newFakeServer(def string, mods ...*proto.GetModuleInfoReply) *fakeServer {
	return &fakeServer{mods: mods, sinks: []string{def}, def: def, next: 100}
}

func (f *fakeServer) modules() ([]*proto.GetModuleInfoReply, error) {
	if f.failList != nil {
		return nil, f.failList
	}
	return slices.Clone(f.mods), nil
}

func (f *fakeServer) loadModule(name, args string) (uint32, error) {
	f.calls = append(f.calls, "load "+name+" "+args)
	if err := f.failLoad[name]; err != nil {
		return 0, err
	}
	f.next++
	f.mods = append(f.mods, module(f.next, name, args))
	if name == remapModule {
		parsed, _ := parseModuleArgs(args)
		f.sinks = append(f.sinks, parsed["sink_name"])
	}
	return f.next, nil
}

// restart drops every module and sink but the hardware one, and reuses the
// module indexes.
func (f *fakeServer) restart() {
	f.mods, f.sinks, f.def, f.next, f.calls = nil, []string{hwSink}, hwSink, 100, nil
}

func (f *fakeServer) unloadModule(index uint32) error {
	f.calls = append(f.calls, fmt.Sprintf("unload %d", index))
	i := slices.IndexFunc(f.mods, func(m *proto.GetModuleInfoReply) bool { return m.ModuleIndex == index })
	if i < 0 {
		return errors.New("no such module")
	}
	if f.mods[i].ModuleName == remapModule {
		parsed, _ := parseModuleArgs(f.mods[i].ModuleArgs)
		f.removeSink(parsed["sink_name"])
	}
	f.mods = slices.Delete(f.mods, i, i+1)
	return nil
}

// removeSink drops a sink, falling back to the first one left as default.
func (f *fakeServer) removeSink(name string) {
	f.sinks = slices.DeleteFunc(f.sinks, func(s string) bool { return s == name })
	if f.def == name && len(f.sinks) > 0 {
		f.def = f.sinks[0]
	}
}

func (f *fakeServer) setDefaultSink(name string) error {
	f.calls = append(f.calls, "default "+name)
	if !slices.Contains(f.sinks, name) {
		return errors.New("no such entity")
	}
	f.def = name
	return nil
}

const (
	hwSink   = workaroundMaster
	monoArgs = "channel_map=mono channels=1 master=" + hwSink + " sink_name=mono sink_properties=device.description=Mono"
)

func newTestProcessing(t *testing.T, srv moduleServer, mono, networkSink bool) *processing {
	t.Helper()
	cfg := processingCfg()
	cfg.Mono.OnBoot = mono
	cfg.NetworkSink.OnBoot = networkSink
	cfg.LedgerFile = ledgerPath(t)
	return newProcessing(cfg, srv)
}

// reopen is a new odio on the same server and ledger, with cfg.
func reopen(p *processing, cfg *config.ProcessingConfig) *processing {
	cfg.LedgerFile = p.cfg.LedgerFile
	return newProcessing(cfg, p.srv)
}

// reconcile passes the fake's current default sink, as start passes the one it
// just read.
func reconcile(t *testing.T, p *processing, kind AudioServerKind) []ProcessingFeature {
	t.Helper()
	return p.reconcile(kind, p.srv.(*fakeServer).def, hostAddrs(t, "192.168.1.42/24"))
}

// set toggles name like reconcile, with the fake's current default sink.
func set(t *testing.T, p *processing, name featureName, enabled bool, kind AudioServerKind) ([]ProcessingFeature, error) {
	t.Helper()
	return p.set(name, enabled, kind, p.srv.(*fakeServer).def, hostAddrs(t, "192.168.1.42/24"))
}

func assertCalls(t *testing.T, srv *fakeServer, want ...string) {
	t.Helper()
	if !reflect.DeepEqual(srv.calls, want) {
		t.Errorf("calls =\n  %q\nwant\n  %q", srv.calls, want)
	}
}

// assertStatus checks the cached state of name; want is named after it.
func assertStatus(t *testing.T, p *processing, name featureName, want ProcessingFeature) {
	t.Helper()
	want.Name = name
	i := slices.IndexFunc(p.state.Load(), func(f ProcessingFeature) bool { return f.Name == name })
	if i < 0 {
		t.Fatalf("%s state not cached", name)
	}
	if got := p.state.Load()[i]; got != want {
		t.Errorf("%s state = %+v, want %+v", name, got, want)
	}
}

func assertOwned(t *testing.T, p *processing, name featureName, want ...record) {
	t.Helper()
	if got := openLedger(p.ledger.path).modules(name); !reflect.DeepEqual(got, want) {
		t.Errorf("%s saved modules = %+v, want %+v", name, got, want)
	}
}

var (
	active   = ProcessingFeature{State: stateActive}
	disabled = ProcessingFeature{State: stateDisabled}
)

func TestNewProcessing_NothingEnabled(t *testing.T) {
	cfg := processingCfg()
	cfg.Mono.Enabled = false
	cfg.NetworkSink.Enabled = false

	if p := newProcessing(cfg, nil); p != nil {
		t.Errorf("newProcessing = %+v, want nil", p)
	}
	if p := newProcessing(nil, nil); p != nil {
		t.Errorf("newProcessing(nil) = %+v, want nil", p)
	}
}

func TestReconcile_LoadsMonoOnTheDefaultSink(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)

	changed := reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+remapModule+" "+monoArgs, "default mono")
	assertStatus(t, p, featureMono, active)
	assertOwned(t, p, featureMono, record{Index: 101, Name: remapModule, Args: monoArgs})
	if srv.def != "mono" {
		t.Errorf("default sink = %s, want mono", srv.def)
	}
	want := []ProcessingFeature{{Name: featureMono, State: stateActive}, {Name: featureNetworkSink, State: stateDisabled}}
	if !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	if again := reconcile(t, p, ServerPulse); again != nil {
		t.Errorf("second reconcile changed %v, want nothing", again)
	}
}

// odios scripts load their own modules: odio must not touch them, nor count
// them as its own.
func TestReconcile_IgnoresForeignModules(t *testing.T) {
	foreign := []*proto.GetModuleInfoReply{
		module(21, tcpModule, legacyTCPArgs),
		module(22, zeroconfModule, ""),
		module(30, remapModule, workaroundMonoArgs),
	}
	srv := newFakeServer(hwSink, foreign...)
	p := newTestProcessing(t, srv, false, false)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv)
	assertStatus(t, p, featureMono, disabled)
	assertStatus(t, p, featureNetworkSink, disabled)

	srv.mods = slices.DeleteFunc(srv.mods, func(m *proto.GetModuleInfoReply) bool { return m.ModuleIndex == 30 })
	if changed := p.refresh(); changed != nil {
		t.Errorf("refresh changed %v on a foreign unload, want nothing", changed)
	}
}

// The port held by a foreign module, odio's load fails and it says so.
func TestReconcile_PortHeldByForeignModule(t *testing.T) {
	srv := newFakeServer(hwSink, module(21, tcpModule, legacyTCPArgs))
	srv.failLoad = map[string]error{tcpModule: errors.New("module initialization failed")}
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+tcpModule+" "+tcpArgs)
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: "module initialization failed"})
}

func TestReconcile_RestoresAfterServerRestart(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, true)
	reconcile(t, p, ServerPulse)

	srv.restart()
	reconcile(t, p, ServerPulse)

	assertCalls(t, srv,
		"load "+remapModule+" "+monoArgs, "default mono",
		"load "+tcpModule+" "+tcpArgs, "load "+zeroconfModule+" ",
	)
	assertStatus(t, p, featureMono, active)
	assertStatus(t, p, featureNetworkSink, active)
}

func TestReconcile_OdioRestartKeepsItsModules(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, true)
	reconcile(t, p, ServerPulse)
	srv.calls = nil

	p = reopen(p, p.cfg)
	reconcile(t, p, ServerPulse)

	assertCalls(t, srv)
	assertStatus(t, p, featureMono, active)
	assertStatus(t, p, featureNetworkSink, active)
}

// Within a session, the last state wins over onBoot.
func TestReconcile_OdioRestartKeepsTheLastState(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, true)
	reconcile(t, p, ServerPulse)
	srv.mods = slices.DeleteFunc(srv.mods, func(m *proto.GetModuleInfoReply) bool { return m.ModuleName == tcpModule })
	p.refresh()
	srv.calls = nil

	p = reopen(p, p.cfg)
	reconcile(t, p, ServerPulse)

	// zeroconf, left by the external unload, goes with the feature off
	assertCalls(t, srv, "unload 102")
	assertStatus(t, p, featureNetworkSink, disabled)
}

func TestReconcile_ConfigChangedBetweenRuns(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, true)
	reconcile(t, p, ServerPulse)
	srv.calls = nil

	cfg := processingCfg()
	cfg.Mono.OnBoot = false
	cfg.NetworkSink.Port = 4714
	p = reopen(p, cfg)
	reconcile(t, p, ServerPulse)

	args := "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4714"
	assertCalls(t, srv, "unload 101", "load "+tcpModule+" "+args)
	assertOwned(t, p, featureNetworkSink, record{Index: 102, Name: zeroconfModule}, record{Index: 103, Name: tcpModule, Args: args})
	assertStatus(t, p, featureNetworkSink, active)
}

func TestReconcile_UnloadsFeatureRemovedFromConfig(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)
	reconcile(t, p, ServerPulse)
	srv.calls = nil

	cfg := processingCfg()
	cfg.Mono.Enabled = false
	p = reopen(p, cfg)
	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "default "+hwSink, "unload 101")
	if _, ok := openLedger(p.ledger.path).features[featureMono]; ok {
		t.Error("mono still in the ledger")
	}
	if got := p.state.Load(); len(got) != 1 || got[0].Name != featureNetworkSink {
		t.Errorf("state = %+v, want the network sink alone", got)
	}
}

func TestReconcile_UnloadsMonoWhenMasterGone(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)
	reconcile(t, p, ServerPulse)
	srv.sinks = []string{"mono", "alsa_output.hdmi"}
	srv.calls = nil

	cfg := processingCfg()
	cfg.Mono.Enabled = false
	p = reopen(p, cfg)
	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "default "+hwSink, "unload 101")
	assertOwned(t, p, featureMono)
}

func TestReconcile_ReloadsDriftedMono(t *testing.T) {
	drifted := "master=" + hwSink + " sink_name=mono channels=2"
	srv := newFakeServer(hwSink, module(30, remapModule, drifted))
	srv.sinks = append(srv.sinks, "mono")
	srv.def = "mono"
	p := newTestProcessing(t, srv, true, false)
	p.ledger.wanted(featureMono, true)
	p.ledger.add(featureMono, record{Index: 30, Name: remapModule, Args: drifted})

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv,
		"default "+hwSink, "unload 30",
		"load "+remapModule+" "+monoArgs, "default mono",
	)
	assertStatus(t, p, featureMono, active)
	assertOwned(t, p, featureMono, record{Index: 101, Name: remapModule, Args: monoArgs})
}

func TestReconcile_LoadFailure(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failLoad = map[string]error{tcpModule: errors.New("module initialization failed")}
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+tcpModule+" "+tcpArgs)
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: "module initialization failed"})
}

func TestReconcile_OptionalLoadFailureOnlyWarns(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failLoad = map[string]error{zeroconfModule: errors.New("avahi unreachable")}
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+tcpModule+" "+tcpArgs, "load "+zeroconfModule+" ")
	assertStatus(t, p, featureNetworkSink, active)
}

// A feature whose config no longer applies, here the LAN gone at a reconnect,
// keeps the modules odio loaded.
func TestReconcile_FeatureErrorLeavesModulesAlone(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, true)
	reconcile(t, p, ServerPulse)
	srv.calls = nil

	p.reconcile(ServerPulse, srv.def, hostAddrs(t, "169.254.3.4/16"))

	assertCalls(t, srv)
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: errNoLANSubnet.Error()})
	assertOwned(t, p, featureNetworkSink, record{Index: 101, Name: tcpModule, Args: tcpArgs}, record{Index: 102, Name: zeroconfModule})
}

func TestReconcile_FeatureErrorWhileDisabled(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, false)

	reconcile(t, p, ServerPipeWire)

	assertCalls(t, srv)
	assertStatus(t, p, featureNetworkSink, disabled)
}

func TestReconcile_ListingFailure(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failList = errors.New("connection closed")
	p := newTestProcessing(t, srv, true, true)

	if changed := reconcile(t, p, ServerPulse); changed != nil {
		t.Errorf("changed = %v, want nothing", changed)
	}
	assertCalls(t, srv)
}

func TestRefresh_ExternalUnloadTurnsOff(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, true)
	reconcile(t, p, ServerPulse)
	srv.mods = slices.DeleteFunc(srv.mods, func(m *proto.GetModuleInfoReply) bool { return m.ModuleName == tcpModule })

	changed := p.refresh()

	if want := []ProcessingFeature{{Name: featureNetworkSink, State: stateDisabled}}; !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	assertStatus(t, p, featureNetworkSink, disabled)

	// the server restarts: nothing comes back
	srv.restart()
	reconcile(t, p, ServerPulse)
	assertCalls(t, srv)
}

func TestRefresh_MasterLossTurnsMonoOff(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)
	reconcile(t, p, ServerPulse)
	// the remap sink unloads itself along with its master
	srv.mods = nil

	p.refresh()

	assertStatus(t, p, featureMono, disabled)
	assertOwned(t, p, featureMono)
}

func TestRefresh_ZeroconfLossKeepsActive(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, true)
	reconcile(t, p, ServerPulse)
	srv.mods = slices.DeleteFunc(srv.mods, func(m *proto.GetModuleInfoReply) bool { return m.ModuleName == zeroconfModule })

	if changed := p.refresh(); changed != nil {
		t.Errorf("changed = %v, want nothing", changed)
	}
	assertStatus(t, p, featureNetworkSink, active)
	assertOwned(t, p, featureNetworkSink, record{Index: 101, Name: tcpModule, Args: tcpArgs})
}

func TestRefresh_LeavesErrorsAndSteadyState(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failLoad = map[string]error{tcpModule: errors.New("module initialization failed")}
	p := newTestProcessing(t, srv, true, true)
	reconcile(t, p, ServerPulse)

	if changed := p.refresh(); changed != nil {
		t.Errorf("changed = %v, want nothing", changed)
	}
	assertStatus(t, p, featureMono, active)
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: "module initialization failed"})
}

func TestSet_TogglesOneFeature(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, false)
	reconcile(t, p, ServerPulse)

	changed, err := set(t, p, featureNetworkSink, true, ServerPulse)

	if err != nil {
		t.Fatalf("set error: %v", err)
	}
	assertCalls(t, srv, "load "+tcpModule+" "+tcpArgs, "load "+zeroconfModule+" ")
	if want := []ProcessingFeature{{Name: featureNetworkSink, State: stateActive}}; !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	assertStatus(t, p, featureMono, disabled)

	// the intention sticks: a server restart brings it back
	srv.restart()
	reconcile(t, p, ServerPulse)
	assertCalls(t, srv, "load "+tcpModule+" "+tcpArgs, "load "+zeroconfModule+" ")
}

// Within a session, a toggle wins over onBoot across an odio restart.
func TestSet_HoldsAcrossOdioRestart(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)
	reconcile(t, p, ServerPulse)
	if _, err := set(t, p, featureMono, false, ServerPulse); err != nil {
		t.Fatalf("set error: %v", err)
	}
	srv.calls = nil

	p = reopen(p, p.cfg)
	reconcile(t, p, ServerPulse)

	assertCalls(t, srv)
	assertStatus(t, p, featureMono, disabled)
}

func TestSet_FeatureEndingInError(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, false)
	reconcile(t, p, ServerPipeWire)

	_, err := set(t, p, featureNetworkSink, true, ServerPipeWire)

	if err == nil || err.Error() != errACLUnsupported.Error() {
		t.Errorf("set error = %v, want %v", err, errACLUnsupported)
	}
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: errACLUnsupported.Error()})
}

func TestSet_Errors(t *testing.T) {
	t.Run("unknown feature", func(t *testing.T) {
		p := newTestProcessing(t, newFakeServer(hwSink), false, false)
		reconcile(t, p, ServerPulse)

		var notFound *NotFoundError
		if _, err := set(t, p, "reverb", true, ServerPulse); !errors.As(err, &notFound) {
			t.Errorf("set error = %v, want a NotFoundError", err)
		}
	})
	t.Run("feature not enabled", func(t *testing.T) {
		cfg := processingCfg()
		cfg.NetworkSink.Enabled = false
		cfg.LedgerFile = ledgerPath(t)
		p := newProcessing(cfg, newFakeServer(hwSink))
		reconcile(t, p, ServerPulse)

		var notFound *NotFoundError
		if _, err := set(t, p, featureNetworkSink, true, ServerPulse); !errors.As(err, &notFound) {
			t.Errorf("set error = %v, want a NotFoundError", err)
		}
	})
	t.Run("listing failure", func(t *testing.T) {
		srv := newFakeServer(hwSink)
		p := newTestProcessing(t, srv, false, false)
		reconcile(t, p, ServerPulse)
		srv.failList = errors.New("connection closed")

		if _, err := set(t, p, featureMono, true, ServerPulse); err == nil {
			t.Error("set succeeded, want the listing error")
		}
		assertCalls(t, srv)
		if p.ledger.features[featureMono].Loaded {
			t.Error("mono wanted after a failed set")
		}
	})
}

func TestEnableProcessing_MonoOnCachedDefault(t *testing.T) {
	srv := newFakeServer(hwSink)
	pa := &PulseAudioBackend{events: make(chan events.Event, 2)}
	pa.outputCache.Store([]AudioOutput{{Name: "alsa_output.hdmi"}, {Name: hwSink, Default: true}})
	pa.processing = newTestProcessing(t, srv, false, false)
	reconcile(t, pa.processing, ServerPulse)

	if err := pa.EnableProcessing("mono"); err != nil {
		t.Fatalf("EnableProcessing error: %v", err)
	}

	assertCalls(t, srv, "load "+remapModule+" "+monoArgs, "default mono")
	want := events.Event{
		Type: events.TypeAudioProcessing,
		Data: []ProcessingFeature{{Name: featureMono, State: stateActive}},
	}
	select {
	case got := <-pa.events:
		if !reflect.DeepEqual(got, want) {
			t.Errorf("event = %+v, want %+v", got, want)
		}
	default:
		t.Fatal("no event")
	}
	list, err := pa.ListProcessing()
	wantList := []ProcessingFeature{{Name: featureMono, State: stateActive}, {Name: featureNetworkSink, State: stateDisabled}}
	if err != nil || !reflect.DeepEqual(list, wantList) {
		t.Errorf("ListProcessing = %v, %v; want %v", list, err, wantList)
	}
}

func TestProcessing_Disabled(t *testing.T) {
	pa := &PulseAudioBackend{}

	var disabled *DisabledError
	if err := pa.EnableProcessing("mono"); !errors.As(err, &disabled) {
		t.Errorf("EnableProcessing error = %v, want a DisabledError", err)
	}
	if _, err := pa.ListProcessing(); !errors.As(err, &disabled) {
		t.Errorf("ListProcessing error = %v, want a DisabledError", err)
	}
}
