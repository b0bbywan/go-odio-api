package sendspin

import "testing"

func TestWriteConvertsAndClampsSamples(t *testing.T) {
	o := newPulseOutput("test")
	o.maxFIFO = 100

	// 2^23 is full scale; 2^22 is half; values beyond full scale clamp to ±1.
	if err := o.Write([]int32{0, 1 << 22, 1 << 23, -(1 << 23), 1 << 24, -(1 << 24)}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	want := []float32{0, 0.5, 1, -1, 1, -1}
	if len(o.fifo) != len(want) {
		t.Fatalf("fifo len = %d, want %d", len(o.fifo), len(want))
	}
	for i, w := range want {
		if o.fifo[i] != w {
			t.Errorf("fifo[%d] = %v, want %v", i, o.fifo[i], w)
		}
	}
}

func TestReadDrainsAppliesGainAndZeroFills(t *testing.T) {
	o := newPulseOutput("test")
	o.maxFIFO = 100
	o.SetVolume(50) // gain 0.5
	if err := o.Write([]int32{1 << 23, 1 << 23}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	out := make([]float32, 4)
	n, err := o.read(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n != len(out) {
		t.Fatalf("read returned %d, want %d (must fill the whole buffer)", n, len(out))
	}
	// Two buffered samples at gain 0.5, then silence padding.
	want := []float32{0.5, 0.5, 0, 0}
	for i, w := range want {
		if out[i] != w {
			t.Errorf("out[%d] = %v, want %v", i, out[i], w)
		}
	}
	if len(o.fifo) != 0 {
		t.Errorf("fifo not drained: %d left", len(o.fifo))
	}
}

func TestReadMuteSilences(t *testing.T) {
	o := newPulseOutput("test")
	o.maxFIFO = 100
	o.SetMuted(true)
	if err := o.Write([]int32{1 << 23, 1 << 23}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	out := make([]float32, 2)
	if _, err := o.read(out); err != nil {
		t.Fatalf("read: %v", err)
	}
	for i, v := range out {
		if v != 0 {
			t.Errorf("muted out[%d] = %v, want 0", i, v)
		}
	}
}

func TestWriteOverflowDropsOldest(t *testing.T) {
	o := newPulseOutput("test")
	o.maxFIFO = 4

	if err := o.Write([]int32{1 << 23, 1 << 23}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	// Half scale marks the newer batch so we can prove the oldest were dropped.
	if err := o.Write([]int32{1 << 22, 1 << 22, 1 << 22, 1 << 22}); err != nil {
		t.Fatalf("Write: %v", err)
	}

	if len(o.fifo) != o.maxFIFO {
		t.Fatalf("fifo len = %d, want capped at %d", len(o.fifo), o.maxFIFO)
	}
	for i, v := range o.fifo {
		if v != 0.5 {
			t.Errorf("fifo[%d] = %v, want 0.5 (oldest full-scale samples should be gone)", i, v)
		}
	}
}

func TestSetVolumeClamps(t *testing.T) {
	o := newPulseOutput("test")
	o.SetVolume(150)
	if o.gain != 1 {
		t.Errorf("gain = %v, want 1 after clamping 150", o.gain)
	}
	o.SetVolume(-10)
	if o.gain != 0 {
		t.Errorf("gain = %v, want 0 after clamping -10", o.gain)
	}
}
