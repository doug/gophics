// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

package audio

import "sync"

// Mixer combines multiple audio sources into a single output stream.
// Each source has its own volume, and the mixer applies a master volume
// after summing. Output is clamped to [-1.0, 1.0].
type Mixer struct {
	mu      sync.Mutex
	sources []*mixerSource
	master  float64
}

type mixerSource struct {
	player *Player
	buf    []float32
}

// NewMixer creates a mixer with master volume set to 1.0.
func NewMixer() *Mixer {
	return &Mixer{master: 1.0}
}

// AddSource registers a player as a mix input.
func (m *Mixer) AddSource(p *Player) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sources = append(m.sources, &mixerSource{player: p})
}

// RemoveSource unregisters a player from the mixer.
func (m *Mixer) RemoveSource(p *Player) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, s := range m.sources {
		if s.player == p {
			m.sources = append(m.sources[:i], m.sources[i+1:]...)
			return
		}
	}
}

// Mix reads samples from all active sources, sums them with per-source
// volume, applies master volume, and writes the result into out.
// Inactive or finished sources contribute silence.
func (m *Mixer) Mix(out []float32) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Zero the output buffer
	for i := range out {
		out[i] = 0
	}

	for _, s := range m.sources {
		if s.player == nil {
			continue
		}

		s.player.mu.Lock()
		if !s.player.playing || s.player.paused || s.player.done {
			s.player.mu.Unlock()
			continue
		}
		vol := float32(s.player.volume)
		s.player.mu.Unlock()

		// Ensure source buffer is large enough
		if len(s.buf) < len(out) {
			s.buf = make([]float32, len(out))
		}

		n := s.player.readSamples(s.buf[:len(out)])
		for i := 0; i < n; i++ {
			out[i] += s.buf[i] * vol
		}
	}

	// Apply master volume and clamp
	masterVol := float32(m.master)
	for i := range out {
		v := out[i] * masterVol
		if v > 1.0 {
			v = 1.0
		} else if v < -1.0 {
			v = -1.0
		}
		out[i] = v
	}
}

// SetMasterVolume sets the master output volume (0.0 to 1.0).
func (m *Mixer) SetMasterVolume(v float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if v < 0 {
		v = 0
	} else if v > 1 {
		v = 1
	}
	m.master = v
}

// MasterVolume returns the current master volume.
func (m *Mixer) MasterVolume() float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.master
}

// ReadFloat32s fills buf with mixed audio from all active sources.
// It implements ReadFloat32er so the Mixer can be used as a Driver source.
func (m *Mixer) ReadFloat32s(buf []float32) (int, error) {
	m.Mix(buf)
	return len(buf), nil
}
