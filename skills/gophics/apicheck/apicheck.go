// Package apicheck is a compile-time contract for the gophics skill. It names
// every API symbol SKILL.md documents — funcs by value, structs by a literal of
// the fields the skill mentions — so a rename or removal upstream breaks
// `go build ./...` in CI instead of silently leaving the skill teaching a dead
// API. It has no runtime behavior; the blank vars exist only to be type-checked.
//
// Every field the skill names is in a literal here. An empty literal only
// proves the type exists, and the skill taught Flex{Direction, Justify} and
// LazyList{Item} for a while past their renaming because the literals that
// were meant to catch it were empty.
//
// When you change SKILL.md's documented surface, mirror the change here.
package apicheck

import (
	"image"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Entry point + config (fields the skill documents).
var (
	_ = app.Run
	_ = app.Config{
		Title: "", Size: geom.Size{}, Background: paint.Color{}, Font: nil,
		FontFamilies: map[string][]byte{theme.FontBold: nil},
		Fallbacks:    nil,
		SystemFonts:  false,
	}
)

// State kinds the skill describes, and what StateBase gives a state.
var (
	_ widget.Stateful  // CreateState() State
	_ widget.Stateless // Build(Ctx) Widget
	_ widget.State
	_ = (&widget.StateBase[widget.Text]{}).W
	_ = (&widget.StateBase[widget.Text]{}).SetState
)

// Layout + core primitives (the SKILL.md table), each with the fields named.
var (
	_ = widget.Column
	_ = widget.Row
	_ = widget.Center
	_ = widget.Expand
	_ = widget.Spacer
	_ = widget.Flex{Axis: 0, MainAlign: 0, CrossAlign: 0, Children: nil}
	_ = widget.Flexible{Flex: 0, Child: nil}
	_ = widget.Align{X: 0, Y: 0, Child: nil}
	_ = widget.Padding{All: 0, Insets: geom.Insets{}, Child: nil}
	_ = widget.Sized{W: 0, H: 0, Child: nil}
	_ = widget.Fill{Color: paint.Color{}, Child: nil}
	_ = widget.Stack{Children: nil}
	_ = widget.Scroll{Child: nil}
	_ = widget.LazyList{Count: 0, Build: nil}
	_ = widget.WithKey{Key: nil, Child: nil}
	_ = widget.Text{Value: "", Font: "", Size: 0, Color: paint.Color{}, Wrap: false}
	_ = widget.Interactive{Gestures: widget.Gestures{}, Child: nil}
	_ = widget.Gestures{OnTap: nil, OnEnter: nil, OnExit: nil, OnPress: nil, OnDrag: nil, OnKey: nil, OnText: nil}
	_ = widget.Canvas{W: 0, H: 0, Clip: false, Draw: nil}
)

// Drag and drop, as the drag-board section describes it.
var (
	_ = widget.Draggable{Payload: nil, Child: nil}
	_ = widget.DropTarget{Accept: nil, OnDrop: nil, Builder: nil}
	_ = widget.LayoutObserver{OnLayout: nil, Child: nil}
	_ = widget.DragHost{Child: nil}
)

// Fields and their focus behaviour (the gotchas section).
var (
	_ = widget.TextField{Autofocus: false}
	_ = theme.Field{Autofocus: false}
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
	FillPath(*paint.Path, paint.Color)
	StrokePath(*paint.Path, float32, paint.Color)
	Line(geom.Pt, geom.Pt, float32, paint.Color)
	TextIn(string, string, geom.Pt, float32, paint.Color)
	Image(image.Image, geom.Rect)
	DrawSprite(image.Image, paint.Sprite)
	PushClip(geom.Rect)
	PopClip()
	PushOpacity(float32)
	PopOpacity()
	PushTransform(paint.Transform)
	PopTransform()
}

var _ = func(c paint.Canvas) _canvasPrimitives { return c } // paint.Canvas satisfies the above

// The skill's custom-draw snippet, as written there.
var _ = widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
	c.Clear(paint.RGB(0.09, 0.10, 0.13))
	c.FillRRect(geom.RectXYWH(20, 20, 120, 80), 8, paint.RGB(0.36, 0.62, 0.98))
	c.TextIn("", "hello", geom.Pt{X: 30, Y: 60}, 16, paint.RGB(1, 1, 1))
}}
