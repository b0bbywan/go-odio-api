package pulseaudio

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.yaml.in/yaml/v3"

	"github.com/b0bbywan/go-odio-api/events"
)

const dropIn = "90-processing-state.yaml"

func readDropIn(t *testing.T, path string) map[string]toggle {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading drop-in: %v", err)
	}
	var st stateDropIn
	if err := yaml.Unmarshal(data, &st); err != nil {
		t.Fatalf("parsing drop-in: %v", err)
	}
	return st.PulseAudio.Processing
}

func TestSaveToggle_CreatesConfD(t *testing.T) {
	path := filepath.Join(t.TempDir(), "conf.d", dropIn)

	saveToggle(path, featureNetworkSink, true)

	if got, want := readDropIn(t, path), map[string]toggle{"networkSink": {Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Errorf("drop-in = %v, want %v", got, want)
	}
}

func TestSaveToggle_KeepsTheOtherToggle(t *testing.T) {
	path := filepath.Join(t.TempDir(), dropIn)
	saveToggle(path, featureNetworkSink, false)

	saveToggle(path, featureMono, true)

	want := map[string]toggle{"mono": {Enabled: true}, "networkSink": {Enabled: false}}
	if got := readDropIn(t, path); !reflect.DeepEqual(got, want) {
		t.Errorf("drop-in = %v, want %v", got, want)
	}
}

func TestSaveToggle_RewritesUnreadable(t *testing.T) {
	path := filepath.Join(t.TempDir(), dropIn)
	if err := os.WriteFile(path, []byte("pulseaudio: [not a map"), 0o644); err != nil {
		t.Fatal(err)
	}

	saveToggle(path, featureMono, true)

	if got, want := readDropIn(t, path), map[string]toggle{"mono": {Enabled: true}}; !reflect.DeepEqual(got, want) {
		t.Errorf("drop-in = %v, want %v", got, want)
	}
}

func TestEnableProcessing_MonoOnCachedDefault(t *testing.T) {
	srv := newFakeServer(hwSink)
	pa := &PulseAudioBackend{events: make(chan events.Event, 2)}
	pa.outputCache.Store([]AudioOutput{{Name: "alsa_output.hdmi"}, {Name: hwSink, Default: true}})
	pa.processing = newTestProcessing(t, srv, false, false)
	pa.processing.cfg.StateFile = filepath.Join(t.TempDir(), dropIn)
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
	if got := readDropIn(t, pa.processing.cfg.StateFile); !got["mono"].Enabled {
		t.Errorf("drop-in = %v, want mono enabled", got)
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
