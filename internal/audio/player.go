// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import (
	"encoding/binary"
	"io"
	"math"
	"sync"
)

// Player plays audio from an io.Reader source that provides float32 PCM
// samples encoded as little-endian bytes (4 bytes per sample).
type Player struct {
	mu      sync.Mutex
	ctx     *Context
	src     io.Reader
	volume  float64
	playing bool
	paused  bool
	done    bool
	buf     []byte // byte buffer for reading from src
}

// Play starts playback. If the player is paused, it resumes.
// If playback already finished, this is a no-op.
func (p *Player) Play() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.playing = true
	p.paused = false
}

// Pause suspends playback without losing position.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.playing && !p.done {
		p.paused = true
	}
}

// Resume continues playback after a pause.
func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.playing && p.paused && !p.done {
		p.paused = false
	}
}

// Stop ends playback permanently. The player cannot be restarted.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.playing = false
	p.paused = false
	p.done = true
}

// IsPlaying reports whether the player is actively producing audio
// (playing and not paused, not finished).
func (p *Player) IsPlaying() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.playing && !p.paused && !p.done
}

// SetVolume sets the playback volume. Values are clamped to [0.0, 1.0].
func (p *Player) SetVolume(v float64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	p.volume = v
}

// Volume returns the current playback volume.
func (p *Player) Volume() float64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volume
}

// readSamples fills dst with float32 samples from the source reader.
// Returns the number of samples written into dst. Marks the player done
// on EOF. Caller must NOT hold p.mu.
func (p *Player) readSamples(dst []float32) int {
	bytesNeeded := len(dst) * 4 // 4 bytes per float32
	p.mu.Lock()
	if p.done || p.src == nil {
		p.mu.Unlock()
		return 0
	}

	// Grow byte buffer if needed
	if len(p.buf) < bytesNeeded {
		p.buf = make([]byte, bytesNeeded)
	}
	src := p.src
	p.mu.Unlock()

	n, err := io.ReadFull(src, p.buf[:bytesNeeded])

	// Convert complete float32 values
	samples := n / 4
	for i := 0; i < samples; i++ {
		bits := binary.LittleEndian.Uint32(p.buf[i*4 : i*4+4])
		dst[i] = math.Float32frombits(bits)
	}

	if err != nil {
		p.mu.Lock()
		p.done = true
		p.playing = false
		p.mu.Unlock()
	}

	return samples
}
