// Command embed-ebiten runs a gophics UI as a translucent overlay on a live
// Ebiten game — the embedding seam, exercised rather than described.
//
// gophics names "UIs embedded inside larger Go programs" a center of gravity,
// and app.NewHandler has always existed "for embedded hosts that own the
// surface and event loop". What was missing is a host that proves it: this is
// roughly 200 lines, and every line of it is something a real embedder has to
// write.
//
// The seam is three things.
//
//   - app.NewHandler gives a shell.Handler. The host calls Frame once per tick
//     and Event for input; gophics never touches the window or the loop.
//   - shell.Window is implemented by the host (below), which is where clipboard,
//     dark mode and invalidation come from.
//   - shell.PixelTarget carries the finished frame back. Put receives a damage
//     rect, so the host uploads the region that changed rather than the screen.
//
// GPU presentation is deliberately not part of the seam: two Go WebGPU bindings
// cannot exchange a Device through Go types, so gophics cannot accept Ebiten's.
// The CPU path is the supported one, and the damage rect is what makes it
// affordable.
//
//	go run .
package main

import (
	"image"
	"image/color"
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"

	"gophics-embed-ebiten/overlay"
)

const (
	winW, winH = 720, 480
	overlayW   = 260 // the HUD occupies a panel, not the whole window
)

func main() {
	g := &game{}
	root := overlay.UI{M: g}

	h, err := app.NewHandler(root, app.Config{
		Size: geom.Size{W: overlayW, H: winH},
		// The overlay composites over live game content, so the background must
		// stay translucent. This is what Config.Transparent is for: it also
		// turns off surface retention, because a blended background replayed
		// over retained pixels would ghost the previous frame.
		Transparent: true,
		Background:  paint.Color{R: 0.05, G: 0.06, B: 0.10, A: 0.82},
		Font:        goregular.TTF,
	})
	if err != nil {
		log.Fatal(err)
	}
	g.h = h
	g.overlay = ebiten.NewImage(overlayW, winH)

	ebiten.SetWindowSize(winW, winH)
	ebiten.SetWindowTitle("gophics over Ebiten")
	if err := ebiten.RunGame(g); err != nil {
		log.Fatal(err)
	}
}

// --- the game, which is only here to have something to overlay ---

type game struct {
	h       shell.Handler
	overlay *ebiten.Image
	frame   *hostFrame
	t       float64

	speed  float32
	paused bool
	clip   string // the host owns the clipboard; here, a process-local one

	// Input state, diffed each tick into shell events.
	lastX, lastY int
	lastDown     bool
}

func (g *game) Layout(int, int) (int, int) { return winW, winH }

func (g *game) Update() error {
	if !g.paused {
		g.t += float64(g.speed) / 60
	}
	g.pumpInput()
	return nil
}

func (g *game) Draw(screen *ebiten.Image) {
	// The "game": a drifting field of dots, so there is live content under the
	// overlay and any ghosting would be obvious.
	screen.Fill(color.RGBA{12, 14, 22, 255})
	for i := range 90 {
		f := float64(i)
		x := math.Mod(f*67+g.t*40, winW)
		y := math.Mod(f*37+math.Sin(g.t+f)*60+winH, winH)
		screen.Set(int(x), int(y), color.RGBA{90, 160, 230, 255})
		screen.Set(int(x)+1, int(y), color.RGBA{60, 110, 170, 255})
	}

	// One gophics frame, drawn into the overlay image and composited.
	g.frame = &hostFrame{g: g}
	g.h.Frame(g, g.frame, 1.0/60)

	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(winW-overlayW, 0)
	screen.DrawImage(g.overlay, op)
}

// --- shell.Frame ---

type hostFrame struct{ g *game }

func (f *hostFrame) Size() geom.Size { return geom.Size{W: overlayW, H: winH} }
func (f *hostFrame) Scale() float32  { return 1 }

func (f *hostFrame) Target() shell.Target {
	return shell.PixelTarget{Put: func(img *image.RGBA, damage geom.Rect) {
		// The damage rect is the point of the CPU seam. gophics redrew only the
		// changed region, and uploading only that region is what keeps a
		// per-frame CPU present affordable — for a HUD where one label changed,
		// this is a few rows rather than the whole panel.
		//
		// An empty rect means nothing changed: the overlay image still holds
		// the last frame, so there is nothing to do.
		if damage.IsEmpty() {
			return
		}
		r := image.Rect(int(damage.Min.X), int(damage.Min.Y),
			int(damage.Max.X), int(damage.Max.Y)).
			Intersect(img.Bounds())
		if r.Empty() {
			return
		}
		// WritePixels wants the whole image, so a sub-rect goes through a
		// SubImage draw. Ebiten's ReplacePixels-style path is per-image; this
		// keeps the upload proportional to the damage.
		sub := ebiten.NewImageFromImage(img.SubImage(r))
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(float64(r.Min.X), float64(r.Min.Y))
		op.Blend = ebiten.BlendCopy // replace, do not blend onto the stale frame
		f.g.overlay.DrawImage(sub, op)
	}}
}

// --- overlay.Model: the state the UI reads and writes ---

func (g *game) Elapsed() float64   { return g.t }
func (g *game) Paused() bool       { return g.paused }
func (g *game) TogglePause()       { g.paused = !g.paused }
func (g *game) Speed() float32     { return g.speed }
func (g *game) SetSpeed(s float32) { g.speed = s }

// --- shell.Window ---
//
// The host owns the window, so gophics asks it for these. Everything here is
// what an embedder must decide: there is no default, because the answers depend
// on what the host is.

func (g *game) Invalidate()       {} // Ebiten redraws every tick; nothing to schedule
func (g *game) SetTitle(t string) { ebiten.SetWindowTitle(t) }
func (g *game) Close()            { ebiten.SetWindowClosingHandled(false) }

func (g *game) ClipboardRead() (string, error) { return g.clip, nil }
func (g *game) ClipboardWrite(s string) error  { g.clip = s; return nil }
func (g *game) OpenURL(string) error           { return nil }
func (g *game) DarkMode() bool                 { return true }
