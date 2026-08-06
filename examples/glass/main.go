// Command glass demonstrates the frosted-glass material: theme.Glass() makes
// themed surfaces translucent over a paint.Canvas backdrop blur, so panels
// layered on a colorful backdrop show it softly through them. The blur is a real
// primitive (paint.Canvas.BackdropBlur) on both paths — a pixmap box blur on the
// CPU rasterizer, and a reduced-offscreen downsample composite on the GPU.
//
//	go run ./examples/glass
package main

import (
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type App struct{}

func (App) CreateState() widget.State { return &state{} }

type state struct{ widget.StateBase[App] }

func (s *state) Build(ctx widget.Ctx) widget.Widget {
	// The glass material as a theme: translucent surfaces + a backdrop blur.
	// Provide GlassDark in dark mode so the tint suits the backdrop.
	th := theme.Glass()
	if ctx.DarkMode() {
		th = theme.GlassDark()
	}
	return widget.Provide[theme.Theme]{
		Value: th,
		Child: widget.Stack{Children: []widget.Widget{
			widget.Canvas{Draw: drawBackdrop}, // vivid backdrop, seen through the glass
			widget.Padding{All: 34, Child: panels()},
		}},
	}
}

// panels is a column of frosted theme.Cards over the backdrop.
func panels() widget.Widget {
	card := func(title, body string) widget.Widget {
		return widget.Padding{Insets: geom.Insets{Bottom: 18}, Child: theme.Card{Pad: 22, Child: widget.Flex{
			CrossAlign: layout.CrossStart,
			Children: []widget.Widget{
				theme.Title(title),
				widget.Padding{Insets: geom.Insets{Top: 6}, Child: theme.Body(body)},
			},
		}}}
	}
	return widget.Flex{CrossAlign: layout.CrossStretch, Children: []widget.Widget{
		card("Frosted glass", "theme.Glass() makes surfaces translucent over a real backdrop blur — one token, the whole app frosts."),
		card("Same tokens", "Accent, type scale, and chart palette are inherited from Light; only the material changes."),
		card("Not the mandate", "Glass is a theme, not the framework's look. Swap it back to Light or Dark and the panels go opaque."),
	}}
}

// drawBackdrop paints vivid soft blobs so the blur behind the glass is obvious.
func drawBackdrop(c paint.Canvas, size geom.Size) {
	c.Clear(paint.RGB(0.09, 0.10, 0.16))
	blobs := []struct {
		x, y, r float32
		col     paint.Color
	}{
		{0.12, 0.16, 0.26, paint.RGB(0.96, 0.42, 0.36)},
		{0.86, 0.12, 0.30, paint.RGB(0.28, 0.68, 0.96)},
		{0.52, 0.58, 0.34, paint.RGB(0.52, 0.86, 0.42)},
		{0.90, 0.78, 0.26, paint.RGB(0.96, 0.78, 0.26)},
		{0.16, 0.82, 0.30, paint.RGB(0.70, 0.40, 0.92)},
	}
	d := min(size.W, size.H)
	for _, b := range blobs {
		cx, cy, rad := b.x*size.W, b.y*size.H, b.r*d
		c.FillRRect(geom.RectXYWH(cx-rad, cy-rad, 2*rad, 2*rad), rad, b.col)
	}
}

func main() {
	if err := app.Run(App{}, app.Config{
		Title:        "Glass",
		Size:         geom.Size{W: 760, H: 540},
		Background:   paint.RGB(0.09, 0.10, 0.16),
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
