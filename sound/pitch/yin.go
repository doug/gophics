// Package pitch estimates the fundamental frequency of monophonic audio — a
// sung or played note — and names it musically.
//
// It implements YIN (de Cheveigné & Kawahara, 2002), an autocorrelation method
// chosen over a bare FFT peak because a voice's loudest partial is often not its
// fundamental: sing an open vowel and the first or second harmonic can dominate
// the spectrum, so a spectral peak-picker reports the note an octave high. YIN
// works in the time domain on periodicity itself, which is what "the note being
// sung" actually means.
//
// The package is pure Go with no device dependency, so detection is
// deterministic and unit-testable against synthesized tones with no audio
// hardware — see sound.Osc for a generator.
package pitch

import "math"

// Result is one pitch estimate over a window of samples.
type Result struct {
	// Freq is the estimated fundamental in Hz, or 0 when Voiced is false.
	Freq float64
	// Clarity is how strongly periodic the window was, 0..1. Near 1 means a
	// clean sustained tone; low values mean noise, silence, or a consonant.
	Clarity float64
	// Voiced reports whether the window held a pitch worth trusting: periodic
	// enough, loud enough, and inside [MinFreq, MaxFreq].
	Voiced bool
	// RMS is the window's root-mean-square level, 0..1. Useful for a level
	// meter and for telling "silent" apart from "loud but unpitched".
	RMS float64
}

// Detector estimates pitch over successive windows. It holds scratch buffers, so
// reuse one across frames rather than allocating per call. A Detector is not
// safe for concurrent use.
type Detector struct {
	// SampleRate is the input rate in Hz. Required.
	SampleRate int
	// MinFreq and MaxFreq bound the search. Defaults cover the human singing
	// range with margin: 65 Hz (C2, low bass) to 1200 Hz (roughly D6, above a
	// soprano's staff). Narrowing them speeds detection and rejects octave
	// errors.
	MinFreq, MaxFreq float64
	// Threshold is YIN's absolute threshold on the normalized difference: the
	// first dip below it wins. Lower is stricter. 0 uses DefaultThreshold.
	Threshold float64
	// MinRMS is the quietest window considered voiced, 0..1. 0 uses
	// DefaultMinRMS.
	MinRMS float64

	diff []float64 // difference function d(tau)
	cmnd []float64 // cumulative mean normalized difference d'(tau)
}

// Defaults applied when the corresponding Detector field is zero.
const (
	DefaultThreshold = 0.15
	DefaultMinRMS    = 0.01
	DefaultMinFreq   = 65.0
	DefaultMaxFreq   = 1200.0
)

func (d *Detector) minFreq() float64 {
	if d.MinFreq > 0 {
		return d.MinFreq
	}
	return DefaultMinFreq
}

func (d *Detector) maxFreq() float64 {
	if d.MaxFreq > 0 {
		return d.MaxFreq
	}
	return DefaultMaxFreq
}

func (d *Detector) threshold() float64 {
	if d.Threshold > 0 {
		return d.Threshold
	}
	return DefaultThreshold
}

func (d *Detector) minRMS() float64 {
	if d.MinRMS > 0 {
		return d.MinRMS
	}
	return DefaultMinRMS
}

// WindowFor reports the smallest window length that can resolve freq at rate.
// YIN needs at least two full periods to see periodicity at all; this returns
// the doubled period rounded up, so sizing a capture buffer for the lowest note
// an app cares about is arithmetic rather than guesswork.
func WindowFor(freq float64, rate int) int {
	if freq <= 0 || rate <= 0 {
		return 0
	}
	return int(math.Ceil(2 * float64(rate) / freq))
}

// Detect estimates the pitch of one window of mono samples in [-1,1].
//
// The window must span at least two periods of MinFreq — WindowFor computes
// that length. A shorter window cannot express the lowest notes and Detect
// reports them unvoiced rather than guessing.
func (d *Detector) Detect(x []float32) Result {
	var res Result
	if d.SampleRate <= 0 || len(x) < 2 {
		return res
	}

	// RMS first: it is cheap, and a silent window needs no correlation at all.
	var sum float64
	for _, v := range x {
		sum += float64(v) * float64(v)
	}
	res.RMS = math.Sqrt(sum / float64(len(x)))
	if res.RMS < d.minRMS() {
		return res
	}

	rate := float64(d.SampleRate)
	// Lag bounds come from the frequency bounds: high frequency = short lag.
	tauMin := int(rate / d.maxFreq())
	tauMax := int(rate/d.minFreq()) + 1
	if tauMin < 1 {
		tauMin = 1
	}
	// The difference function at lag tau compares x[j] with x[j+tau], so the
	// largest usable lag is half the window: beyond that, too few pairs remain
	// for the comparison to mean anything.
	if half := len(x) / 2; tauMax > half {
		tauMax = half
	}
	if tauMax <= tauMin {
		return res
	}

	d.grow(tauMax + 1)
	d.difference(x, tauMax)
	d.normalize(tauMax)

	tau, ok := d.absoluteThreshold(tauMin, tauMax)
	if !ok {
		return res
	}
	period := d.refine(tau, tauMax)
	if period <= 0 {
		return res
	}

	freq := rate / period
	if freq < d.minFreq() || freq > d.maxFreq() {
		return res
	}
	// d'(tau) near 0 is a perfect match, near 1 is no periodicity: invert it so
	// larger means more confident, which is what a UI wants to show.
	clarity := 1 - d.cmnd[tau]
	if clarity < 0 {
		clarity = 0
	} else if clarity > 1 {
		clarity = 1
	}
	res.Freq, res.Clarity, res.Voiced = freq, clarity, true
	return res
}

func (d *Detector) grow(n int) {
	if cap(d.diff) < n {
		d.diff = make([]float64, n)
		d.cmnd = make([]float64, n)
		return
	}
	d.diff, d.cmnd = d.diff[:n], d.cmnd[:n]
}

// difference computes YIN's squared-difference function
//
//	d(tau) = sum_j (x[j] - x[j+tau])^2
//
// over the pairs that fit in the window. Unlike plain autocorrelation this dips
// toward zero at the period instead of peaking, and it is insensitive to
// amplitude drift within the window.
func (d *Detector) difference(x []float32, tauMax int) {
	n := len(x) - tauMax
	d.diff[0] = 0
	for tau := 1; tau <= tauMax; tau++ {
		var sum float64
		for j := 0; j < n; j++ {
			delta := float64(x[j]) - float64(x[j+tau])
			sum += delta * delta
		}
		d.diff[tau] = sum
	}
}

// normalize converts d(tau) into the cumulative mean normalized difference.
//
// This is the step that makes YIN scale-free: dividing each d(tau) by the
// running mean of all shorter lags keeps d'(tau) around 1 for aperiodic lags no
// matter how loud the signal is, so one absolute threshold works for a whisper
// and a belt alike. It also suppresses the dip at tau=0 that plain
// autocorrelation always has, which is what would otherwise be picked as "the"
// period.
func (d *Detector) normalize(tauMax int) {
	d.cmnd[0] = 1
	var running float64
	for tau := 1; tau <= tauMax; tau++ {
		running += d.diff[tau]
		if running == 0 {
			d.cmnd[tau] = 1
			continue
		}
		d.cmnd[tau] = d.diff[tau] * float64(tau) / running
	}
}

// absoluteThreshold picks the first lag whose normalized difference dips below
// the threshold, descending to that dip's local minimum.
//
// Taking the *first* qualifying dip rather than the global minimum is
// deliberate and is YIN's octave defense: a signal periodic at tau is also
// periodic at 2*tau and 3*tau, often with a marginally deeper dip. Choosing the
// global minimum would therefore report the note an octave or a twelfth too
// low, which for a singing app means telling someone they are wildly flat when
// they are exactly in tune.
func (d *Detector) absoluteThreshold(tauMin, tauMax int) (int, bool) {
	thr := d.threshold()
	for tau := tauMin; tau <= tauMax; tau++ {
		if d.cmnd[tau] >= thr {
			continue
		}
		// Walk down to the bottom of this dip.
		for tau+1 <= tauMax && d.cmnd[tau+1] < d.cmnd[tau] {
			tau++
		}
		return tau, true
	}
	// Nothing crossed the threshold. Fall back to the shallowest dip in range
	// and let Clarity report how weak it was, rather than discarding a note
	// that is merely breathy.
	best, bestVal := -1, math.Inf(1)
	for tau := tauMin; tau <= tauMax; tau++ {
		if d.cmnd[tau] < bestVal {
			best, bestVal = tau, d.cmnd[tau]
		}
	}
	if best < 0 || bestVal >= 1 {
		return 0, false
	}
	return best, true
}

// refine interpolates the true minimum between samples.
//
// Lag is quantized to whole samples, and that quantization is coarse where it
// hurts most: at 44.1 kHz, A4 (440 Hz) sits near lag 100, so one sample of error
// is already about 17 cents — a fifth of a semitone, plainly audible and far too
// coarse to tell a singer they are in tune. Fitting a parabola through the
// minimum and its two neighbours recovers sub-sample precision.
func (d *Detector) refine(tau, tauMax int) float64 {
	if tau <= 0 {
		return 0
	}
	if tau <= 1 || tau >= tauMax {
		return float64(tau)
	}
	s0, s1, s2 := d.cmnd[tau-1], d.cmnd[tau], d.cmnd[tau+1]
	denom := 2 * (2*s1 - s2 - s0)
	if denom == 0 {
		return float64(tau)
	}
	shift := (s2 - s0) / denom
	// A well-formed minimum shifts by less than half a sample; anything larger
	// means the parabola did not fit and the integer lag is the better answer.
	if shift > 0.5 || shift < -0.5 {
		return float64(tau)
	}
	return float64(tau) + shift
}
