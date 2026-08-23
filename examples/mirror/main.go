// Command mirror is a voice-reactive mirror: your camera, warped every frame by
// what the microphone hears. Columns of the image rise with the energy in their
// part of the spectrum, rows slide on a wave that grows with loudness, and the
// colour channels split apart as you get louder — so the picture sings.
//
// It is the driver example for the live-capture capabilities, shell.CameraPreview
// and shell.Microphone: streaming capture, as opposed to the one-shot still and
// clip that shell.Camera and shell.Audio provide. Frames arrive as *image.RGBA,
// the warp is a plain Go pixel loop (effect.go, pure and unit-tested), and the
// result is handed to one widget.Canvas. No shader, no platform image pipeline.
//
// Live capture is implemented by the web shell today; the native shells leave
// it nil and this app hides the affordance and says why, which is what every
// capability is supposed to do where a platform doesn't provide it.
//
//	gophics dev -p web ./examples/mirror
package main

import (
	"fmt"
	"image"
	"log"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// numBands is how many spectrum bands the effect and the meter use. The
// Microphone contract folds the analyser's own resolution onto whatever length
// is asked for, so this is purely a question of how coarse the columns look.
const numBands = 48

var (
	bg     = paint.RGB(0.05, 0.05, 0.07)
	dim    = paint.Color{R: 1, G: 1, B: 1, A: 0.55}
	barCol = paint.RGB(0.45, 0.86, 0.98)
)

type App struct{}

func (App) CreateState() widget.State { return &mirror{} }

type mirror struct {
	widget.StateBase[App]
	ctx widget.Ctx

	frames shell.Frames
	mon    shell.Monitor
	source Source // a test-installed source, in place of the camera

	bands  []float32
	level  float32
	smooth float32 // level, low-passed, so the image doesn't flicker on consonants
	t      float32

	// Output is double-buffered for the same reason the shell rotates frame
	// buffers: the scene compares images by identity, so handing the canvas the
	// same *image.RGBA with new pixels in it would never repaint.
	out  [2]*image.RGBA
	cur  int
	show *image.RGBA

	amount   float32
	mirrored bool
	starting bool
	err      string
	fps      float32
	lastWarp time.Duration
}

// Source lets a test drive the app with frames and audio of its own. When one
// is installed the capability path is skipped entirely, so the whole app — not
// just the effect — is exercisable headless.
type Source interface {
	Frame() *image.RGBA
	Level() float32
	Bands(dst []float32) int
}

var (
	// testSource, if set, replaces the platform capabilities on mount.
	testSource Source
	// stateHook, if set, receives the state on mount — for tests to drive input.
	stateHook func(*mirror)
)

func (m *mirror) Init(ctx widget.Ctx) {
	m.ctx = ctx
	m.bands = make([]float32, numBands)
	m.amount = 0.75
	m.mirrored = true
	if testSource != nil {
		m.source = testSource
	}
	ctx.AddTicker(m)
	if stateHook != nil {
		stateHook(m)
	}
}

// Dispose releases the camera and microphone. Without this the capture light
// stays on after the app closes, which is the kind of bug a user notices and
// does not forgive.
func (m *mirror) Dispose() { m.stop() }

func (m *mirror) stop() {
	if m.frames != nil {
		m.frames.Stop()
		m.frames = nil
	}
	if m.mon != nil {
		m.mon.Stop()
		m.mon = nil
	}
}

// live reports whether this platform can capture for real.
//
// Both are required. The app is a camera warped by a voice, so without either
// one there is nothing to show — and it says so rather than substituting
// something. A drawing stood in here once; it made the demo look like it worked
// on platforms where it did not.
func (m *mirror) live() bool {
	return m.ctx.CameraPreview() != nil && m.ctx.Microphone() != nil
}

func (m *mirror) running() bool { return m.source != nil || m.frames != nil }

// start opens both streams. It must be called from a tap: browsers only honour
// getUserMedia inside a user gesture, so there is no starting this on mount.
func (m *mirror) start() {
	if m.starting || m.running() {
		return
	}
	m.SetState(func() { m.starting, m.err = true, "" })

	m.ctx.CameraPreview().Start(shell.PreviewOptions{Facing: shell.FacingFront, Width: 640},
		func(f shell.Frames, err error) {
			m.SetState(func() {
				m.starting = false
				if err != nil {
					m.err = "camera: " + err.Error()
					return
				}
				m.frames = f
			})
		})

	if m.ctx.Microphone() == nil {
		return // camera-only platform: the effect runs, it just does not breathe
	}
	m.ctx.Microphone().Listen(func(mon shell.Monitor, err error) {
		m.SetState(func() {
			if err != nil {
				// A mirror with no microphone is still a mirror; it just sits
				// still. Losing the camera is fatal, losing the mic is not.
				m.err = "microphone: " + err.Error()
				return
			}
			m.mon = mon
		})
	})
}

// Tick pulls a frame and a spectrum, warps one into the other, and asks for a
// repaint. Everything here is polled rather than pushed: nothing is captured
// for a frame the app was never going to draw.
func (m *mirror) Tick(dt float64) bool {
	if dt > 0.1 {
		dt = 0.1
	}
	m.t += float32(dt)
	if dt > 0 {
		m.fps += (float32(1/dt) - m.fps) * 0.05
	}
	if !m.running() {
		return true
	}

	m.readAudio()
	src := m.readFrame()
	if src == nil {
		m.ctx.Invalidate()
		return true
	}

	dst := m.buffer(src.Bounds())
	start := time.Now()
	Warp(dst, src, Params{
		Level: m.smooth, Bands: m.bands, T: m.t,
		Amount: m.amount, Mirror: m.mirrored,
	})
	m.lastWarp = time.Since(start)
	m.show = dst
	m.ctx.Invalidate()
	return true
}

func (m *mirror) readAudio() {
	switch {
	case m.source != nil:
		m.level = m.source.Level()
		m.source.Bands(m.bands)
	case m.mon != nil:
		m.level = m.mon.Level()
		m.mon.Bands(m.bands)
	default:
		m.level = 0
		clear(m.bands)
	}
	// Attack fast, release slow: the image should jump on a syllable and settle
	// afterwards, not chatter at the frame rate.
	k := float32(0.35)
	if m.level < m.smooth {
		k = 0.10
	}
	m.smooth += (m.level - m.smooth) * k
}

func (m *mirror) readFrame() *image.RGBA {
	if m.source != nil {
		return m.source.Frame()
	}
	return m.frames.Frame()
}

// buffer returns the next output image, reallocating only when the camera
// changes frame size.
func (m *mirror) buffer(r image.Rectangle) *image.RGBA {
	m.cur = (m.cur + 1) % len(m.out)
	if m.out[m.cur] == nil || m.out[m.cur].Bounds() != r {
		m.out[m.cur] = image.NewRGBA(r)
	}
	return m.out[m.cur]
}

// --- Build -------------------------------------------------------------------

func (m *mirror) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Dark() // a mirror is a lit rectangle in a dark room
	var body widget.Widget
	if m.running() {
		body = widget.Canvas{Clip: true, Draw: m.draw}
	} else {
		body = m.idle(th)
	}
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: bg,
		Child: widget.Flex{
			Axis:       layout.Vertical,
			CrossAlign: layout.CrossStretch,
			Children: []widget.Widget{
				widget.Expand(body),
				m.controls(th),
			},
		}}}
}

func (m *mirror) idle(th theme.Theme) widget.Widget {
	label := "Start the mirror"
	if m.starting {
		label = "Asking permission…"
	}
	kids := []widget.Widget{
		widget.Text{S: "Mirror", Font: theme.FontBold, Size: th.Type.Display, Color: th.Text},
		widget.Sized{H: 8},
		widget.Text{S: "Your camera, warped by your voice. Nothing leaves the device, " +
			"and nothing is recorded — the frames are read, drawn, and dropped.",
			Size: th.Type.Body, Color: th.Muted, Wrap: true},
		widget.Sized{H: 20},
	}
	if !m.live() {
		// No substitute, and no button for a stream that cannot open. Saying so
		// is the honest thing a capability-gated demo does.
		kids = append(kids, widget.Text{
			S:    "This platform has no camera and microphone available to the app yet, so there is nothing to mirror.",
			Size: th.Type.Body, Color: th.Muted, Wrap: true,
		})
		return widget.Center(widget.Sized{W: 420, Child: widget.Padding{All: 24,
			Child: widget.Flex{Axis: layout.Vertical, CrossAlign: layout.CrossStart, Children: kids}}})
	}
	kids = append(kids, theme.Button{Label: label, Primary: true, OnTap: m.start})
	if m.err != "" {
		kids = append(kids, widget.Sized{H: 14},
			widget.Text{S: m.err, Size: th.Type.Label, Color: th.Danger, Wrap: true})
	}
	return widget.Center(widget.Sized{W: 420, Child: widget.Padding{All: 24,
		Child: widget.Flex{Axis: layout.Vertical, CrossAlign: layout.CrossStart, Children: kids}}})
}

func (m *mirror) controls(th theme.Theme) widget.Widget {
	if !m.running() {
		return widget.Sized{H: 0}
	}
	// The rate is a running average, so it means nothing for the first second —
	// and nothing at all under the headless thumbnail renderer, which doesn't
	// step in real time. Report it once there is something to report.
	status := ""
	if m.fps >= 1 {
		status = fmt.Sprintf("%.0f fps · warp %.1f ms", m.fps, float64(m.lastWarp.Microseconds())/1000)
	}
	row := widget.Row(
		widget.Sized{W: 200, Child: widget.Flex{
			Axis:       layout.Vertical,
			CrossAlign: layout.CrossStretch,
			Children: []widget.Widget{
				widget.Text{S: "Effect", Size: th.Type.Caption, Color: th.Muted},
				widget.Sized{H: 4},
				theme.Slider{Value: m.amount, Label: "Effect amount",
					OnChange: func(v float32) { m.SetState(func() { m.amount = v }) }},
			},
		}},
		widget.Sized{W: 18},
		theme.Checkbox{Checked: m.mirrored, Label: "Mirror",
			OnChange: func(v bool) { m.SetState(func() { m.mirrored = v }) }},
		widget.Expand(widget.Align{X: 1, Y: 0.5,
			Child: widget.Text{S: status, Font: "mono", Size: th.Type.Caption, Color: th.Muted}}),
		widget.Sized{W: 14},
		theme.Button{Label: "Stop", OnTap: func() { m.SetState(m.stop) }},
	)
	row.CrossAlign = layout.CrossCenter
	return widget.Padding{All: 14, Child: row}
}

// draw paints the warped frame to fill the surface, then the spectrum over it.
func (m *mirror) draw(c paint.Canvas, sz geom.Size) {
	c.Clear(bg)
	if m.show == nil {
		c.TextIn("", "waiting for the camera…", geom.Pt{X: 24, Y: sz.H / 2}, 15, dim)
		return
	}
	b := m.show.Bounds()
	// Cover the surface, preserving aspect: a mirror with letterbox bars looks
	// like a video player, not a mirror.
	scale := sz.W / float32(b.Dx())
	if s := sz.H / float32(b.Dy()); s > scale {
		scale = s
	}
	w, h := float32(b.Dx())*scale, float32(b.Dy())*scale
	c.Image(m.show, geom.RectXYWH((sz.W-w)/2, (sz.H-h)/2, w, h))
	m.drawSpectrum(c, sz)
}

func (m *mirror) drawSpectrum(c paint.Canvas, sz geom.Size) {
	const pad = 18
	maxH := sz.H * 0.14
	bw := (sz.W - pad*2) / float32(len(m.bands))
	for i, v := range m.bands {
		bh := clamp01(v) * maxH
		if bh < 2 {
			bh = 2
		}
		x := pad + float32(i)*bw
		c.FillRRect(geom.RectXYWH(x, sz.H-pad-bh, bw*0.72, bh), bw*0.3,
			barCol.WithAlpha(0.30+0.55*clamp01(v)))
	}
}

func main() {
	if err := app.Run(App{}, app.Config{
		Title:          "Mirror",
		AppID:          "com.gophics.mirror",
		Size:           geom.Size{W: 960, H: 720},
		Background:     bg,
		BackgroundDark: bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
