package sound

import "testing"

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func peak(buf []float32) float32 {
	var p float32
	for _, v := range buf {
		if a := absf(v); a > p {
			p = a
		}
	}
	return p
}

func TestMixerPlaysAndDrops(t *testing.T) {
	m := NewMixer(2)
	if m.Len() != 0 {
		t.Fatalf("new mixer len = %d", m.Len())
	}
	clip := Blip(440, 0.05) // ~2205 samples
	m.Play(clip, 1)
	if m.Len() != 1 {
		t.Fatalf("after Play len = %d, want 1", m.Len())
	}
	// Pull more frames than the clip has → it finishes and is dropped.
	buf := make([]float32, 2*(len(clip.Data)+64)) // interleaved stereo
	m.ReadFloat32s(buf)
	if p := peak(buf); p < 0.1 {
		t.Fatalf("expected audible output, peak = %v", p)
	}
	if m.Len() != 0 {
		t.Fatalf("finished clip not dropped: len = %d", m.Len())
	}
}

func TestStereoDuplicatesMono(t *testing.T) {
	m := NewMixer(2)
	m.Play(Blip(440, 0.02), 1)
	buf := make([]float32, 16)
	if n, _ := m.ReadFloat32s(buf); n != len(buf) {
		t.Fatalf("ReadFloat32s returned %d, want %d", n, len(buf))
	}
	for i := 0; i < len(buf); i += 2 {
		if buf[i] != buf[i+1] {
			t.Fatalf("L (%v) != R (%v) at frame %d", buf[i], buf[i+1], i/2)
		}
	}
}

func TestToneFinishes(t *testing.T) {
	src := Tone(440, 0.01, 0.5)
	out := make([]float32, int(0.01*SampleRate)+16) // longer than the tone
	if src.Process(out) {
		t.Fatal("a tone shorter than the block should report finished")
	}
}

func TestLoopStops(t *testing.T) {
	m := NewMixer(1)
	v := m.Loop(Blip(220, 0.01), 1)
	m.ReadFloat32s(make([]float32, 64))
	if m.Len() != 1 {
		t.Fatal("loop should keep playing")
	}
	v.Stop()
	m.ReadFloat32s(make([]float32, 64))
	if m.Len() != 0 {
		t.Fatalf("stopped loop not dropped: len = %d", m.Len())
	}
}
