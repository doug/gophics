package sound

import "math"

// This file synthesizes small sound effects in Go — no audio assets, matching
// the framework's "generate it in code" grain (like the roguelike's tileset).

// renderMono renders src for `secs` seconds into a Sample.
func renderMono(src Source, secs float64) *Sample {
	n := int(secs * SampleRate)
	d := make([]float32, n)
	src.Process(d)
	return &Sample{Data: d}
}

// Blip is a short pure tone (menu tick, step).
func Blip(freq, secs float64) *Sample { return renderMono(Tone(freq, secs, 0.5), secs) }

// Coin is a quick two-note rising chime (pickup).
func Coin() *Sample {
	a := renderMono(Tone(880, 0.06, 0.5), 0.06)
	b := renderMono(Tone(1320, 0.10, 0.5), 0.10)
	return &Sample{Data: append(a.Data, b.Data...)}
}

// Thud is a low, quick descending blip (footstep on stone, descend).
func Thud() *Sample {
	n := int(0.14 * SampleRate)
	d := make([]float32, n)
	phase := 0.0
	for i := range d {
		f := 180 - 120*float64(i)/float64(n) // glide down
		env := 1 - float64(i)/float64(n)
		d[i] = float32(0.5 * env * math.Sin(2*math.Pi*phase))
		phase += f / SampleRate
	}
	return &Sample{Data: d}
}

// Hit is a short noise burst with a fast decay (attack/impact).
func Hit() *Sample {
	n := int(0.16 * SampleRate)
	d := make([]float32, n)
	var lcg uint32 = 0x1234567
	for i := range d {
		lcg = lcg*1664525 + 1013904223
		noise := float64(int32(lcg))/float64(1<<31) - 0 // ~[-1,1]
		env := math.Pow(1-float64(i)/float64(n), 2)
		d[i] = float32(0.55 * env * noise)
	}
	return &Sample{Data: d}
}
