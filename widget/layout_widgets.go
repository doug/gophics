package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
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

func (f Fill) createBox(Ctx) layout.Box { return &layoutbox.Filled{} }
func (f Fill) updateBox(_ Ctx, b layout.Box) {
	b.(*layoutbox.Filled).Color = f.Color
}
func (f Fill) childWidgets() []Widget { return []Widget{f.Child} }
func (f Fill) soleChild() Widget      { return f.Child }
func (f Fill) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Filled).Child = first(kids)
}

// Opacity fades its child as a group at Alpha [0,1] (1 = opaque, 0 =
// hidden but still laid out). Pair with AnimateFloat for fade transitions.
type Opacity struct {
	Alpha float32
	Child Widget
}

func (o Opacity) createBox(Ctx) layout.Box { return &layoutbox.Opacity{} }
func (o Opacity) updateBox(_ Ctx, b layout.Box) {
	b.(*layoutbox.Opacity).Alpha = o.Alpha
}
func (o Opacity) childWidgets() []Widget { return []Widget{o.Child} }
func (o Opacity) soleChild() Widget      { return o.Child }
func (o Opacity) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Opacity).Child = first(kids)
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

func (t Transform) createBox(Ctx) layout.Box { return &layoutbox.Transformed{} }
func (t Transform) updateBox(_ Ctx, b layout.Box) {
	tb := b.(*layoutbox.Transformed)
	tb.T, tb.Center = t.T, t.Center
}
func (t Transform) childWidgets() []Widget { return []Widget{t.Child} }
func (t Transform) soleChild() Widget      { return t.Child }
func (t Transform) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Transformed).Child = first(kids)
}

// AspectRatio sizes its child to Ratio (width/height), as large as fits.
type AspectRatio struct {
	Ratio float32
	Child Widget
}

func (a AspectRatio) createBox(Ctx) layout.Box { return &layoutbox.AspectRatio{} }
func (a AspectRatio) updateBox(_ Ctx, b layout.Box) {
	b.(*layoutbox.AspectRatio).Ratio = a.Ratio
}
func (a AspectRatio) childWidgets() []Widget { return []Widget{a.Child} }
func (a AspectRatio) soleChild() Widget      { return a.Child }
func (a AspectRatio) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.AspectRatio).Child = first(kids)
}

// Grid arranges children in Columns equal-width columns with Spacing.
// Shrink-wraps its height; wrap in Scroll for long grids.
type Grid struct {
	Columns  int
	Spacing  float32
	Children []Widget
}

func (g Grid) createBox(Ctx) layout.Box { return &layoutbox.Grid{} }
func (g Grid) updateBox(_ Ctx, b layout.Box) {
	gb := b.(*layoutbox.Grid)
	gb.Columns, gb.Spacing = g.Columns, g.Spacing
}
func (g Grid) childWidgets() []Widget { return g.Children }
func (g Grid) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Grid).Children = append(b.(*layoutbox.Grid).Children[:0], kids...)
}

// Wrap flows children left to right, wrapping to new runs; Spacing
// separates children, RunSpacing separates runs.
type Wrap struct {
	Spacing    float32
	RunSpacing float32
	Children   []Widget
}

func (w Wrap) createBox(Ctx) layout.Box { return &layoutbox.Wrap{} }
func (w Wrap) updateBox(_ Ctx, b layout.Box) {
	wb := b.(*layoutbox.Wrap)
	wb.Spacing, wb.RunSpacing = w.Spacing, w.RunSpacing
}
func (w Wrap) childWidgets() []Widget { return w.Children }
func (w Wrap) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Wrap).Children = append(b.(*layoutbox.Wrap).Children[:0], kids...)
}

// Stack layers children (first at the bottom); each fills or centers within
// the stack. Combine with Center/Align/Padding to position, and Fill for a
// full-bleed layer. The overlay system builds on this.
type Stack struct {
	Children []Widget
}

func (s Stack) createBox(Ctx) layout.Box  { return &layoutbox.Stack{} }
func (s Stack) updateBox(Ctx, layout.Box) {}
func (s Stack) childWidgets() []Widget    { return s.Children }
func (s Stack) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Stack).Children = append(b.(*layoutbox.Stack).Children[:0], kids...)
}

// Padding insets its child. Set Insets, or All as shorthand.
type Padding struct {
	Insets geom.Insets
	All    float32
	// Start and End are directional: in a left-to-right subtree Start is the
	// left inset and End the right, and in a right-to-left one they swap. Use
	// them for anything that should follow the reading direction (a leading
	// icon's gap, a list row's text indent) and keep Insets.Left/Right for
	// what genuinely means left and right. They are added to Insets, so a
	// widget can mix a fixed vertical inset with a directional horizontal one.
	Start, End float32
	Child      Widget
}

func (p Padding) insets(dir Direction) geom.Insets {
	in := p.Insets
	if p.All != 0 {
		in = geom.InsetsAll(p.All)
	}
	if p.Start == 0 && p.End == 0 {
		return in
	}
	start, end := p.Start, p.End
	if dir.RTL() {
		start, end = end, start
	}
	in.Left += start
	in.Right += end
	return in
}

func (p Padding) createBox(Ctx) layout.Box { return &layoutbox.Padded{} }
func (p Padding) updateBox(ctx Ctx, b layout.Box) {
	b.(*layoutbox.Padded).Insets = p.insets(DirectionOf(ctx))
}
func (p Padding) childWidgets() []Widget { return []Widget{p.Child} }
func (p Padding) soleChild() Widget      { return p.Child }
func (p Padding) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Padded).Child = first(kids)
}

// Sized forces dimensions (zero = unspecified). With no child it is a spacer.
type Sized struct {
	W, H  float32
	Child Widget
}

func (s Sized) createBox(Ctx) layout.Box { return &layoutbox.Sized{} }
func (s Sized) updateBox(_ Ctx, b layout.Box) {
	sb := b.(*layoutbox.Sized)
	sb.W, sb.H = s.W, s.H
}
func (s Sized) childWidgets() []Widget { return []Widget{s.Child} }
func (s Sized) soleChild() Widget      { return s.Child }
func (s Sized) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Sized).Child = first(kids)
}

// Decorated paints a rounded-rect background and/or border behind its child.
type Decorated struct {
	Color       paint.Color
	Radius      float32
	BorderColor paint.Color
	BorderWidth float32
	// Blur, when > 0, frosts the backdrop behind the surface (glass) before the
	// Color tint is painted over it. Pair with a translucent Color.
	Blur  float32
	Child Widget
}

func (d Decorated) createBox(Ctx) layout.Box { return &layoutbox.Decorated{} }
func (d Decorated) updateBox(_ Ctx, b layout.Box) {
	db := b.(*layoutbox.Decorated)
	db.Color, db.Radius = d.Color, d.Radius
	db.BorderColor, db.BorderWidth = d.BorderColor, d.BorderWidth
	db.Blur = d.Blur
}
func (d Decorated) childWidgets() []Widget { return []Widget{d.Child} }
func (d Decorated) soleChild() Widget      { return d.Child }
func (d Decorated) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Decorated).Child = first(kids)
}

// Align positions its child; X/Y in [0,1] (0 start, 0.5 center, 1 end).
type Align struct {
	X, Y float32
	// Directional makes X follow the reading direction: X=0 is the leading
	// edge, which is the left in a left-to-right subtree and the right in a
	// right-to-left one. Leave it off for anything anchored to a real side.
	Directional bool
	Child       Widget
}

// Center centers its child.
func Center(child Widget) Align { return Align{X: 0.5, Y: 0.5, Child: child} }

func (a Align) createBox(Ctx) layout.Box { return &layoutbox.Aligned{} }
func (a Align) updateBox(ctx Ctx, b layout.Box) {
	ab := b.(*layoutbox.Aligned)
	x := a.X
	if a.Directional && DirectionOf(ctx).RTL() {
		x = 1 - x
	}
	ab.AlignX, ab.AlignY = x, a.Y
}
func (a Align) childWidgets() []Widget { return []Widget{a.Child} }
func (a Align) soleChild() Widget      { return a.Child }
func (a Align) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Aligned).Child = first(kids)
}

// Flexible gives a Flex child a share of the remaining main-axis space.
type Flexible struct {
	Flex  int
	Child Widget
}

// Expand wraps w to fill remaining space in a Row/Column (flex 1).
func Expand(w Widget) Flexible { return Flexible{Flex: 1, Child: w} }

// Flex lays out children along an axis. Use Row/Column constructors.
type Flex struct {
	Axis       layout.Axis
	MainAlign  layout.MainAlign
	CrossAlign layout.CrossAlign
	// NoMirror keeps a horizontal Flex in written order even in a
	// right-to-left subtree. A row of controls is read like a sentence and
	// should mirror, which is the default; set this for a row whose order
	// means something physical rather than textual — a media scrubber, a
	// timeline, a chart legend keyed to plotted positions.
	NoMirror bool
	Children []Widget
}

// Row is a horizontal Flex (children cross-centered). In a right-to-left
// subtree it lays its children from the right, so a row reads in the same
// order as the text around it; see Flex.NoMirror to opt out.
func Row(children ...Widget) Flex {
	return Flex{Axis: layout.Horizontal, CrossAlign: layout.CrossCenter, Children: children}
}

// Column is a vertical Flex (children cross-centered).
func Column(children ...Widget) Flex {
	return Flex{Axis: layout.Vertical, CrossAlign: layout.CrossCenter, Children: children}
}

func (f Flex) createBox(Ctx) layout.Box { return &layoutbox.Flex{} }
func (f Flex) updateBox(ctx Ctx, b layout.Box) {
	fb := b.(*layoutbox.Flex)
	fb.Axis, fb.MainAlign, fb.CrossAlign = f.Axis, f.MainAlign, f.CrossAlign
	// Only a horizontal run mirrors: a column reads top-to-bottom in every
	// script gophics supports.
	fb.Reverse = f.Axis == layout.Horizontal && !f.NoMirror && DirectionOf(ctx).RTL()
}

func (f Flex) childWidgets() []Widget {
	// The common case has no Flexible wrappers to unwrap, so f.Children is
	// already exactly the slice the reconciler wants — handing it back
	// directly avoids a fresh allocate-and-copy every frame for every Row/
	// Column in the tree.
	hasFlexible := false
	for _, c := range f.Children {
		if _, ok := c.(Flexible); ok {
			hasFlexible = true
			break
		}
	}
	if !hasFlexible {
		return f.Children
	}
	out := make([]Widget, len(f.Children))
	for i, c := range f.Children {
		if fl, ok := c.(Flexible); ok {
			c = fl.Child
		}
		out[i] = c
	}
	return out
}

func (f Flex) attach(b layout.Box, kids []layout.Box) {
	fb := b.(*layoutbox.Flex)
	fb.Children = fb.Children[:0]
	ki := 0
	for _, c := range f.Children {
		flex := 0
		if fl, ok := c.(Flexible); ok {
			flex, c = fl.Flex, fl.Child // unwrap to match childWidgets
		}
		if c == nil {
			continue // childWidgets yielded nil → the reconciler made no box
		}
		fb.Children = append(fb.Children, layoutbox.FlexChild{Box: kids[ki], Flex: flex})
		ki++
	}
}
