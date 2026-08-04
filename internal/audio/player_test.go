// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"testing"
)

func TestPlayer_InitialState(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.1, 0.2})
	p := ctx.NewPlayer(src)

	if p.IsPlaying() {
		t.Error("new player should not be playing before Play()")
	}
	if p.Volume() != 1.0 {
		t.Errorf("default Volume = %v, want 1.0", p.Volume())
	}
}

func TestPlayer_PlayPauseResume(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader(make([]float32, 4096))
	p := ctx.NewPlayer(src)

	// Play
	p.Play()
	if !p.IsPlaying() {
		t.Error("expected IsPlaying() == true after Play()")
	}

	// Pause
	p.Pause()
	if p.IsPlaying() {
		t.Error("expected IsPlaying() == false after Pause()")
	}

	// Resume
	p.Resume()
	if !p.IsPlaying() {
		t.Error("expected IsPlaying() == true after Resume()")
	}
}

func TestPlayer_Stop(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader(make([]float32, 4096))
	p := ctx.NewPlayer(src)

	p.Play()
	p.Stop()
	if p.IsPlaying() {
		t.Error("expected IsPlaying() == false after Stop()")
	}

	// Play after Stop should be a no-op (player is permanently done)
	p.Play()
	if p.IsPlaying() {
		t.Error("expected IsPlaying() == false after Stop() + Play()")
	}
}

func TestPlayer_PauseWithoutPlay(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader(make([]float32, 64))
	p := ctx.NewPlayer(src)

	// Pause before Play should be a no-op
	p.Pause()
	if p.IsPlaying() {
		t.Error("should not be playing after Pause() without Play()")
	}
}

func TestPlayer_ResumeWithoutPause(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader(make([]float32, 4096))
	p := ctx.NewPlayer(src)

	p.Play()
	// Resume without prior Pause should keep playing
	p.Resume()
	if !p.IsPlaying() {
		t.Error("Resume() without Pause() should keep playing")
	}
}

func TestPlayer_SetVolume(t *testing.T) {
	tests := []struct {
		name   string
		set    float64
		expect float64
	}{
		{"normal", 0.5, 0.5},
		{"zero", 0.0, 0.0},
		{"max", 1.0, 1.0},
		{"below zero clamps", -0.5, 0.0},
		{"above one clamps", 1.5, 1.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx, err := NewContext(nullDriver())
			if err != nil {
				t.Fatalf("NewContext: %v", err)
			}
			defer ctx.Close()

			src := makePCMReader(make([]float32, 64))
			p := ctx.NewPlayer(src)

			p.SetVolume(tt.set)
			got := p.Volume()
			if got != tt.expect {
				t.Errorf("Volume() = %v, want %v", got, tt.expect)
			}
		})
	}
}

func TestPlayer_ReadSamples(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	input := []float32{0.1, 0.2, 0.3, 0.4}
	src := makePCMReader(input)
	p := ctx.NewPlayer(src)
	p.Play()

	dst := make([]float32, 4)
	n := p.readSamples(dst)
	if n != 4 {
		t.Fatalf("readSamples returned %d, want 4", n)
	}

	for i, want := range input {
		if dst[i] != want {
			t.Errorf("dst[%d] = %v, want %v", i, dst[i], want)
		}
	}
}

func TestPlayer_ReadSamples_EOF(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5})
	p := ctx.NewPlayer(src)
	p.Play()

	// Read more samples than available
	dst := make([]float32, 8)
	n := p.readSamples(dst)
	if n != 1 {
		t.Errorf("readSamples returned %d, want 1", n)
	}

	// After EOF, readSamples should return 0
	n = p.readSamples(dst)
	if n != 0 {
		t.Errorf("readSamples after EOF returned %d, want 0", n)
	}
}

func TestPlayer_ReadSamples_Stopped(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, 0.5})
	p := ctx.NewPlayer(src)
	p.Stop()

	dst := make([]float32, 4)
	n := p.readSamples(dst)
	if n != 0 {
		t.Errorf("readSamples after Stop returned %d, want 0", n)
	}
}
