package widget

import (
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Spacer expands to consume free space along a Row/Column's main axis
// (equivalent to Expand(Sized{})). Use it to push siblings apart.
func Spacer() Widget { return Expand(Sized{}) }

// Fill expands to fill available space and paints an optional background;
// the primitive for page backgrounds and modal scrims.
type Fill struct {
	Color paint.Color
	Child Widget
}

func (f Fill) createBox(Ctx) layout.Box { return &layout.Filled{} }
func (f Fill) updateBox(_ Ctx, b layout.Box) {
	b.(*layout.Filled).Color = f.Color
}
func (f Fill) childWidgets() []Widget { return []Widget{f.Child} }
func (f Fill) soleChild() Widget      { return f.Child }
func (f Fill) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Filled).Child = first(kids)
}

// Opacity fades its child as a group at Alpha [0,1] (1 = opaque, 0 =
// hidden but still laid out). Pair with AnimateFloat for fade transitions.
type Opacity struct {
	Alpha float32
	Child Widget
}

func (o Opacity) createBox(Ctx) layout.Box { return &layout.Opacity{} }
func (o Opacity) updateBox(_ Ctx, b layout.Box) {
	b.(*layout.Opacity).Alpha = o.Alpha
}
func (o Opacity) childWidgets() []Widget { return []Widget{o.Child} }
func (o Opacity) soleChild() Widget      { return o.Child }
func (o Opacity) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Opacity).Child = first(kids)
}

// Transform applies an affine transform to its child when painting (scale,
// rotate, translate) without affecting layout — like CSS transform. Pair with
// AnimateFloat to drive scale/rotate animations; the shared-element flight
// (Hero) builds on it via paint.MapRect.
type Transform struct {
	T paint.Transform
	// Center pivots scale/rotate on the child's center instead of the
	// transform's Pivot fields (which are relative to the child's top-left).
	Center bool
	Child  Widget
}

// Scale returns a Transform that scales child by factor about its top-left.
// (Set Center or use AnimatedScale for centered scaling.)
func Scale(factor float32, child Widget) Transform {
	return Transform{T: paint.Transform{SX: factor, SY: factor}, Child: child}
}

func (t Transform) createBox(Ctx) layout.Box { return &layout.Transformed{} }
func (t Transform) updateBox(_ Ctx, b layout.Box) {
	tb := b.(*layout.Transformed)
	tb.T, tb.Center = t.T, t.Center
}
func (t Transform) childWidgets() []Widget { return []Widget{t.Child} }
func (t Transform) soleChild() Widget      { return t.Child }
func (t Transform) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Transformed).Child = first(kids)
}

// AspectRatio sizes its child to Ratio (width/height), as large as fits.
type AspectRatio struct {
	Ratio float32
	Child Widget
}

func (a AspectRatio) createBox(Ctx) layout.Box { return &layout.AspectRatio{} }
func (a AspectRatio) updateBox(_ Ctx, b layout.Box) {
	b.(*layout.AspectRatio).Ratio = a.Ratio
}
func (a AspectRatio) childWidgets() []Widget { return []Widget{a.Child} }
func (a AspectRatio) soleChild() Widget      { return a.Child }
func (a AspectRatio) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.AspectRatio).Child = first(kids)
}

// Grid arranges children in Columns equal-width columns with Spacing.
// Shrink-wraps its height; wrap in Scroll for long grids.
type Grid struct {
	Columns  int
	Spacing  float32
	Children []Widget
}

func (g Grid) createBox(Ctx) layout.Box { return &layout.Grid{} }
func (g Grid) updateBox(_ Ctx, b layout.Box) {
	gb := b.(*layout.Grid)
	gb.Columns, gb.Spacing = g.Columns, g.Spacing
}
func (g Grid) childWidgets() []Widget { return g.Children }
func (g Grid) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Grid).Children = append(b.(*layout.Grid).Children[:0], kids...)
}

// Wrap flows children left to right, wrapping to new runs; Spacing
// separates children, RunSpacing separates runs.
type Wrap struct {
	Spacing    float32
	RunSpacing float32
	Children   []Widget
}

func (w Wrap) createBox(Ctx) layout.Box { return &layout.Wrap{} }
func (w Wrap) updateBox(_ Ctx, b layout.Box) {
	wb := b.(*layout.Wrap)
	wb.Spacing, wb.RunSpacing = w.Spacing, w.RunSpacing
}
func (w Wrap) childWidgets() []Widget { return w.Children }
func (w Wrap) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Wrap).Children = append(b.(*layout.Wrap).Children[:0], kids...)
}

// Stack layers children (first at the bottom); each fills or centers within
// the stack. Combine with Center/Align/Padding to position, and Fill for a
// full-bleed layer. The overlay system builds on this.
type Stack struct {
	Children []Widget
}

func (s Stack) createBox(Ctx) layout.Box  { return &layout.Stack{} }
func (s Stack) updateBox(Ctx, layout.Box) {}
func (s Stack) childWidgets() []Widget    { return s.Children }
func (s Stack) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Stack).Children = append(b.(*layout.Stack).Children[:0], kids...)
}
