// Package apicheck is a compile-time contract for the gophics skill. It names
// every API symbol SKILL.md documents — funcs by value, structs by a literal of
// the fields the skill mentions — so a rename or removal upstream breaks
// `go build ./...` in CI instead of silently leaving the skill teaching a dead
// API. It has no runtime behavior; the blank vars exist only to be type-checked.
//
// When you change SKILL.md's documented surface, mirror the change here.
package apicheck

import (
	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Entry point + config (fields the skill documents).
var (
	_ = app.Run
	_ = app.Config{Title: "", Size: geom.Size{}, Background: paint.Color{}, Font: nil}
)

// State kinds the skill describes.
var (
	_ widget.Stateful  // CreateState() State
	_ widget.Stateless // Build(Ctx) Widget
	_ widget.State
)

// Layout + core primitives (the SKILL.md table).
var (
	_ = widget.Column
	_ = widget.Row
	_ = widget.Center
	_ = widget.Expand
	_ = widget.Spacer
	_ = widget.Flex{}
	_ = widget.Flexible{}
	_ = widget.Align{}
	_ = widget.Padding{All: 0, Insets: geom.Insets{}}
	_ = widget.Sized{W: 0, H: 0}
	_ = widget.Fill{Color: paint.Color{}}
	_ = widget.Stack{}
	_ = widget.Scroll{}
	_ = widget.LazyList{}
	_ = widget.WithKey{}
	_ = widget.Text{S: "", Size: 0, Color: paint.Color{}, Wrap: false}
	_ = widget.Interactive{}
	_ = widget.Handler{OnTap: nil, OnEnter: nil, OnExit: nil, OnPress: nil, OnDrag: nil}
	_ = widget.Canvas{W: 0, H: 0, Clip: false, Draw: nil}
)

// Colors + geometry helpers the skill uses.
var (
	_ = paint.RGB
	_ = paint.Lerp
	_ = paint.Color{R: 0, G: 0, B: 0, A: 0}
	_ = paint.Color{}.WithAlpha
	_ = geom.RectXYWH
	_ = geom.Size{W: 0, H: 0}
	_ = geom.Pt{X: 0, Y: 0}
	_ = geom.Insets{}
)

// paint.Canvas is the custom-draw surface; assert the primitives the skill lists.
type _canvasPrimitives interface {
	Clear(paint.Color)
	FillRect(geom.Rect, paint.Color)
	FillRRect(geom.Rect, float32, paint.Color)
	FillRRectGradient(geom.Rect, float32, paint.Color, paint.Color, bool)
	StrokeRRect(geom.Rect, float32, float32, paint.Color)
	Line(geom.Pt, geom.Pt, float32, paint.Color)
	TextIn(string, string, geom.Pt, float32, paint.Color)
	PushClip(geom.Rect)
	PopClip()
	PushOpacity(float32)
	PopOpacity()
}

var _ = func(c paint.Canvas) _canvasPrimitives { return c } // paint.Canvas satisfies the above
