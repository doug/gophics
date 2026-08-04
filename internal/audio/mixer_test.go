// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"math"
	"testing"
)

func TestMixer_SingleSource(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, -0.5, 0.25, -0.25})
	p := ctx.NewPlayer(src)
	p.Play()

	m := NewMixer()
	m.AddSource(p)

	out := make([]float32, 4)
	m.Mix(out)

	want := []float32{0.5, -0.5, 0.25, -0.25}
	for i, w := range want {
		assertNearF32(t, i, out[i], w)
	}
}

func TestMixer_TwoSources(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src1 := makePCMReader([]float32{0.3, 0.3})
	p1 := ctx.NewPlayer(src1)
	p1.Play()

	src2 := makePCMReader([]float32{0.2, -0.2})
	p2 := ctx.NewPlayer(src2)
	p2.Play()

	m := NewMixer()
	m.AddSource(p1)
	m.AddSource(p2)

	out := make([]float32, 2)
	m.Mix(out)

	// 0.3 + 0.2 = 0.5, 0.3 + (-0.2) = 0.1
	assertNearF32(t, 0, out[0], 0.5)
	assertNearF32(t, 1, out[1], 0.1)
}

func TestMixer_PerSourceVolume(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{1.0, 1.0})
	p := ctx.NewPlayer(src)
	p.SetVolume(0.5)
	p.Play()

	m := NewMixer()
	m.AddSource(p)

	out := make([]float32, 2)
	m.Mix(out)

	// 1.0 * 0.5 = 0.5
	assertNearF32(t, 0, out[0], 0.5)
	assertNearF32(t, 1, out[1], 0.5)
}

func TestMixer_MasterVolume(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.8, 0.8})
	p := ctx.NewPlayer(src)
	p.Play()

	m := NewMixer()
	m.AddSource(p)
	m.SetMasterVolume(0.5)

	out := make([]float32, 2)
	m.Mix(out)

	// 0.8 * 1.0 (source vol) * 0.5 (master) = 0.4
	assertNearF32(t, 0, out[0], 0.4)
	assertNearF32(t, 1, out[1], 0.4)
}

func TestMixer_Clamping(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	// Two sources that sum beyond 1.0
	src1 := makePCMReader([]float32{0.8, -0.8})
	p1 := ctx.NewPlayer(src1)
	p1.Play()

	src2 := makePCMReader([]float32{0.8, -0.8})
	p2 := ctx.NewPlayer(src2)
	p2.Play()

	m := NewMixer()
	m.AddSource(p1)
	m.AddSource(p2)

	out := make([]float32, 2)
	m.Mix(out)

	// 0.8 + 0.8 = 1.6 → clamped to 1.0
	if out[0] != 1.0 {
		t.Errorf("out[0] = %v, want 1.0 (clamped)", out[0])
	}
	// -0.8 + -0.8 = -1.6 → clamped to -1.0
	if out[1] != -1.0 {
		t.Errorf("out[1] = %v, want -1.0 (clamped)", out[1])
	}
}

func TestMixer_PausedSourceSkipped(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, 0.5})
	p := ctx.NewPlayer(src)
	p.Play()
	p.Pause()

	m := NewMixer()
	m.AddSource(p)

	out := make([]float32, 2)
	m.Mix(out)

	// Paused source should produce silence
	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d] = %v, want 0 (paused)", i, v)
		}
	}
}

func TestMixer_StoppedSourceSkipped(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, 0.5})
	p := ctx.NewPlayer(src)
	p.Stop()

	m := NewMixer()
	m.AddSource(p)

	out := make([]float32, 2)
	m.Mix(out)

	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d] = %v, want 0 (stopped)", i, v)
		}
	}
}

func TestMixer_RemoveSource(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src1 := makePCMReader([]float32{0.5, 0.5})
	p1 := ctx.NewPlayer(src1)
	p1.Play()

	src2 := makePCMReader([]float32{0.3, 0.3})
	p2 := ctx.NewPlayer(src2)
	p2.Play()

	m := NewMixer()
	m.AddSource(p1)
	m.AddSource(p2)

	m.RemoveSource(p1)

	out := make([]float32, 2)
	m.Mix(out)

	// Only p2 should contribute
	assertNearF32(t, 0, out[0], 0.3)
	assertNearF32(t, 1, out[1], 0.3)
}

func TestMixer_RemoveNonexistent(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5})
	p := ctx.NewPlayer(src)

	m := NewMixer()
	// Removing a player that was never added should not panic
	m.RemoveSource(p)
}

func TestMixer_MasterVolumeClamp(t *testing.T) {
	m := NewMixer()

	m.SetMasterVolume(-0.5)
	if m.MasterVolume() != 0.0 {
		t.Errorf("MasterVolume = %v, want 0.0 (clamped from -0.5)", m.MasterVolume())
	}

	m.SetMasterVolume(2.0)
	if m.MasterVolume() != 1.0 {
		t.Errorf("MasterVolume = %v, want 1.0 (clamped from 2.0)", m.MasterVolume())
	}

	m.SetMasterVolume(0.7)
	if m.MasterVolume() != 0.7 {
		t.Errorf("MasterVolume = %v, want 0.7", m.MasterVolume())
	}
}

func TestMixer_EmptyMix(t *testing.T) {
	m := NewMixer()

	out := make([]float32, 4)
	// Pre-fill with non-zero to verify zeroing
	for i := range out {
		out[i] = 999.0
	}

	m.Mix(out)

	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d] = %v, want 0 (no sources)", i, v)
		}
	}
}

func TestMixer_NotPlayingSourceSkipped(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, 0.5})
	p := ctx.NewPlayer(src)
	// Not calling Play() — should be skipped

	m := NewMixer()
	m.AddSource(p)

	out := make([]float32, 2)
	m.Mix(out)

	for i, v := range out {
		if v != 0 {
			t.Errorf("out[%d] = %v, want 0 (not playing)", i, v)
		}
	}
}

func assertNearF32(t *testing.T, idx int, got, want float32) {
	t.Helper()
	const epsilon = 1e-6
	diff := math.Abs(float64(got) - float64(want))
	if diff > epsilon {
		t.Errorf("out[%d] = %v, want %v (diff %v > epsilon %v)", idx, got, want, diff, epsilon)
	}
}
