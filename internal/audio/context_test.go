// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"
)

func nullDriver() Option { return WithDriver(&NullDriver{}) }

func TestNewContext_Defaults(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	if ctx.SampleRate() != 44100 {
		t.Errorf("SampleRate = %d, want 44100", ctx.SampleRate())
	}
	if ctx.Channels() != 2 {
		t.Errorf("Channels = %d, want 2", ctx.Channels())
	}
}

func TestNewContext_WithOptions(t *testing.T) {
	ctx, err := NewContext(
		nullDriver(),
		WithSampleRate(48000),
		WithChannels(1),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	if ctx.SampleRate() != 48000 {
		t.Errorf("SampleRate = %d, want 48000", ctx.SampleRate())
	}
	if ctx.Channels() != 1 {
		t.Errorf("Channels = %d, want 1", ctx.Channels())
	}
}

func TestNewContext_InvalidOptions(t *testing.T) {
	// Zero or negative values should be ignored, keeping defaults
	ctx, err := NewContext(
		nullDriver(),
		WithSampleRate(0),
		WithChannels(-1),
	)
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	if ctx.SampleRate() != 44100 {
		t.Errorf("SampleRate = %d, want 44100 (default)", ctx.SampleRate())
	}
	if ctx.Channels() != 2 {
		t.Errorf("Channels = %d, want 2 (default)", ctx.Channels())
	}
}

func TestNewContext_WithDriver(t *testing.T) {
	drv := &recordingDriver{}
	ctx, err := NewContext(WithDriver(drv))
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	if !drv.opened {
		t.Error("driver was not opened")
	}
}

func TestNewContext_DriverOpenError(t *testing.T) {
	drv := &failDriver{openErr: errors.New("device not found")}
	_, err := NewContext(WithDriver(drv))
	if err == nil {
		t.Fatal("expected error when driver Open fails, got nil")
	}
}

func TestContext_DoubleClose(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}

	if err := ctx.Close(); err != nil {
		t.Errorf("first Close: %v", err)
	}
	// Second close should be a no-op
	if err := ctx.Close(); err != nil {
		t.Errorf("second Close: %v", err)
	}
}

func TestContext_NewPlayer(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	src := makePCMReader([]float32{0.5, -0.5})
	p := ctx.NewPlayer(src)
	if p == nil {
		t.Fatal("NewPlayer returned nil")
	}
}

func TestContext_PlayWAV(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	pcm := make([]byte, 4)
	binary.LittleEndian.PutUint16(pcm[0:2], 0x4000)
	binary.LittleEndian.PutUint16(pcm[2:4], 0x2000)
	wav := buildWAV(wavFormatPCM, 1, 44100, 16, pcm)

	p, err := ctx.PlayWAV(wav)
	if err != nil {
		t.Fatalf("PlayWAV: %v", err)
	}
	if !p.IsPlaying() {
		t.Error("player should be playing after PlayWAV")
	}
}

func TestContext_PlayWAV_InvalidData(t *testing.T) {
	ctx, err := NewContext(nullDriver())
	if err != nil {
		t.Fatalf("NewContext: %v", err)
	}
	defer ctx.Close()

	_, err = ctx.PlayWAV([]byte{0, 1, 2})
	if err == nil {
		t.Error("expected error for invalid WAV data, got nil")
	}
}

// recordingDriver records Open/Start/Close calls for verification.
type recordingDriver struct {
	opened  bool
	started bool
	closed  bool
	source  ReadFloat32er
}

func (d *recordingDriver) Open(_, _, _ int) error {
	d.opened = true
	return nil
}

func (d *recordingDriver) SetSource(src ReadFloat32er) {
	d.source = src
}

func (d *recordingDriver) Start() error {
	d.started = true
	return nil
}

func (d *recordingDriver) Stop() error { return nil }

func (d *recordingDriver) Close() error {
	d.closed = true
	return nil
}

// failDriver returns errors from Open.
type failDriver struct {
	openErr error
}

func (d *failDriver) Open(_, _, _ int) error    { return d.openErr }
func (d *failDriver) SetSource(_ ReadFloat32er) {}
func (d *failDriver) Start() error              { return nil }
func (d *failDriver) Stop() error               { return nil }
func (d *failDriver) Close() error              { return nil }

// makePCMReader creates an io.Reader that produces float32 PCM bytes.
func makePCMReader(samples []float32) *bytes.Reader {
	buf := make([]byte, len(samples)*4)
	for i, s := range samples {
		binary.LittleEndian.PutUint32(buf[i*4:], math.Float32bits(s))
	}
	return bytes.NewReader(buf)
}
