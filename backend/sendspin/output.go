package sendspin

import (
	"fmt"
	"sync"

	ssoutput "github.com/Sendspin/sendspin-go/pkg/audio/output"
	"github.com/jfreymuth/pulse"

	"github.com/b0bbywan/go-odio-api/logger"
)

// sendspin delivers samples left-justified in 24 bits regardless of the
// declared bit depth (see audio.SampleFromInt16 / SampleFrom24Bit), so 2^23 is
// always full scale when converting to float32.
const sampleFullScale = float32(1 << 23)

var _ ssoutput.Output = (*pulseOutput)(nil)

// pulseOutput adapts sendspin's push-based output onto jfreymuth/pulse's
// pull-based playback. Write converts and appends interleaved samples to a
// bounded FIFO; the pulse read callback drains it, zero-filling on underrun so
// the stream never stops mid-track. Volume and mute are applied as gain in the
// callback so changes take effect on already-buffered audio.
type pulseOutput struct {
	appName string
	latency float64

	// lifeMu guards the pulse client/stream. It is never held across a blocking
	// stream Stop/Close, which can wait for the read callback and would deadlock
	// against dataMu.
	lifeMu sync.Mutex
	client *pulse.Client
	stream *pulse.PlaybackStream

	dataMu  sync.Mutex
	fifo    []float32
	maxFIFO int
	gain    float32
	muted   bool
}

func newPulseOutput(appName string) *pulseOutput {
	return &pulseOutput{appName: appName, latency: 0.1, gain: 1}
}

func (o *pulseOutput) Open(sampleRate, channels, bitDepth int) error {
	var chanOpt pulse.PlaybackOption
	switch channels {
	case 1:
		chanOpt = pulse.PlaybackMono
	case 2:
		chanOpt = pulse.PlaybackStereo
	default:
		return fmt.Errorf("unsupported channel count: %d", channels)
	}

	o.lifeMu.Lock()
	defer o.lifeMu.Unlock()

	o.stopStreamLocked()

	if o.client == nil {
		c, err := pulse.NewClient(pulse.ClientApplicationName(o.appName))
		if err != nil {
			return fmt.Errorf("pulse connect: %w", err)
		}
		o.client = c
	}

	o.dataMu.Lock()
	o.maxFIFO = sampleRate * channels // ~1s ceiling so a stalled sink can't grow it unbounded
	o.fifo = o.fifo[:0]
	o.dataMu.Unlock()

	stream, err := o.client.NewPlayback(
		pulse.Float32Reader(o.read),
		pulse.PlaybackSampleRate(sampleRate),
		chanOpt,
		pulse.PlaybackLatency(o.latency),
	)
	if err != nil {
		return fmt.Errorf("pulse playback: %w", err)
	}
	o.stream = stream
	stream.Start()

	logger.Info("[sendspin] output opened: %dHz/%dch/%d-bit (pulse)", sampleRate, channels, bitDepth)
	return nil
}

// read is the pulse pull callback. It drains the FIFO, applies gain/mute and
// pads any remainder with silence, keeping the stream running across underruns.
func (o *pulseOutput) read(out []float32) (int, error) {
	o.dataMu.Lock()
	n := copy(out, o.fifo)
	o.fifo = o.fifo[:copy(o.fifo, o.fifo[n:])]
	gain := o.gain
	if o.muted {
		gain = 0
	}
	o.dataMu.Unlock()

	for i := 0; i < n; i++ {
		out[i] *= gain
	}
	for i := n; i < len(out); i++ {
		out[i] = 0
	}
	return len(out), nil
}

func (o *pulseOutput) Write(samples []int32) error {
	o.dataMu.Lock()
	defer o.dataMu.Unlock()

	if o.maxFIFO > 0 && len(o.fifo)+len(samples) > o.maxFIFO {
		overflow := len(o.fifo) + len(samples) - o.maxFIFO
		if overflow >= len(o.fifo) {
			o.fifo = o.fifo[:0]
		} else {
			o.fifo = o.fifo[:copy(o.fifo, o.fifo[overflow:])]
		}
		logger.Warn("[sendspin] output FIFO overflow, dropping %d samples", overflow)
	}

	for _, s := range samples {
		f := float32(s) / sampleFullScale
		if f > 1 {
			f = 1
		} else if f < -1 {
			f = -1
		}
		o.fifo = append(o.fifo, f)
	}
	return nil
}

func (o *pulseOutput) SetVolume(volume int) {
	switch {
	case volume < 0:
		volume = 0
	case volume > 100:
		volume = 100
	}
	o.dataMu.Lock()
	o.gain = float32(volume) / 100
	o.dataMu.Unlock()
}

func (o *pulseOutput) SetMuted(muted bool) {
	o.dataMu.Lock()
	o.muted = muted
	o.dataMu.Unlock()
}

func (o *pulseOutput) Close() error {
	o.lifeMu.Lock()
	o.stopStreamLocked()
	client := o.client
	o.client = nil
	o.lifeMu.Unlock()

	if client != nil {
		client.Close()
	}
	return nil
}

// stopStreamLocked tears down the active stream. Caller holds lifeMu; the
// blocking pulse calls run without dataMu held.
func (o *pulseOutput) stopStreamLocked() {
	if o.stream == nil {
		return
	}
	stream := o.stream
	o.stream = nil
	stream.Stop()
	stream.Close()
}
