package healthui

import (
	"math"
	"math/rand"
	"sync"
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

// DeviceProvider is the Provider used on iOS/Android: the native host reads the
// platform health store (HealthKit / Health Connect) and pushes samples in via
// Push. It is NOT an Advancer — the platform drives updates — so the UI ticker
// only repaints. All access is mutex-guarded because Push is called from the
// host's callback threads while the UI reads on the frame thread.
type DeviceProvider struct {
	mu       sync.RWMutex
	name     string
	authed   bool
	series   [4][]Sample // indexed by Metric
	stepsSum float64     // Steps reports a running total, like the synthetic one
}

// NewDeviceProvider builds an empty device provider labelled with the platform
// store's name (e.g. "Apple Health", "Health Connect").
func NewDeviceProvider(name string) *DeviceProvider {
	return &DeviceProvider{name: name}
}

func (d *DeviceProvider) Name() string {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.name
}

func (d *DeviceProvider) Authorized() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.authed
}

// SetAuthorized records the result of the platform permission prompt.
func (d *DeviceProvider) SetAuthorized(ok bool) {
	d.mu.Lock()
	d.authed = ok
	d.mu.Unlock()
}

// Push appends a sample for a metric from the native health store. cap bounds
// the retained history (0 = unbounded); the newest samples are kept.
func (d *DeviceProvider) Push(m Metric, t, v float64, capN int) {
	if m < 0 || int(m) >= len(d.series) {
		return
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	s := append(d.series[m], Sample{T: t, V: v})
	if capN > 0 && len(s) > capN {
		s = s[len(s)-capN:]
	}
	d.series[m] = s
	if m == Steps {
		d.stepsSum = v // HealthKit/Health Connect report cumulative steps directly
	}
}

// ReplaceSeries swaps a metric's whole history at once — used when the host
// backfills a range query (e.g. 30 days of weight) rather than streaming.
func (d *DeviceProvider) ReplaceSeries(m Metric, xs []Sample) {
	if m < 0 || int(m) >= len(d.series) {
		return
	}
	d.mu.Lock()
	d.series[m] = append(d.series[m][:0], xs...)
	if m == Steps && len(xs) > 0 {
		d.stepsSum = xs[len(xs)-1].V
	}
	d.mu.Unlock()
}

func (d *DeviceProvider) Series(m Metric) []Sample {
	if m < 0 || int(m) >= len(d.series) {
		return nil
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return append([]Sample(nil), d.series[m]...) // copy: caller reads without the lock
}

func (d *DeviceProvider) Latest(m Metric) (Sample, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	if m == Steps {
		return Sample{V: d.stepsSum}, len(d.series[Steps]) > 0
	}
	if m < 0 || int(m) >= len(d.series) || len(d.series[m]) == 0 {
		return Sample{}, false
	}
	s := d.series[m]
	return s[len(s)-1], true
}

var _ Provider = (*DeviceProvider)(nil)
