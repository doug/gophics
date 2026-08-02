package sound

import (
	"math"
	"math/rand"
)

// This file synthesizes ambient music in Go — no audio files, matching the
// framework's "generate it in code" grain.

// drone is a continuous dark pad: a detuned root triad with a slow breathing
// tremolo. It never finishes.
type drone struct {
	oscs    []*Osc
	scratch []float32
	lfo     float64
	lfoHz   float64
}

// Drone returns an ambient pad centered on root (Hz).
func Drone(root float64) Source {
	d := &drone{lfoHz: 0.07}
	// Root, fifth, octave, plus slight detunes for a chorused beat.
	for _, f := range []float64{root, root * 1.5, root * 2, root * 1.004, root * 1.5 * 0.996} {
		d.oscs = append(d.oscs, &Osc{Wave: Triangle, Freq: f, Amp: 0.11})
	}
	return d
}

func (d *drone) Process(out []float32) bool {
	if cap(d.scratch) < len(out) {
		d.scratch = make([]float32, len(out))
	}
	for i := range out {
		out[i] = 0
	}
	sc := d.scratch[:len(out)]
	for _, o := range d.oscs {
		o.Process(sc)
		for i := range out {
			out[i] += sc[i]
		}
	}
	for i := range out {
		out[i] *= float32(0.55 + 0.45*math.Sin(2*math.Pi*d.lfo))
		if d.lfo += d.lfoHz / SampleRate; d.lfo >= 1 {
			d.lfo -= 1
		}
	}
	return true
}

// pluck is a soft sine note with an exponential decay (a distant bell).
type pluck struct {
	osc        Osc
	pos, total int
}

func newPluck(freq float64) *pluck {
	return &pluck{osc: Osc{Wave: Sine, Freq: freq, Amp: 0.18}, total: int(1.4 * SampleRate)}
}

func (p *pluck) Process(out []float32) bool {
	if p.pos >= p.total {
		return false
	}
	p.osc.Process(out)
	for i := range out {
		if p.pos >= p.total {
			for j := i; j < len(out); j++ {
				out[j] = 0
			}
			break
		}
		out[i] *= float32(math.Exp(-3.5 * float64(p.pos) / float64(p.total)))
		p.pos++
	}
	return p.pos < p.total
}

// music layers a drone with sparse notes drawn from a minor-pentatonic scale on
// a slow, randomized schedule — evolving dungeon ambience.
type music struct {
	drone   Source
	scale   []float64
	plucks  []*pluck
	next    int
	rng     *rand.Rand
	scratch []float32
}

// DungeonMusic returns a continuous ambient source seeded by seed.
func DungeonMusic(seed int64) Source {
	base := 220.0 // A3
	var scale []float64
	for _, semi := range []float64{0, 3, 5, 7, 10, 12, 15} { // minor pentatonic + octave
		scale = append(scale, base*math.Pow(2, semi/12))
	}
	return &music{
		drone: Drone(110),
		scale: scale,
		rng:   rand.New(rand.NewSource(seed)),
		next:  SampleRate,
	}
}

func (m *music) Process(out []float32) bool {
	m.drone.Process(out) // fills out
	if m.next -= len(out); m.next <= 0 {
		m.next = int((1.5 + m.rng.Float64()*3) * SampleRate) // a note every 1.5–4.5s
		m.plucks = append(m.plucks, newPluck(m.scale[m.rng.Intn(len(m.scale))]))
	}
	if cap(m.scratch) < len(out) {
		m.scratch = make([]float32, len(out))
	}
	sc := m.scratch[:len(out)]
	alive := m.plucks[:0]
	for _, p := range m.plucks {
		for i := range sc {
			sc[i] = 0
		}
		if p.Process(sc) {
			alive = append(alive, p)
		}
		for i := range out {
			out[i] += sc[i] * 0.5
		}
	}
	m.plucks = alive
	return true
}
