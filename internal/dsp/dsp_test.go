package dsp

import (
	"math"
	"math/cmplx"
	"sync"
	"testing"
)

// dft is the textbook O(n^2) transform, used to check the fast one. Verifying
// an FFT against its own output proves nothing; verifying it against the
// definition proves it computes a Fourier transform.
func dft(x []complex128) []complex128 {
	n := len(x)
	out := make([]complex128, n)
	for k := 0; k < n; k++ {
		var sum complex128
		for t := 0; t < n; t++ {
			ang := -2 * math.Pi * float64(k) * float64(t) / float64(n)
			sum += x[t] * cmplx.Exp(complex(0, ang))
		}
		out[k] = sum
	}
	return out
}

func TestFFTMatchesDFT(t *testing.T) {
	for _, n := range []int{2, 4, 8, 64, 256} {
		x := make([]complex128, n)
		for i := range x {
			// An arbitrary but reproducible signal with both parts non-zero.
			x[i] = complex(math.Sin(float64(i)*0.7)+0.3*float64(i%5), math.Cos(float64(i)*0.31))
		}
		want := dft(x)
		got := append([]complex128(nil), x...)
		fft(got)
		for k := range want {
			if cmplx.Abs(got[k]-want[k]) > 1e-9*float64(n) {
				t.Fatalf("n=%d bin %d: got %v, want %v", n, k, got[k], want[k])
			}
		}
	}
}

// TestFFTFindsATone is the property that actually matters downstream: a pure
// tone must produce a peak in the bin its frequency belongs to.
func TestFFTFindsATone(t *testing.T) {
	const n, rate = 1024, 48000
	for _, freq := range []float64{375, 750, 3000} {
		x := make([]complex128, n)
		for i := range x {
			x[i] = complex(math.Sin(2*math.Pi*freq*float64(i)/rate), 0)
		}
		fft(x)

		best, bestMag := 0, 0.0
		for k := 1; k < n/2; k++ {
			if m := cmplx.Abs(x[k]); m > bestMag {
				best, bestMag = k, m
			}
		}
		gotFreq := float64(best) * rate / n
		if math.Abs(gotFreq-freq) > rate/n {
			t.Errorf("%.0f Hz peaked at bin %d (%.0f Hz)", freq, best, gotFreq)
		}
	}
}

func TestIsPow2(t *testing.T) {
	for _, n := range []int{1, 2, 4, 1024, 2048} {
		if !isPow2(n) {
			t.Errorf("isPow2(%d) = false", n)
		}
	}
	for _, n := range []int{0, -4, 3, 1000, 2047} {
		if isPow2(n) {
			t.Errorf("isPow2(%d) = true", n)
		}
	}
}

func TestNewRoundsWindowToPowerOfTwo(t *testing.T) {
	if got := New(48000, 1000).WindowSize(); got != 1024 {
		t.Errorf("window 1000 became %d, want 1024", got)
	}
	if got := New(48000, 2048).WindowSize(); got != 2048 {
		t.Errorf("window 2048 became %d", got)
	}
	if got := New(48000, 0).WindowSize(); got != DefaultWindow {
		t.Errorf("window 0 became %d, want %d", got, DefaultWindow)
	}
}

// TestSamplesAreChronological is the ordering guarantee pitch detection depends
// on: a ring buffer that hands back a rotated window destroys periodicity and
// every note comes out wrong.
func TestSamplesAreChronological(t *testing.T) {
	a := New(48000, 256)
	// Write a ramp longer than the ring so it wraps, then check the tail.
	const total = 600
	in := make([]float32, total)
	for i := range in {
		in[i] = float32(i)
	}
	// Feed in small blocks, as a real driver would.
	for i := 0; i < total; i += 37 {
		end := i + 37
		if end > total {
			end = total
		}
		a.Write(in[i:end])
	}

	out := make([]float32, 256)
	n := a.Samples(out)
	if n != 256 {
		t.Fatalf("got %d samples, want 256", n)
	}
	for i := 1; i < n; i++ {
		if out[i] != out[i-1]+1 {
			t.Fatalf("samples not chronological at %d: %v then %v", i, out[i-1], out[i])
		}
	}
	if want := float32(total - 1); out[n-1] != want {
		t.Errorf("newest sample is %v, want %v", out[n-1], want)
	}
}

func TestSamplesBeforeWrap(t *testing.T) {
	a := New(48000, 256)
	a.Write([]float32{1, 2, 3, 4})
	out := make([]float32, 256)
	if n := a.Samples(out); n != 4 {
		t.Fatalf("got %d samples, want 4", n)
	}
	for i, want := range []float32{1, 2, 3, 4} {
		if out[i] != want {
			t.Errorf("sample %d = %v, want %v", i, out[i], want)
		}
	}
}

// TestShortDstTakesTheNewest: a caller asking for fewer samples than the window
// wants the most recent ones, not the oldest.
func TestShortDstTakesTheNewest(t *testing.T) {
	a := New(48000, 256)
	in := make([]float32, 256)
	for i := range in {
		in[i] = float32(i)
	}
	a.Write(in)

	out := make([]float32, 8)
	if n := a.Samples(out); n != 8 {
		t.Fatalf("got %d, want 8", n)
	}
	if out[7] != 255 {
		t.Errorf("newest sample is %v, want 255", out[7])
	}
	if out[0] != 248 {
		t.Errorf("oldest of the tail is %v, want 248", out[0])
	}
}

// TestOversizedWriteKeepsTheTail: a driver block bigger than the whole window
// must leave the newest audio, not the oldest.
func TestOversizedWriteKeepsTheTail(t *testing.T) {
	a := New(48000, 256)
	in := make([]float32, 1000)
	for i := range in {
		in[i] = float32(i)
	}
	a.Write(in)

	out := make([]float32, 256)
	a.Samples(out)
	if out[255] != 999 {
		t.Errorf("newest sample is %v, want 999", out[255])
	}
	if out[0] != 744 {
		t.Errorf("oldest sample is %v, want 744", out[0])
	}
}

func TestWriteInt16Scales(t *testing.T) {
	a := New(48000, 256)
	a.WriteInt16([]int16{0, 16384, -16384, 32767})
	out := make([]float32, 4)
	a.Samples(out)
	want := []float32{0, 0.5, -0.5, 32767.0 / 32768}
	for i := range want {
		if math.Abs(float64(out[i]-want[i])) > 1e-6 {
			t.Errorf("sample %d = %v, want %v", i, out[i], want[i])
		}
	}
}

func TestWriteInt16Wraps(t *testing.T) {
	a := New(48000, 256)
	in := make([]int16, 600)
	for i := range in {
		in[i] = int16(i % 100)
	}
	a.WriteInt16(in)
	out := make([]float32, 256)
	if n := a.Samples(out); n != 256 {
		t.Fatalf("got %d, want 256", n)
	}
	if want := float32(599%100) / 32768; math.Abs(float64(out[255]-want)) > 1e-6 {
		t.Errorf("newest = %v, want %v", out[255], want)
	}
}

func TestLevel(t *testing.T) {
	a := New(48000, 256)
	if got := a.Level(); got != 0 {
		t.Errorf("silence level = %v, want 0", got)
	}
	in := make([]float32, 256)
	for i := range in {
		in[i] = 0.5 * float32(math.Sin(float64(i)))
	}
	in[10] = -0.8
	a.Write(in)
	if got := a.Level(); math.Abs(float64(got-0.8)) > 1e-6 {
		t.Errorf("level = %v, want 0.8", got)
	}
}

// TestBandsFindATone checks the spectrum end to end: a tone should light a band
// in the right part of a log-spaced display, and silence should light none.
func TestBandsFindATone(t *testing.T) {
	const rate, n = 48000, 2048
	a := New(rate, n)

	dst := make([]float32, 32)
	a.Bands(dst)
	for i, v := range dst {
		if v > 0.02 {
			t.Errorf("silence lit band %d at %.3f", i, v)
		}
	}

	// A loud low tone and a loud high tone must peak in different halves.
	peakFor := func(freq float64) int {
		a.Reset()
		in := make([]float32, n)
		for i := range in {
			in[i] = float32(0.9 * math.Sin(2*math.Pi*freq*float64(i)/rate))
		}
		a.Write(in)
		a.Bands(dst)
		best, bestV := 0, float32(0)
		for i, v := range dst {
			if v > bestV {
				best, bestV = i, v
			}
		}
		if bestV < 0.2 {
			t.Errorf("%.0f Hz produced no visible band (peak %.3f)", freq, bestV)
		}
		return best
	}

	low, high := peakFor(200), peakFor(6000)
	if low >= high {
		t.Errorf("200 Hz peaked at band %d and 6000 Hz at %d — bands are not ascending", low, high)
	}
}

func TestFoldBandsEdges(t *testing.T) {
	if got := FoldBands([]float32{1, 1, 1}, nil); got != 0 {
		t.Errorf("FoldBands into nil returned %d", got)
	}
	if got := FoldBands(nil, make([]float32, 4)); got != 0 {
		t.Errorf("FoldBands from nil returned %d", got)
	}
	// Every band gets a value, and none exceeds 1.
	bins := make([]float32, 512)
	for i := range bins {
		bins[i] = 2 // deliberately out of range
	}
	dst := make([]float32, 24)
	if got := FoldBands(bins, dst); got != 24 {
		t.Errorf("FoldBands wrote %d bands, want 24", got)
	}
	for i, v := range dst {
		if v != 1 {
			t.Errorf("band %d = %v, want clamped to 1", i, v)
		}
	}
}

func TestReset(t *testing.T) {
	a := New(48000, 256)
	in := make([]float32, 256)
	for i := range in {
		in[i] = 0.9
	}
	a.Write(in)
	a.Reset()
	if got := a.Level(); got != 0 {
		t.Errorf("level after Reset = %v, want 0", got)
	}
	out := make([]float32, 256)
	if n := a.Samples(out); n != 0 {
		t.Errorf("Samples after Reset returned %d, want 0", n)
	}
}

// TestConcurrentWriteAndRead is the shape the real thing runs in: a capture
// thread writing while the UI reads. Run with -race.
func TestConcurrentWriteAndRead(t *testing.T) {
	a := New(48000, 2048)
	var wg sync.WaitGroup
	stop := make(chan struct{})

	wg.Add(1)
	go func() { // the capture thread
		defer wg.Done()
		block := make([]float32, 256)
		for i := 0; ; i++ {
			select {
			case <-stop:
				return
			default:
			}
			for j := range block {
				block[j] = float32(math.Sin(float64(i*len(block)+j) * 0.01))
			}
			a.Write(block)
		}
	}()

	out := make([]float32, 2048)
	bands := make([]float32, 32)
	for i := 0; i < 200; i++ { // the UI goroutine
		a.Samples(out)
		a.Level()
		a.Bands(bands)
	}
	close(stop)
	wg.Wait()
}

func BenchmarkBands(b *testing.B) {
	a := New(48000, 2048)
	in := make([]float32, 2048)
	for i := range in {
		in[i] = float32(math.Sin(float64(i) * 0.05))
	}
	a.Write(in)
	dst := make([]float32, 48)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.Bands(dst)
	}
}
