// Package trace is the shared record of one scroll gesture and what a scroll
// view did with it — the contract between gophics and a native twin.
//
// The measurement harness works by recording once and replaying everywhere. A
// native twin (tools/native-twin) logs the finger-phase deltas of one real
// flick with timestamps, and the offset its scroll view showed per frame
// through the momentum. gophics is then handed the identical finger deltas
// through app.Headless, steps its frame clock at the same rate, and logs its
// own offset per frame. Same input in, two curves out; Metrics turns each
// curve into numbers that can be compared with a tolerance.
//
// That replaces "tuned to NSScrollView; feel-test" — the annotation on
// gophics's fling constants — with a measured decay. On a Mac trackpad the
// desktop shell passes the OS's momentum events straight through, so the
// constants matter for touch: mobile and web-touch, where there is no OS
// momentum and gophics imitates Apple's curve itself.
//
// Sign convention, stated once because every other bug here is a sign bug:
// Input deltas are finger movement in screen coordinates, so a flick upward is
// negative DY. Offset is the scroll position, increasing as content moves up.
// A negative-DY flick therefore produces an increasing Offset.
package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"sort"
)

// Sample is one value at a time, in seconds from the gesture's first touch.
type Sample struct {
	T float64 `json:"t"`
	V float64 `json:"v"`
}

// Trace is one gesture and one scroll view's response to it.
type Trace struct {
	// Source names what produced Offset: "gophics", "macos-appkit", ...
	Source string `json:"source"`
	// Hz is the frame rate Offset was sampled at.
	Hz float64 `json:"hz"`
	// Notes is free text: device, OS version, how the gesture was made.
	Notes string `json:"notes,omitempty"`
	// Input is the finger phase: DY per event, in logical pixels, negative
	// for an upward flick. Events carry their own timestamps because real
	// input arrives in bursts and is not frame-aligned.
	Input []Sample `json:"input"`
	// Offset is the scroll position per frame, from first touch through the
	// end of momentum. V holds the offset.
	Offset []Sample `json:"offset"`
	// ReleaseT is when the finger lifted: the boundary between the finger
	// phase and the momentum the curve is judged on.
	ReleaseT float64 `json:"release_t"`
}

// Read loads a trace from JSON.
func Read(r io.Reader) (*Trace, error) {
	var t Trace
	if err := json.NewDecoder(r).Decode(&t); err != nil {
		return nil, err
	}
	sort.Slice(t.Input, func(i, j int) bool { return t.Input[i].T < t.Input[j].T })
	sort.Slice(t.Offset, func(i, j int) bool { return t.Offset[i].T < t.Offset[j].T })
	return &t, nil
}

// ReadFile loads a trace from path.
func ReadFile(path string) (*Trace, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return Read(f)
}

// Write stores the trace as indented JSON.
func (t *Trace) Write(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", " ")
	return enc.Encode(t)
}

// WriteCSV emits t, offset, velocity per frame — the shape a spreadsheet or a
// plotting script wants, with velocity as the centred finite difference so a
// jittery tail shows as a jittery column rather than being smoothed away.
func (t *Trace) WriteCSV(w io.Writer) error {
	if _, err := fmt.Fprintln(w, "t,offset,velocity"); err != nil {
		return err
	}
	v := t.Velocity()
	for i, s := range t.Offset {
		if _, err := fmt.Fprintf(w, "%.5f,%.3f,%.2f\n", s.T, s.V, v[i]); err != nil {
			return err
		}
	}
	return nil
}

// Velocity is the offset's derivative per frame, px/s, by centred difference
// (one-sided at the ends).
func (t *Trace) Velocity() []float64 {
	n := len(t.Offset)
	v := make([]float64, n)
	if n < 2 {
		return v
	}
	for i := range n {
		lo, hi := i-1, i+1
		if lo < 0 {
			lo = 0
		}
		if hi >= n {
			hi = n - 1
		}
		dt := t.Offset[hi].T - t.Offset[lo].T
		if dt > 0 {
			v[i] = (t.Offset[hi].V - t.Offset[lo].V) / dt
		}
	}
	return v
}

// Metrics are the numbers a decay curve reduces to. Two traces of the same
// gesture agree when these agree; they are what the tolerance is applied to.
type Metrics struct {
	// ReleaseV is the velocity at the moment the finger lifted — the initial
	// condition the momentum phase starts from.
	ReleaseV float64
	// PeakV is the largest speed seen anywhere in the gesture.
	PeakV float64
	// Tau is the exponential time constant of the momentum decay in seconds:
	// v(t) = v0·e^(−t/Tau), fitted by least squares on ln|v|. For UIKit's
	// documented 0.998/ms this is 0.5s; gophics's flingFriction of 2.0 is the
	// same number by construction.
	Tau float64
	// TauR2 is the fit's coefficient of determination — how exponential the
	// decay actually was. A native curve that is not a clean exponential will
	// show here before it shows anywhere else.
	TauR2 float64
	// SettleT is seconds from release until speed stays under RestSpeed.
	SettleT float64
	// MomentumDist is how far the content travelled after release.
	MomentumDist float64
	// TotalDist is the whole gesture's travel.
	TotalDist float64
}

// RestSpeed is the speed below which the scroll is considered stopped, px/s.
// It matches gophics's flingMinSpeed so that "settled" means the same thing
// in both measurements.
const RestSpeed = 20

// Compute derives Metrics from the trace's offset series.
func (t *Trace) Compute() Metrics {
	var m Metrics
	n := len(t.Offset)
	if n < 3 {
		return m
	}
	v := t.Velocity()
	m.TotalDist = t.Offset[n-1].V - t.Offset[0].V

	// Index of the first frame at or after release.
	rel := sort.Search(n, func(i int) bool { return t.Offset[i].T >= t.ReleaseT })
	if rel >= n {
		rel = n - 1
	}
	m.ReleaseV = v[rel]
	for _, s := range v {
		if math.Abs(s) > m.PeakV {
			m.PeakV = math.Abs(s)
		}
	}
	m.MomentumDist = t.Offset[n-1].V - t.Offset[rel].V

	// Momentum phase: from release until speed drops under RestSpeed.
	end := n
	for i := rel; i < n; i++ {
		if math.Abs(v[i]) < RestSpeed {
			end = i
			break
		}
	}
	if end < n {
		m.SettleT = t.Offset[end].T - t.ReleaseT
	} else {
		m.SettleT = t.Offset[n-1].T - t.ReleaseT
	}

	// Least squares on ln|v| against t over the momentum phase.
	var sx, sy, sxx, sxy float64
	k := 0
	for i := rel; i < end; i++ {
		if math.Abs(v[i]) < 1 {
			continue
		}
		x, y := t.Offset[i].T-t.ReleaseT, math.Log(math.Abs(v[i]))
		sx += x
		sy += y
		sxx += x * x
		sxy += x * y
		k++
	}
	if k >= 3 {
		kf := float64(k)
		den := kf*sxx - sx*sx
		if den != 0 {
			slope := (kf*sxy - sx*sy) / den
			if slope < 0 {
				m.Tau = -1 / slope
			}
			// R² of the fit.
			mean := sy / kf
			intercept := (sy - slope*sx) / kf
			var ssTot, ssRes float64
			for i := rel; i < end; i++ {
				if math.Abs(v[i]) < 1 {
					continue
				}
				x, y := t.Offset[i].T-t.ReleaseT, math.Log(math.Abs(v[i]))
				ssTot += (y - mean) * (y - mean)
				pred := intercept + slope*x
				ssRes += (y - pred) * (y - pred)
			}
			if ssTot > 0 {
				m.TauR2 = 1 - ssRes/ssTot
			}
		}
	}
	return m
}

// String is the metrics as a short report.
func (m Metrics) String() string {
	return fmt.Sprintf(
		"release velocity  %8.1f px/s\n"+
			"peak velocity     %8.1f px/s\n"+
			"decay tau         %8.3f s   (R² %.3f)\n"+
			"settle time       %8.3f s\n"+
			"momentum distance %8.1f px\n"+
			"total distance    %8.1f px",
		m.ReleaseV, m.PeakV, m.Tau, m.TauR2, m.SettleT, m.MomentumDist, m.TotalDist)
}

// SyntheticFlick builds the finger phase of an idealized flick: the finger
// moves at v0 px/s (negative for upward) for dur seconds, reported at hz. Real
// flicks ramp; this does not, which is fine for a self-test and wrong as a
// reference — the reference is a recorded one.
func SyntheticFlick(v0, dur, hz float64) []Sample {
	dt := 1 / hz
	n := int(dur * hz)
	out := make([]Sample, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, Sample{T: float64(i) * dt, V: v0 * dt})
	}
	return out
}
