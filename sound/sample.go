package sound

import "time"

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

// clip plays a Sample, optionally looped, at a playback rate (pitch) with linear
// interpolation. Volume and pan are applied by the Voice, not here.
type clip struct {
	data  []float32
	pos   float64
	pitch float64
	loop  bool
}

func (c *clip) Process(out []float32) bool {
	n := float64(len(c.data))
	pitch := c.pitch
	if pitch <= 0 {
		pitch = 1
	}
	for i := range out {
		if c.pos >= n {
			if !c.loop {
				for j := i; j < len(out); j++ {
					out[j] = 0
				}
				return false
			}
			c.pos -= n
		}
		idx := int(c.pos)
		frac := float32(c.pos - float64(idx))
		s0 := c.data[idx]
		s1 := s0
		if idx+1 < len(c.data) {
			s1 = c.data[idx+1]
		} else if c.loop {
			s1 = c.data[0]
		}
		out[i] = s0 + (s1-s0)*frac
		c.pos += pitch
	}
	return true
}

// Play starts a voice for s (Volume/Pan/Pitch/Loop from opts). Safe to call from
// the UI goroutine while audio plays.
func (m *Mixer) Play(s *Sample, opts PlayOptions) *Voice {
	if s == nil || len(s.Data) == 0 {
		return nil
	}
	return m.PlaySource(&clip{data: s.Data, pitch: opts.Pitch, loop: opts.Loop}, opts)
}
