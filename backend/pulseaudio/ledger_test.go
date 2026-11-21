package pulseaudio

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/jfreymuth/pulse/proto"
)

func ledgerPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), "odio-api", "processing.json")
}

func TestLedger_MissingFileIsEmpty(t *testing.T) {
	l := openLedger(ledgerPath(t))

	if len(l.features) != 0 {
		t.Errorf("features = %+v, want none", l.features)
	}
}

func TestLedger_CorruptFileIsEmpty(t *testing.T) {
	path := ledgerPath(t)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	if l := openLedger(path); len(l.features) != 0 {
		t.Errorf("features = %+v, want none", l.features)
	}
}

func TestLedger_SurvivesReopen(t *testing.T) {
	path := ledgerPath(t)
	l := openLedger(path)
	l.wanted(featureMono, true)
	l.add(featureMono, record{Index: 30, Name: remapModule, Args: monoArgs})
	l.wanted(featureNetworkSink, false)
	l.save()

	got := openLedger(path).features

	want := map[featureName]*ledgerEntry{
		featureMono:        {Loaded: true, Modules: []record{{Index: 30, Name: remapModule, Args: monoArgs}}},
		featureNetworkSink: {Loaded: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("reopened = %+v, want %+v", got, want)
	}
}

func TestLedger_WantedFallsBackToOnBoot(t *testing.T) {
	l := openLedger(ledgerPath(t))

	if !l.wanted(featureMono, true) {
		t.Error("first wanted(mono, true) = false")
	}
	l.features[featureMono].Loaded = false
	if l.wanted(featureMono, true) {
		t.Error("wanted(mono, true) = true, want the recorded false")
	}
}

func TestLedger_Prune(t *testing.T) {
	l := openLedger(ledgerPath(t))
	l.add(featureNetworkSink, record{Index: 21, Name: tcpModule, Args: "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713"})
	l.add(featureNetworkSink, record{Index: 22, Name: zeroconfModule, Args: ""})
	l.add(featureMono, record{Index: 30, Name: remapModule, Args: monoArgs})
	l.add(featureMono, record{Index: 31, Name: remapModule, Args: monoArgs})

	changed := l.prune([]*proto.GetModuleInfoReply{
		// same args, quoted and in another order
		module(21, tcpModule, `port="4713" auth-ip-acl=127.0.0.1;192.168.1.0/24`),
		// index reused by another module after a server restart
		module(22, tcpModule, "port=4714"),
		module(30, remapModule, monoArgs),
	})

	if !changed {
		t.Error("prune reported no change")
	}
	want := map[featureName][]record{
		featureNetworkSink: {{Index: 21, Name: tcpModule, Args: "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713"}},
		featureMono:        {{Index: 30, Name: remapModule, Args: monoArgs}},
	}
	for name, mods := range want {
		if got := l.features[name].Modules; !reflect.DeepEqual(got, mods) {
			t.Errorf("%s modules = %+v, want %+v", name, got, mods)
		}
	}
	if l.prune([]*proto.GetModuleInfoReply{module(21, tcpModule, "auth-ip-acl=127.0.0.1;192.168.1.0/24 port=4713"), module(30, remapModule, monoArgs)}) {
		t.Error("second prune reported a change")
	}
}

func TestLedger_Remove(t *testing.T) {
	l := openLedger(ledgerPath(t))
	l.add(featureNetworkSink, record{Index: 21, Name: tcpModule})
	l.add(featureNetworkSink, record{Index: 22, Name: zeroconfModule})

	l.remove(featureNetworkSink, 21)

	if got, want := l.features[featureNetworkSink].Modules, []record{{Index: 22, Name: zeroconfModule}}; !reflect.DeepEqual(got, want) {
		t.Errorf("modules = %+v, want %+v", got, want)
	}
}
