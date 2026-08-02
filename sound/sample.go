package sound

import (
	"sync/atomic"
	"time"
)

// Sample is a loaded mono PCM clip at the mixer's SampleRate.
type Sample struct {
	Data []float32 // mono, [-1,1]
}

// NewSample wraps mono float32 PCM.
func NewSample(data []float32) *Sample { return &Sample{Data: data} }

// FromPCM16 converts signed 16-bit mono PCM to a Sample.
func FromPCM16(pcm []int16) *Sample {
	d := make([]float32, len(pcm))
	for i, v := range pcm {
		d[i] = float32(v) / 32768
	}
	return &Sample{Data: d}
}

// Duration is the clip length at SampleRate.
func (s *Sample) Duration() time.Duration {
	return time.Duration(len(s.Data)) * time.Second / SampleRate
}

// clip plays a Sample once (or looped) at a fixed gain.
type clip struct {
	data    []float32
	pos     int
	gain    float32
	loop    bool
	stopped atomic.Bool
}

func (c *clip) Process(out []float32) bool {
	if c.stopped.Load() {
		return false
	}
	for i := range out {
		if c.pos >= len(c.data) {
			if !c.loop {
				return false
			}
			c.pos = 0
		}
		out[i] = c.data[c.pos] * c.gain
		c.pos++
	}
	return true
}

// Voice is a handle to a playing sound; Stop ends it (loops need this).
type Voice struct{ c *clip }

// Stop silences the voice; the mixer drops it on the next block.
func (v *Voice) Stop() {
	if v != nil && v.c != nil {
		v.c.stopped.Store(true)
	}
}

// Play starts a one-shot voice for s at gain (0 → 1). Safe to call from the UI
// goroutine while audio plays.
func (m *Mixer) Play(s *Sample, gain float64) *Voice {
	if s == nil || len(s.Data) == 0 {
		return nil
	}
	if gain <= 0 {
		gain = 1
	}
	c := &clip{data: s.Data, gain: float32(gain)}
	m.Add(c)
	return &Voice{c}
}

// Loop starts a looping voice (e.g. ambience/music); Stop it via the Voice.
func (m *Mixer) Loop(s *Sample, gain float64) *Voice {
	if s == nil || len(s.Data) == 0 {
		return nil
	}
	if gain <= 0 {
		gain = 1
	}
	c := &clip{data: s.Data, gain: float32(gain), loop: true}
	m.Add(c)
	return &Voice{c}
}
