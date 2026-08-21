package main

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
)

// --- The effect --------------------------------------------------------------

// gradient is a frame whose every pixel is a known function of its position, so
// a displacement can be read straight back out of the output.
func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: uint8((x + y) / 2), A: 255})
		}
	}
	return img
}

// TestWarpSilenceIsIdentity is the anchor: with nothing to hear, the mirror
// must pass the camera through untouched (bar the flip). An effect that drifts
// when the room is silent reads as a bug in the camera, not as art.
func TestWarpSilenceIsIdentity(t *testing.T) {
	src := gradient(64, 48)
	dst := image.NewRGBA(src.Bounds())
	Warp(dst, src, Params{Amount: 1, T: 12.5, Bands: make([]float32, 16)})

	for y := 0; y < 48; y++ {
		for x := 0; x < 64; x++ {
			if got, want := dst.RGBAAt(x, y), src.RGBAAt(x, y); got != want {
				t.Fatalf("silence altered (%d,%d): %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestWarpMirrorFlips checks the flip is a true horizontal mirror.
func TestWarpMirrorFlips(t *testing.T) {
	src := gradient(64, 48)
	dst := image.NewRGBA(src.Bounds())
	Warp(dst, src, Params{Amount: 1, Mirror: true, Bands: make([]float32, 8)})
	for y := 0; y < 48; y += 7 {
		for x := 0; x < 64; x++ {
			if got, want := dst.RGBAAt(x, y), src.RGBAAt(63-x, y); got != want {
				t.Fatalf("mirror wrong at (%d,%d): %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestWarpRespondsToSound checks both modulations actually move pixels, and
// that they move them in the directions the effect claims: a band lifts its own
// columns, and level is what opens the colour split.
func TestWarpRespondsToSound(t *testing.T) {
	src := gradient(64, 48)
	quiet := image.NewRGBA(src.Bounds())
	Warp(quiet, src, Params{Amount: 1, Bands: make([]float32, 8)})

	// One band, hard right, so only the right-hand columns should move.
	bands := make([]float32, 8)
	bands[7] = 1
	loud := image.NewRGBA(src.Bounds())
	Warp(loud, src, Params{Amount: 1, Bands: bands})

	if loud.RGBAAt(4, 24) != quiet.RGBAAt(4, 24) {
		t.Error("a band on the right moved a column on the left")
	}
	if loud.RGBAAt(60, 24) == quiet.RGBAAt(60, 24) {
		t.Error("a band at full scale didn't lift its own columns")
	}

	// Level alone (no bands) must still split the colour channels.
	split := image.NewRGBA(src.Bounds())
	Warp(split, src, Params{Amount: 1, Level: 1, Bands: make([]float32, 8)})
	var moved int
	for x := 0; x < 64; x++ {
		if split.RGBAAt(x, 24).R != quiet.RGBAAt(x, 24).R {
			moved++
		}
	}
	if moved == 0 {
		t.Error("level didn't shift the red channel")
	}
}

// TestWarpStaysInBounds pushes every parameter past its range and checks the
// sampler clamps instead of reading outside the frame — the failure mode here
// is a panic in the middle of a live preview.
func TestWarpStaysInBounds(t *testing.T) {
	src := gradient(40, 30)
	dst := image.NewRGBA(src.Bounds())
	bands := make([]float32, 4)
	for i := range bands {
		bands[i] = 9 // deliberately out of the documented 0..1
	}
	for _, p := range []Params{
		{Amount: 5, Level: 5, Bands: bands, T: 1e6},
		{Amount: -3, Level: -3, Bands: bands, Mirror: true},
		{Amount: 1, Level: 1, Bands: nil},
		{Amount: 1, Level: 1, Bands: []float32{}},
	} {
		Warp(dst, src, p) // must not panic
		for y := 0; y < 30; y++ {
			for x := 0; x < 40; x++ {
				if a := dst.RGBAAt(x, y).A; a != 255 {
					t.Fatalf("alpha %d at (%d,%d) for %+v", a, x, y, p)
				}
			}
		}
	}
}

// TestWarpIgnoresMismatchedBuffers checks the size guard, which exists because
// a camera can renegotiate its frame size under a running preview.
func TestWarpIgnoresMismatchedBuffers(t *testing.T) {
	src := gradient(32, 24)
	dst := image.NewRGBA(image.Rect(0, 0, 16, 12))
	Warp(dst, src, Params{Amount: 1, Level: 1}) // must not panic
	for _, v := range dst.Pix {
		if v != 0 {
			t.Fatal("a mismatched destination was written to")
		}
	}
}

func BenchmarkWarp(b *testing.B) {
	src := gradient(640, 480)
	dst := image.NewRGBA(src.Bounds())
	bands := make([]float32, numBands)
	for i := range bands {
		bands[i] = float32(i%7) / 7
	}
	p := Params{Amount: 0.8, Level: 0.6, Bands: bands, Mirror: true}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.T = float32(i) / 60
		Warp(dst, src, p)
	}
}

// --- The app -----------------------------------------------------------------

// fakeSource stands in for the camera and microphone, so the whole app — the
// polling, the smoothing, the double buffering, the paint — is exercisable with
// no hardware, no browser, and no permission prompt.
type fakeSource struct {
	frames  int
	level   float32
	silent  bool // return no frame, as a real preview does before it warms up
	img     [2]*image.RGBA
	current int
}

func (f *fakeSource) Frame() *image.RGBA {
	if f.silent {
		return nil
	}
	f.frames++
	f.current = (f.current + 1) % len(f.img)
	if f.img[f.current] == nil {
		f.img[f.current] = gradient(80, 60)
	}
	return f.img[f.current]
}

func (f *fakeSource) Level() float32 { return f.level }

func (f *fakeSource) Bands(dst []float32) int {
	for i := range dst {
		dst[i] = f.level * float32(math.Abs(math.Sin(float64(i))))
	}
	return len(dst)
}

func newApp(t *testing.T, src Source) (*app.Headless, *mirror) {
	t.Helper()
	var st *mirror
	testSource, stateHook = src, func(m *mirror) { st = m }
	defer func() { testSource, stateHook = nil, nil }()

	h, err := app.NewHeadless(App{}, app.Config{
		Size: geom.Size{W: 960, H: 720}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: 960, H: 720})
	h.Render()
	if st == nil {
		t.Fatal("state never mounted")
	}
	return h, st
}

// TestPipelineRuns drives the whole app off the fake source and checks a warped
// frame actually reaches the canvas.
func TestPipelineRuns(t *testing.T) {
	src := &fakeSource{level: 0.8}
	h, m := newApp(t, src)
	for i := 0; i < 30; i++ {
		h.Step(1.0 / 60)
	}
	if src.frames == 0 {
		t.Fatal("no frames were pulled")
	}
	if m.show == nil {
		t.Fatal("nothing was warped for the canvas")
	}
	if m.lastWarp <= 0 {
		t.Error("the warp was never timed")
	}
	if img := h.Render(); img.Bounds().Dx() != 960 {
		t.Fatalf("rendered %v", img.Bounds())
	}
}

// TestOutputBuffersRotate guards the identity rule the scene relies on: two
// consecutive frames must be different image values, or a canvas handed the
// same pointer with new pixels would never repaint.
func TestOutputBuffersRotate(t *testing.T) {
	h, m := newApp(t, &fakeSource{level: 0.5})
	h.Step(1.0 / 60)
	first := m.show
	h.Step(1.0 / 60)
	if first == nil || m.show == nil {
		t.Fatal("no output produced")
	}
	if first == m.show {
		t.Error("consecutive frames are the same image value; the scene would not repaint")
	}
}

// TestSurvivesFramelessStart checks the first stretch of a real preview, where
// the stream is open but no frame has decoded yet.
func TestSurvivesFramelessStart(t *testing.T) {
	src := &fakeSource{level: 0.4, silent: true}
	h, m := newApp(t, src)
	for i := 0; i < 20; i++ {
		h.Step(1.0 / 60)
	}
	if m.show != nil {
		t.Error("something was drawn before any frame arrived")
	}
	src.silent = false
	for i := 0; i < 5; i++ {
		h.Step(1.0 / 60)
	}
	if m.show == nil {
		t.Error("nothing was drawn once frames started")
	}
}

// TestLevelAttacksFastReleasesSlow pins the envelope: the image should jump on
// a syllable and settle afterwards, not chatter at the frame rate.
func TestLevelAttacksFastReleasesSlow(t *testing.T) {
	src := &fakeSource{}
	h, m := newApp(t, src)

	src.level = 1
	for i := 0; i < 5; i++ {
		h.Step(1.0 / 60)
	}
	rise := m.smooth
	if rise < 0.5 {
		t.Errorf("after 5 loud frames the envelope is only %.2f; the attack is too slow", rise)
	}
	src.level = 0
	for i := 0; i < 5; i++ {
		h.Step(1.0 / 60)
	}
	if m.smooth > rise*0.75 {
		t.Errorf("envelope fell from %.2f to %.2f in 5 frames; the release is too fast", rise, m.smooth)
	}
	if m.smooth >= rise {
		t.Error("the envelope didn't fall at all")
	}
}

// TestDegradesWithoutCapabilities is the capability-layer contract. With no
// camera or microphone the app must stay up, say plainly that what you are
// seeing is not a camera, and still show what the effect does — not present a
// start button for a stream it cannot open.
func TestDegradesWithoutCapabilities(t *testing.T) {
	h, m := newApp(t, nil) // headless publishes no capabilities
	if m.live() {
		t.Fatal("the app claims live capture without any capability")
	}
	if !m.synthetic {
		t.Fatal("no stand-in source was installed")
	}
	for i := 0; i < 20; i++ {
		h.Step(1.0 / 60)
	}
	if m.show == nil {
		t.Error("the stand-in produced nothing to draw")
	}
	h.Render()
	if !hasText(h, "Synthetic preview") {
		t.Error("the synthetic source wasn't disclosed on screen")
	}
	if hasText(h, "Start the mirror") {
		t.Error("a start button was offered with nothing to start")
	}
}

// TestSyntheticIsOnlyAFallback checks the stand-in never displaces a real
// camera: an installed source (as in every other test here) is not labelled
// synthetic, and neither would a live platform's be.
func TestSyntheticIsOnlyAFallback(t *testing.T) {
	_, m := newApp(t, &fakeSource{})
	if m.synthetic {
		t.Error("an explicit source was mislabelled as the stand-in")
	}
}

func hasText(h *app.Headless, sub string) bool {
	for _, n := range h.Semantics() {
		if strings.Contains(n.Label, sub) {
			return true
		}
	}
	return false
}

// TestStopReleasesEverything checks the camera and microphone are handed back.
func TestStopReleasesEverything(t *testing.T) {
	_, m := newApp(t, &fakeSource{})
	m.stop()
	if m.frames != nil || m.mon != nil {
		t.Error("handles survived stop")
	}
}
