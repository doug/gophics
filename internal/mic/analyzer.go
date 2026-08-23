package mic

import (
	"math"
	"sync"
)

// DefaultWindow is the analysis window every native shell captures into.
//
// It matches the web shell's PCM window so an app sees the same resolution
// everywhere. At 44.1-48 kHz, 2048 samples is ~43-46 ms: long enough to hold
// two full periods of any pitch down to ~45 Hz — which autocorrelation needs to
// find a period at all — and short enough that a moving voice is not blurred
// into its own average.
const DefaultWindow = 2048

// Analyzer buffers captured PCM and answers the shell.Monitor queries.
//
// Writes come from the platform's capture thread and reads from the UI
// goroutine, so every method is mutex-guarded. The lock is held only for the
// copy in and out, never across the FFT, so a slow frame cannot stall capture
// and cause the driver to drop audio.
type Analyzer struct {
	mu     sync.Mutex
	ring   []float32
	w      int  // next write index
	filled bool // the ring has wrapped at least once
	rate   int

	// Scratch reused across Bands calls, guarded by its own lock so a Bands
	// call and a capture write do not contend.
	specMu  sync.Mutex
	buf     []complex128
	win     []float64
	mags    []float32
	scratch []float32
}

// New returns an Analyzer buffering window samples at the given rate. window is
// rounded up to a power of two (the FFT requires one) and floored at 256.
func New(rate, window int) *Analyzer {
	if window < 256 {
		window = DefaultWindow
	}
	if !isPow2(window) {
		n := 256
		for n < window {
			n <<= 1
		}
		window = n
	}
	return &Analyzer{
		ring:    make([]float32, window),
		rate:    rate,
		buf:     make([]complex128, window),
		win:     hann(window),
		mags:    make([]float32, window/2),
		scratch: make([]float32, window),
	}
}

// SampleRate is the capture rate in Hz.
func (a *Analyzer) SampleRate() int { return a.rate }

// WindowSize is the most samples Samples can return.
func (a *Analyzer) WindowSize() int { return len(a.ring) }

// Write appends captured mono PCM in [-1,1]. Safe to call from the platform's
// capture thread.
//
// A block longer than the window overwrites it entirely, so only its tail
// survives — which is correct: the ring holds the most recent audio, and if a
// single callback delivered more than a window's worth, the older part of it is
// already stale.
func (a *Analyzer) Write(pcm []float32) {
	if len(pcm) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	n := len(a.ring)
	if len(pcm) >= n {
		copy(a.ring, pcm[len(pcm)-n:])
		a.w, a.filled = 0, true
		return
	}
	for _, v := range pcm {
		a.ring[a.w] = v
		a.w++
		if a.w == n {
			a.w, a.filled = 0, true
		}
	}
}

// WriteInt16 is Write for signed 16-bit PCM, the format Android's AudioRecord
// and most native capture APIs deliver.
func (a *Analyzer) WriteInt16(pcm []int16) {
	if len(pcm) == 0 {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()

	n := len(a.ring)
	src := pcm
	if len(src) >= n {
		src = src[len(src)-n:]
		a.w, a.filled = 0, true
		for i, v := range src {
			a.ring[i] = float32(v) / 32768
		}
		return
	}
	for _, v := range src {
		a.ring[a.w] = float32(v) / 32768
		a.w++
		if a.w == n {
			a.w, a.filled = 0, true
		}
	}
}

// read copies the ring into dst in chronological order, oldest first, and
// reports how many samples are real.
func (a *Analyzer) read(dst []float32) int {
	a.mu.Lock()
	defer a.mu.Unlock()

	n := len(a.ring)
	if !a.filled {
		// Before the first wrap only [0, w) holds audio.
		got := a.w
		if got > len(dst) {
			// Keep the newest part, matching the wrapped case.
			copy(dst, a.ring[got-len(dst):got])
			return len(dst)
		}
		copy(dst, a.ring[:got])
		return got
	}
	want := len(dst)
	if want > n {
		want = n
	}
	// The newest `want` samples end just before a.w, wrapping backwards.
	start := (a.w - want + n) % n
	if start+want <= n {
		copy(dst, a.ring[start:start+want])
	} else {
		k := n - start
		copy(dst, a.ring[start:])
		copy(dst[k:], a.ring[:want-k])
	}
	return want
}

// Samples fills dst with the most recent PCM, oldest sample first.
func (a *Analyzer) Samples(dst []float32) int {
	if len(dst) == 0 {
		return 0
	}
	return a.read(dst)
}

// Level is the peak amplitude of the most recent window, 0..1 — the same
// quantity the web shell reports, so a meter reads alike everywhere.
func (a *Analyzer) Level() float32 {
	a.mu.Lock()
	defer a.mu.Unlock()

	var peak float32
	for _, v := range a.ring {
		if v < 0 {
			v = -v
		}
		if v > peak {
			peak = v
		}
	}
	if peak > 1 {
		peak = 1
	}
	return peak
}

// Bands fills dst with the log-folded spectrum, lowest frequency first.
func (a *Analyzer) Bands(dst []float32) int {
	if len(dst) == 0 {
		return 0
	}
	a.specMu.Lock()
	defer a.specMu.Unlock()

	n := len(a.ring)
	// Reused rather than allocated: Bands runs once a frame, and an 8 KB
	// allocation per frame is pure garbage-collector pressure for no benefit.
	scratch := a.scratch
	got := a.read(scratch)
	if got == 0 {
		for i := range dst {
			dst[i] = 0
		}
		return len(dst)
	}

	for i := 0; i < n; i++ {
		var v float64
		if i < got {
			v = float64(scratch[i])
		}
		a.buf[i] = complex(v*a.win[i], 0)
	}
	fft(a.buf)

	// Only the first half is meaningful; above Nyquist the spectrum mirrors.
	//
	// The scale factor turns raw bin magnitudes into the 0..1 the contract
	// promises. A Hann window sums to about n/2, so that is the magnitude a
	// full-scale sinusoid at bin centre produces; dividing by it puts a loud
	// pure tone near 1 without clipping every other tone to it.
	scale := 2.0 / float64(n)
	for i := range a.mags {
		m := math.Hypot(real(a.buf[i]), imag(a.buf[i])) * scale
		if m > 1 {
			m = 1
		}
		a.mags[i] = float32(m)
	}
	return FoldBands(a.mags, dst)
}

// Reset discards buffered audio, so a restarted monitor does not report the
// previous session's tail.
func (a *Analyzer) Reset() {
	a.mu.Lock()
	defer a.mu.Unlock()
	for i := range a.ring {
		a.ring[i] = 0
	}
	a.w, a.filled = 0, false
}
