package widget

import (
	"image"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Image draws an image.Image scaled into its box. W/H set the logical
// size (zero: the image's pixel size). Reuse the same decoded image value
// across builds — identity drives both caching and damage diffing.
type Image struct {
	Src  image.Image
	W, H float32
}

func (iw Image) createBox(Ctx) layout.Box { return &imageBox{} }
func (iw Image) updateBox(_ Ctx, b layout.Box) {
	ib := b.(*imageBox)
	ib.src, ib.w, ib.h = iw.Src, iw.W, iw.H
}
func (iw Image) childWidgets() []Widget          { return nil }
func (iw Image) attach(layout.Box, []layout.Box) {}

type imageBox struct {
	layoutbox.Base
	src  image.Image
	w, h float32
}

func (b *imageBox) Layout(cs layout.Constraints) geom.Size {
	if sz, ok := b.Skip(cs); ok {
		return sz
	}
	w, h := b.w, b.h
	if (w == 0 || h == 0) && b.src != nil {
		bounds := b.src.Bounds()
		if w == 0 {
			w = float32(bounds.Dx())
		}
		if h == 0 {
			h = float32(bounds.Dy())
		}
	}
	return b.Done(cs, cs.Constrain(geom.Size{W: w, H: h}))
}

func (b *imageBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.src != nil {
		c.Image(b.src, geom.Rect{Min: at, Max: at.Add(b.Size().Pt())})
	}
}

func (b *imageBox) AddHits(p geom.Pt, hits *[]layout.Hit) {}
