package main

import (
	"math"
	"math/rand"
)

// Metric identifies a health series.
type Metric int

const (
	HeartRate Metric = iota // beats per minute, streamed live
	Steps                   // cumulative step count for today
	Weight                  // body mass in kg, one point per day
	Sleep                   // hours slept, one point per night
)

// Sample is one reading. T is a metric-relative logical coordinate (seconds for
// the live heart rate, days for weight/sleep, hours for steps) rather than wall
// time, so the UI renders identically live, headless, and in tests.
type Sample struct {
	T float64
	V float64
}

// Provider is the source of health data. The synthetic provider below backs the
// desktop and web builds; on iOS/Android the SAME interface is implemented over
// HealthKit and Health Connect (Phase 2), so the widget tree never changes —
// one Go UI, real device data on the phone.
type Provider interface {
	// Name identifies the source ("Apple Health", "Health Connect", or the
	// synthetic "Sample data") for display.
	Name() string
	// Series returns the retained history for a metric, oldest first.
	Series(m Metric) []Sample
	// Latest returns the most recent value for a metric.
	Latest(m Metric) (Sample, bool)
	// Authorized reports whether the user granted read access.
	Authorized() bool
}

// Advancer is an optional capability: a live source the app advances once per
// frame so synthetic data streams in real time. Real device providers push
// samples via platform callbacks instead and won't implement this.
type Advancer interface{ Advance(dt float64) }

// hrWindow is how many seconds of heart-rate history the live chart shows.
const hrWindow = 60.0

// synthProvider generates realistic-looking data and streams a live heart rate.
// It is deterministic from a fixed seed so thumbnails and tests are stable.
type synthProvider struct {
	clock              float64 // seconds since start, advanced by Advance
	hr                 []Sample
	steps              []Sample
	weight             []Sample
	sleep              []Sample
	hrAccum, stepAccum float64
	stepsToday         float64
	rng                *rand.Rand
	baseHR             float64
}

func newSynthProvider() *synthProvider {
	p := &synthProvider{rng: rand.New(rand.NewSource(42)), baseHR: 68}

	// Weight: 91 daily points (3 months) drifting ~78 → 75 with day-to-day noise.
	for d := 90; d >= 0; d-- {
		w := 75.0 + float64(d)*0.035 + p.rng.NormFloat64()*0.18
		p.weight = append(p.weight, Sample{T: -float64(d), V: w})
	}
	// Sleep: last 30 nights, 6.4–8.2 hours.
	for n := 29; n >= 0; n-- {
		p.sleep = append(p.sleep, Sample{T: -float64(n), V: 6.4 + p.rng.Float64()*1.8})
	}
	// Steps: cumulative over the ~14 waking hours so far today.
	cum := 0.0
	for h := 0; h <= 14; h++ {
		cum += 250 + p.rng.Float64()*950
		p.steps = append(p.steps, Sample{T: float64(h), V: cum})
	}
	p.stepsToday = cum
	// Heart rate: prefill the live window ending at t=0.
	for t := -hrWindow; t <= 0; t++ {
		p.hr = append(p.hr, Sample{T: t, V: p.hrAt(t)})
	}
	return p
}

func (p *synthProvider) Name() string { return "Sample data · live" }

// hrAt models a resting heart rate: a slow drift plus a faster ripple plus
// beat-to-beat variability.
func (p *synthProvider) hrAt(t float64) float64 {
	return p.baseHR + 7*math.Sin(t*0.08) + 2.5*math.Sin(t*0.6) + p.rng.NormFloat64()*1.1
}

// Advance streams new data forward by dt seconds (called once per frame).
func (p *synthProvider) Advance(dt float64) {
	p.clock += dt

	// Emit ~1 heart-rate sample per second, dropping ones older than the window.
	for p.hrAccum += dt; p.hrAccum >= 1; p.hrAccum -= 1 {
		t := p.clock
		p.hr = append(p.hr, Sample{T: t, V: p.hrAt(t)})
		cut := t - hrWindow
		i := 0
		for i < len(p.hr) && p.hr[i].T < cut {
			i++
		}
		p.hr = p.hr[i:]
	}
	// Steps climb a couple per second while active.
	for p.stepAccum += dt; p.stepAccum >= 1; p.stepAccum -= 1 {
		p.stepsToday += 1 + p.rng.Float64()*3
	}
}

func (p *synthProvider) Series(m Metric) []Sample {
	switch m {
	case HeartRate:
		return p.hr
	case Steps:
		return p.steps
	case Weight:
		return p.weight
	case Sleep:
		return p.sleep
	}
	return nil
}

func (p *synthProvider) Latest(m Metric) (Sample, bool) {
	if m == Steps {
		return Sample{V: p.stepsToday}, true
	}
	s := p.Series(m)
	if len(s) == 0 {
		return Sample{}, false
	}
	return s[len(s)-1], true
}

func (p *synthProvider) Authorized() bool { return true }

// Compile-time proof the synthetic provider satisfies both interfaces.
var (
	_ Provider = (*synthProvider)(nil)
	_ Advancer = (*synthProvider)(nil)
)
