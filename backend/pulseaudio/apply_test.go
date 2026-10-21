package pulseaudio

import (
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	"github.com/jfreymuth/pulse/proto"
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

func (f *fakeServer) loadModule(name, args string) error {
	f.calls = append(f.calls, "load "+name+" "+args)
	if err := f.failLoad[name]; err != nil {
		return err
	}
	f.next++
	f.mods = append(f.mods, module(f.next, name, args))
	if name == remapModule {
		parsed, _ := parseModuleArgs(args)
		f.sinks = append(f.sinks, parsed["sink_name"])
	}
	return nil
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
	return newProcessing(cfg, srv)
}

// reconcile passes the fake's current default sink, as start passes the one it
// just read.
func reconcile(t *testing.T, p *processing, kind AudioServerKind) []ProcessingFeature {
	t.Helper()
	return p.reconcile(kind, p.srv.(*fakeServer).def, hostAddrs(t, "192.168.1.42/24"))
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

func TestReconcile_AdoptsLegacyModules(t *testing.T) {
	srv := newFakeServer(hwSink,
		module(21, tcpModule, legacyTCPArgs),
		module(22, zeroconfModule, ""),
		module(30, remapModule, workaroundMonoArgs),
	)
	srv.sinks = append(srv.sinks, "mono")
	srv.def = "mono"
	p := newTestProcessing(t, srv, true, true)

	changed := reconcile(t, p, ServerPulse)

	assertCalls(t, srv)
	assertStatus(t, p, featureMono, active)
	assertStatus(t, p, featureNetworkSink, active)
	want := []ProcessingFeature{{Name: featureMono, State: stateActive}, {Name: featureNetworkSink, State: stateActive}}
	if !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	if again := reconcile(t, p, ServerPulse); again != nil {
		t.Errorf("second reconcile changed %v, want nothing", again)
	}
}

// odios loads its own network sink: unless enabled, odio must not touch it.
func TestReconcile_LeavesNotEnabledFeatureAlone(t *testing.T) {
	srv := newFakeServer(hwSink, module(21, tcpModule, legacyTCPArgs))
	cfg := processingCfg()
	cfg.NetworkSink.Enabled = false
	p := newProcessing(cfg, srv)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+remapModule+" "+monoArgs, "default mono")
	if got := p.state.Load(); len(got) != 1 || got[0].Name != featureMono {
		t.Errorf("state = %+v, want mono alone", got)
	}
}

func TestReconcile_LoadsMonoOnTheDefaultSink(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, true, false)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+remapModule+" "+monoArgs, "default mono")
	assertStatus(t, p, featureMono, active)
	if srv.def != "mono" {
		t.Errorf("default sink = %s, want mono", srv.def)
	}
}

func TestReconcile_UnloadsMonoAndRestoresMaster(t *testing.T) {
	srv := newFakeServer(hwSink, module(30, remapModule, workaroundMonoArgs))
	srv.sinks = append(srv.sinks, "mono")
	srv.def = "mono"
	p := newTestProcessing(t, srv, false, false)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "default "+hwSink, "unload 30")
	assertStatus(t, p, featureMono, disabled)
	if srv.def != hwSink {
		t.Errorf("default sink = %s, want %s", srv.def, hwSink)
	}
}

func TestReconcile_UnloadsMonoWhenMasterGone(t *testing.T) {
	srv := newFakeServer("alsa_output.hdmi", module(30, remapModule, workaroundMonoArgs))
	srv.sinks = append(srv.sinks, "mono")
	srv.def = "mono"
	p := newTestProcessing(t, srv, false, false)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "default "+hwSink, "unload 30")
	assertStatus(t, p, featureMono, disabled)
}

func TestReconcile_ReloadsDriftedMono(t *testing.T) {
	srv := newFakeServer(hwSink, module(30, remapModule, "master="+hwSink+" sink_name=mono channels=2"))
	srv.sinks = append(srv.sinks, "mono")
	srv.def = "mono"
	p := newTestProcessing(t, srv, true, false)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv,
		"default "+hwSink, "unload 30",
		"load "+remapModule+" "+monoArgs, "default mono",
	)
	assertStatus(t, p, featureMono, active)
}

func TestReconcile_LoadFailure(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failLoad = map[string]error{tcpModule: errors.New("module initialization failed")}
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+tcpModule+" auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713")
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: "module initialization failed"})
}

func TestReconcile_OptionalLoadFailureOnlyWarns(t *testing.T) {
	srv := newFakeServer(hwSink)
	srv.failLoad = map[string]error{zeroconfModule: errors.New("avahi unreachable")}
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPulse)

	assertCalls(t, srv, "load "+tcpModule+" auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713", "load "+zeroconfModule+" ")
	assertStatus(t, p, featureNetworkSink, active)
}

func TestReconcile_FeatureErrorUnloads(t *testing.T) {
	srv := newFakeServer(hwSink, module(536870917, tcpModule, "auth-anonymous=1"))
	p := newTestProcessing(t, srv, false, true)

	reconcile(t, p, ServerPipeWire)

	assertCalls(t, srv, "unload 536870917")
	assertStatus(t, p, featureNetworkSink, ProcessingFeature{State: stateError, Error: errACLUnsupported.Error()})
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

	// the server restarts without any module: nothing comes back
	srv.mods, srv.calls = nil, nil
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
}

func TestRefresh_ExternalLoadTurnsOn(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, false)
	reconcile(t, p, ServerPulse)
	srv.mods = append(srv.mods, module(21, tcpModule, legacyTCPArgs))

	changed := p.refresh()

	if want := []ProcessingFeature{{Name: featureNetworkSink, State: stateActive}}; !reflect.DeepEqual(changed, want) {
		t.Errorf("changed = %v, want %v", changed, want)
	}
	assertStatus(t, p, featureNetworkSink, active)

	// the server restarts: the feature is now wanted
	srv.mods, srv.calls = nil, nil
	reconcile(t, p, ServerPulse)
	assertCalls(t, srv,
		"load "+tcpModule+" auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713",
		"load "+zeroconfModule+" ",
	)
}

func TestRefresh_ZeroconfAloneDoesNotTurnOn(t *testing.T) {
	srv := newFakeServer(hwSink)
	p := newTestProcessing(t, srv, false, false)
	reconcile(t, p, ServerPulse)
	srv.mods = append(srv.mods, module(22, zeroconfModule, ""))

	if changed := p.refresh(); changed != nil {
		t.Errorf("changed = %v, want nothing", changed)
	}
	assertStatus(t, p, featureNetworkSink, disabled)
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
