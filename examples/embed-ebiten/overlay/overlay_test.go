package overlay_test

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"

	"gophics-embed-ebiten/overlay"
)

const w, h = 260, 480

// The example has to produce frames, not merely compile.
//
// An embedding example whose only proof is `go build` teaches a seam that might
// not work, which is the failure it exists to rule out. This drives the same
// handler the game does — no window, no Ebiten — and asserts the three things a
// host actually depends on: a frame arrives, it carries a damage rect, and the
// overlay is translucent enough to composite over live content.
func TestOverlayProducesTranslucentFrames(t *testing.T) {
	m := &fakeModel{speed: 1}
	h2, err := app.NewHandler(overlay.UI{M: m}, app.Config{
		Size:        geom.Size{W: w, H: h},
		Transparent: true,
		Background:  paint.Color{R: 0.05, G: 0.06, B: 0.10, A: 0.82},
		Font:        goregular.TTF,
	})
	if err != nil {
		t.Fatal(err)
	}

	var got *image.RGBA
	var damage geom.Rect
	f := &frame{put: func(img *image.RGBA, d geom.Rect) { got, damage = img, d }}
	win := &window{}

	h2.Frame(win, f, 1.0/60)
	if got == nil {
		t.Fatal("no frame reached the host")
	}
	if damage.IsEmpty() {
		t.Error("first frame reported no damage; the host would upload nothing")
	}
	if gw, gh := got.Rect.Dx(), got.Rect.Dy(); gw != w || gh != h {
		t.Errorf("frame is %dx%d, want %dx%d", gw, gh, w, h)
	}
	if _, _, _, a := got.At(4, 4).RGBA(); a == 0xffff {
		t.Error("the overlay painted itself fully opaque; the game beneath it " +
			"would be hidden, which is the one thing an overlay must not do")
	}
}

// A transparent app replays whole frames, so every frame reports full damage —
// the invariant an overlay host relies on to know its composite is complete.
func TestEveryFrameIsFullyDamaged(t *testing.T) {
	m := &fakeModel{speed: 1}
	h2, err := app.NewHandler(overlay.UI{M: m}, app.Config{
		Size: geom.Size{W: w, H: h}, Transparent: true, Font: goregular.TTF,
	})
	if err != nil {
		t.Fatal(err)
	}
	var last geom.Rect
	f := &frame{put: func(_ *image.RGBA, d geom.Rect) { last = d }}
	win := &window{}

	h2.Frame(win, f, 1.0/60)
	m.t = 3.5 // the elapsed readout is rendered, so this changes the scene
	h2.Frame(win, f, 1.0/60)

	if full := (geom.Rect{Max: geom.Pt{X: w, Y: h}}); last != full {
		t.Errorf("damage = %v, want the whole surface %v", last, full)
	}
}

type fakeModel struct {
	t      float64
	paused bool
	speed  float32
}

func (m *fakeModel) Elapsed() float64   { return m.t }
func (m *fakeModel) Paused() bool       { return m.paused }
func (m *fakeModel) TogglePause()       { m.paused = !m.paused }
func (m *fakeModel) Speed() float32     { return m.speed }
func (m *fakeModel) SetSpeed(s float32) { m.speed = s }

type frame struct{ put func(*image.RGBA, geom.Rect) }

func (f *frame) Size() geom.Size      { return geom.Size{W: w, H: h} }
func (f *frame) Scale() float32       { return 1 }
func (f *frame) Target() shell.Target { return shell.PixelTarget{Put: f.put} }

type window struct{}

func (window) Invalidate()                    {}
func (window) SetTitle(string)                {}
func (window) Close()                         {}
func (window) ClipboardRead() (string, error) { return "", nil }
func (window) ClipboardWrite(string) error    { return nil }
func (window) OpenURL(string) error           { return nil }
func (window) DarkMode() bool                 { return true }
