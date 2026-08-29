// Package dsp is the shared Go half of live microphone capture: level,
// spectrum, and a ring buffer of recent samples.
//
// It holds no device code, which is why it is no longer called mic. That name
// put it beside internal/audio — the package that does open devices — and made
// two very different things look like siblings: one talks to CoreAudio, ALSA
// and WASAPI, the other is arithmetic.
//
// Every shell that captures audio natively — Android, iOS, macOS, Linux,
// Windows — receives raw PCM from a platform callback on some background
// thread and has to answer the same three questions the shell.Monitor contract
// asks: how loud is it, what does its spectrum look like, and what were the
// last N samples. Only the capture plumbing differs per platform, so that
// analysis lives here once rather than five times.
//
// The web shell is the exception: a browser's AnalyserNode already does this
// work natively, so shell/web keeps its own path and only shares the band
// folding, so the bars look identical everywhere.
package dsp

import "math"

// fft computes the in-place radix-2 decimation-in-time FFT of x, whose length
// must be a power of two.
//
// A real-input FFT could do this in half the time by packing the signal into a
// complex sequence of half the length, but a microphone window is 2048 samples
// analysed at most once per frame; the straightforward transform costs tens of
// microseconds and is much easier to be sure is correct.
func fft(x []complex128) {
	n := len(x)
	if n <= 1 {
		return
	}

	// Bit-reversal permutation: reorder the input so the butterflies below can
	// run bottom-up over contiguous pairs.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j |= bit
		if i < j {
			x[i], x[j] = x[j], x[i]
		}
	}

	for length := 2; length <= n; length <<= 1 {
		// The principal root for this stage. Negative angle = forward transform.
		ang := -2 * math.Pi / float64(length)
		wl := complex(math.Cos(ang), math.Sin(ang))
		for i := 0; i < n; i += length {
			w := complex(1, 0)
			for j := 0; j < length/2; j++ {
				u := x[i+j]
				v := x[i+j+length/2] * w
				x[i+j] = u + v
				x[i+j+length/2] = u - v
				w *= wl
			}
		}
	}
}

// hann returns an n-point Hann window.
//
// Windowing is not optional here. An unwindowed FFT treats its input as exactly
// periodic, and a sung note almost never completes a whole number of cycles in
// the window; the discontinuity at the wrap smears energy across every bin
// (spectral leakage), which on a log-folded display shows up as a noise floor
// that rises and falls with the singer's volume.
func hann(n int) []float64 {
	w := make([]float64, n)
	if n == 1 {
		w[0] = 1
		return w
	}
	for i := range w {
		w[i] = 0.5 * (1 - math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return w
}

// isPow2 reports whether n is a positive power of two.
func isPow2(n int) bool { return n > 0 && n&(n-1) == 0 }

// FoldBands groups linear FFT magnitudes into logarithmically spaced bands,
// writing len(dst) values in 0..1 and returning how many it wrote.
//
// It is exported so the web shell can fold its AnalyserNode's bins through the
// same code: the shell.Monitor contract promises a caller that asking for N
// bands means the same thing on every platform, and two independent folding
// implementations would quietly drift apart.
//
// Pitch is logarithmic and FFT bins are not. Folded linearly, the bottom two
// octaves — where a voice lives — all land in the first couple of bars while
// the rest of the display shows hiss.
func FoldBands(bins []float32, dst []float32) int {
	n := len(dst)
	if n == 0 {
		return 0
	}
	// Skip bin 0: it is DC, and a microphone's DC offset would peg the first
	// band at full scale in a silent room.
	const lo = 1
	hi := len(bins)
	if hi <= lo {
		return 0
	}
	ratio := math.Log(float64(hi) / float64(lo))
	edge := func(i int) int {
		e := min(int(float64(lo)*math.Exp(ratio*float64(i)/float64(n))), hi)
		return e
	}
	for i := range n {
		a, b := edge(i), edge(i+1)
		if b <= a {
			b = a + 1 // the lowest bands are narrower than one bin
		}
		if a >= hi {
			dst[i] = 0
			continue
		}
		if b > hi {
			b = hi
		}
		// Peak, not mean, across the group: a narrow tone inside a wide
		// high-frequency band would be averaged into invisibility.
		var peak float32
		for _, v := range bins[a:b] {
			if v > peak {
				peak = v
			}
		}
		if peak > 1 {
			peak = 1
		}
		dst[i] = peak
	}
	return n
}
