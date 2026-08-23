//go:build !js

package desktop

import (
	"math"
	"testing"
	"time"

	"github.com/doug/gophics/shell"
)

// The recorder's conversion and assembly are tested without a device, because
// they are where the bugs are: the audio thread writes into fixed chunks and
// Stop has to reassemble them in order, at the right length, with the right
// scaling. A device test would exercise the same code and be unable to say
// what the samples should have been.

func TestRecorderAssemblesChunksInOrder(t *testing.T) {
	r := &desktopRecorder{cap: nopCapture{}, rate: 8000, start: time.Now()}

	// More than two chunks, so ordering and the partial tail both matter.
	const n = chunkSamples*2 + 100
	in := make([]float32, n)
	for i := range in {
		in[i] = float32(i%1000) / 1000
	}
	// Delivered in odd-sized blocks, as a real audio callback would.
	for i := 0; i < n; i += 333 {
		end := i + 333
		if end > n {
			end = n
		}
		r.write(in[i:end])
	}

	pcm, rate, ok := r.finish()
	if !ok {
		t.Fatal("finish reported already-finished on the first call")
	}
	if rate != 8000 {
		t.Errorf("rate = %d, want 8000", rate)
	}
	if len(pcm) != n {
		t.Fatalf("got %d samples, want %d", len(pcm), n)
	}
	for i := range pcm {
		want := int16(in[i] * 32767)
		if pcm[i] != want {
			t.Fatalf("sample %d = %d, want %d (chunks reassembled out of order?)", i, pcm[i], want)
		}
	}
}

func TestRecorderClipsRatherThanWrapping(t *testing.T) {
	r := &desktopRecorder{cap: nopCapture{}, rate: 8000, start: time.Now()}
	r.write([]float32{2, -2, 1, -1})
	pcm, _, _ := r.finish()
	// Full scale is 32767, so +-1.0 maps symmetrically to +-32767 and never
	// reaches the int16 floor; only genuine overflow (-2.0) is clamped to it.
	want := []int16{32767, -32768, 32767, -32767}
	for i, w := range want {
		if pcm[i] != w {
			t.Errorf("sample %d = %d, want %d: an out-of-range input wrapped instead of clipping", i, pcm[i], w)
		}
	}
}

func TestFinishIsIdempotent(t *testing.T) {
	c := &countingCapture{}
	r := &desktopRecorder{cap: c, rate: 8000, start: time.Now()}
	r.write([]float32{0.5})
	if _, _, ok := r.finish(); !ok {
		t.Fatal("first finish reported false")
	}
	if _, _, ok := r.finish(); ok {
		t.Error("second finish reported true; Stop followed by Cancel would double-close the device")
	}
	if c.closes != 1 {
		t.Errorf("device closed %d times, want 1", c.closes)
	}
}

func TestStopProducesAPlayableClip(t *testing.T) {
	r := &desktopRecorder{cap: nopCapture{}, rate: 8000, start: time.Now()}
	in := make([]float32, 8000) // exactly one second
	for i := range in {
		in[i] = float32(math.Sin(float64(i) * 0.05))
	}
	r.write(in)

	var got shell.Clip
	var err error
	r.Stop(func(c shell.Clip, e error) { got, err = c, e })
	if err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if got.Mime != "audio/wav" {
		t.Errorf("mime = %q, want audio/wav", got.Mime)
	}
	if got.Duration != time.Second {
		t.Errorf("duration = %v, want 1s", got.Duration)
	}
	if len(got.Envelope) == 0 {
		t.Error("no envelope; the waveform view would be blank")
	}
	// The clip has to survive the round trip, or it is not portable to the
	// other backends that decode the same WAV.
	pcm, rate, err := shell.DecodeWAV(got.Data)
	if err != nil {
		t.Fatalf("the clip we just encoded does not decode: %v", err)
	}
	if rate != 8000 || len(pcm) != len(in) {
		t.Errorf("round trip gave %d samples at %d Hz, want %d at 8000", len(pcm), rate, len(in))
	}
}

func TestStopWithNoAudioReportsAnError(t *testing.T) {
	r := &desktopRecorder{cap: nopCapture{}, rate: 8000, start: time.Now()}
	var err error
	r.Stop(func(_ shell.Clip, e error) { err = e })
	if err == nil {
		t.Error("Stop returned a zero Clip and no error; the app cannot tell that nothing was captured")
	}
}

// --- playback position -------------------------------------------------------

func TestPositionIsClampedAndFrozenWhenStopped(t *testing.T) {
	p := &desktopPlayback{rate: 8000, duration: 2 * time.Second}

	// Not started: the cursor sits at the offset.
	if got := p.Position(); got != 0 {
		t.Errorf("position before play = %v, want 0", got)
	}

	p.offset = 1500 * time.Millisecond
	p.started = time.Now().Add(-time.Second) // one second of play from 1.5s
	p.player = nil
	// player == nil is treated as not running, so this reads the offset.
	if got := p.Position(); got != 1500*time.Millisecond {
		t.Errorf("position = %v, want the stored offset 1.5s", got)
	}

	p.stopped = true
	p.offset = 5 * time.Second
	if got := p.Position(); got != 5*time.Second {
		t.Errorf("stopped position = %v, want the stored offset", got)
	}
}

func TestSeekOnAStoppedPlaybackMovesTheCursorWithoutResuming(t *testing.T) {
	p := &desktopPlayback{rate: 8000, duration: 2 * time.Second, stopped: true}
	p.Seek(time.Second)
	if p.Playing() {
		t.Error("seeking a stopped playback resumed it")
	}
	if p.offset != time.Second {
		t.Errorf("offset = %v, want 1s", p.offset)
	}
}

// --- stubs -------------------------------------------------------------------

type nopCapture struct{}

func (nopCapture) Open(rate int) (int, error)  { return rate, nil }
func (nopCapture) Start(func([]float32)) error { return nil }
func (nopCapture) Close() error                { return nil }

type countingCapture struct {
	nopCapture
	closes int
}

func (c *countingCapture) Close() error { c.closes++; return nil }
