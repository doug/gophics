package widget

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// Text displays text: single-line by default, word-wrapped when Wrap is
// set, with optional decorations.
type Text struct {
	S         string
	Size      float32 // 0 → 14
	Color     paint.Color
	Wrap      bool
	Strike    bool
	Underline bool
}

func (t Text) size() float32 {
	if t.Size == 0 {
		return 14
	}
	return t.Size
}

func (t Text) createBox(ctx Ctx) layout.Box { return &layout.TextBox{Painter: ctx.Painter()} }
func (t Text) updateBox(ctx Ctx, b layout.Box) {
	tb := b.(*layout.TextBox)
	tb.Text, tb.TextSize, tb.Color = t.S, t.size(), t.Color
	tb.Wrap, tb.Strike, tb.Underline = t.Wrap, t.Strike, t.Underline
}
func (t Text) childWidgets() []Widget               { return nil }
func (t Text) attach(layout.Box, []layout.Box)      {}

// Padding insets its child. Set Insets, or All as shorthand.
type Padding struct {
	Insets geom.Insets
	All    float32
	Child  Widget
}

func (p Padding) insets() geom.Insets {
	if p.All != 0 {
		return geom.InsetsAll(p.All)
	}
	return p.Insets
}

func (p Padding) createBox(Ctx) layout.Box { return &layout.Padded{} }
func (p Padding) updateBox(_ Ctx, b layout.Box) {
	b.(*layout.Padded).Insets = p.insets()
}
func (p Padding) childWidgets() []Widget { return []Widget{p.Child} }
func (p Padding) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Padded).Child = first(kids)
}

// Sized forces dimensions (zero = unspecified). With no child it is a spacer.
type Sized struct {
	W, H  float32
	Child Widget
}

func (s Sized) createBox(Ctx) layout.Box { return &layout.Sized{} }
func (s Sized) updateBox(_ Ctx, b layout.Box) {
	sb := b.(*layout.Sized)
	sb.W, sb.H = s.W, s.H
}
func (s Sized) childWidgets() []Widget { return []Widget{s.Child} }
func (s Sized) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Sized).Child = first(kids)
}

// Decorated paints a rounded-rect background and/or border behind its child.
type Decorated struct {
	Color       paint.Color
	Radius      float32
	BorderColor paint.Color
	BorderWidth float32
	Child       Widget
}

func (d Decorated) createBox(Ctx) layout.Box { return &layout.Decorated{} }
func (d Decorated) updateBox(_ Ctx, b layout.Box) {
	db := b.(*layout.Decorated)
	db.Color, db.Radius = d.Color, d.Radius
	db.BorderColor, db.BorderWidth = d.BorderColor, d.BorderWidth
}
func (d Decorated) childWidgets() []Widget { return []Widget{d.Child} }
func (d Decorated) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Decorated).Child = first(kids)
}

// Align positions its child; X/Y in [0,1] (0 start, 0.5 center, 1 end).
type Align struct {
	X, Y  float32
	Child Widget
}

// Center centers its child.
func Center(child Widget) Align { return Align{X: 0.5, Y: 0.5, Child: child} }

func (a Align) createBox(Ctx) layout.Box { return &layout.Aligned{} }
func (a Align) updateBox(_ Ctx, b layout.Box) {
	ab := b.(*layout.Aligned)
	ab.AlignX, ab.AlignY = a.X, a.Y
}
func (a Align) childWidgets() []Widget { return []Widget{a.Child} }
func (a Align) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Aligned).Child = first(kids)
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
	Children   []Widget
}

// Row is a horizontal Flex (children cross-centered).
func Row(children ...Widget) Flex {
	return Flex{Axis: layout.Horizontal, CrossAlign: layout.CrossCenter, Children: children}
}

// Column is a vertical Flex (children cross-centered).
func Column(children ...Widget) Flex {
	return Flex{Axis: layout.Vertical, CrossAlign: layout.CrossCenter, Children: children}
}

func (f Flex) createBox(Ctx) layout.Box { return &layout.Flex{} }
func (f Flex) updateBox(_ Ctx, b layout.Box) {
	fb := b.(*layout.Flex)
	fb.Axis, fb.MainAlign, fb.CrossAlign = f.Axis, f.MainAlign, f.CrossAlign
}

func (f Flex) childWidgets() []Widget {
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
	fb := b.(*layout.Flex)
	fb.Children = fb.Children[:0]
	ki := 0
	for _, c := range f.Children {
		if c == nil {
			continue
		}
		flex := 0
		if fl, ok := c.(Flexible); ok {
			flex = fl.Flex
		}
		fb.Children = append(fb.Children, layout.FlexChild{Box: kids[ki], Flex: flex})
		ki++
	}
}

// Scroll makes its child scrollable along Axis (default vertical) via
// wheel/trackpad and drag. The offset is retained widget state.
type Scroll struct {
	Axis  layout.Axis
	Child Widget
}

func (s Scroll) CreateState() State { return &scrollState{} }

type scrollState struct {
	StateBase[Scroll]
	offset float32
	vp     *viewportRef
}

type viewportRef struct{ box *layout.Viewport }

func (s *scrollState) Init(Ctx) { s.vp = &viewportRef{} }

func (s *scrollState) scrollBy(d geom.Pt) {
	w := s.W()
	delta := d.Y
	if w.Axis == layout.Horizontal {
		delta = d.X
	}
	if delta == 0 {
		return
	}
	s.SetState(func() {
		s.offset += delta
		// Clamp against the last layout when available; Layout re-clamps.
		if s.vp.box != nil {
			if m := s.vp.box.MaxOffset(); s.offset > m {
				s.offset = m
			}
		}
		if s.offset < 0 {
			s.offset = 0
		}
	})
}

func (s *scrollState) Build(Ctx) Widget {
	w := s.W()
	return Interactive{
		Handler: Handler{
			OnScroll: func(d geom.Pt) { s.scrollBy(geom.Pt{X: -d.X, Y: -d.Y}) },
			OnDrag:   func(_, d geom.Pt) { s.scrollBy(geom.Pt{X: -d.X, Y: -d.Y}) },
		},
		Child: viewport{Axis: w.Axis, Offset: s.offset, Ref: s.vp, Child: w.Child},
	}
}

// viewport is the internal render widget behind Scroll.
type viewport struct {
	Axis   layout.Axis
	Offset float32
	Ref    *viewportRef
	Child  Widget
}

func (v viewport) createBox(Ctx) layout.Box { return &layout.Viewport{} }
func (v viewport) updateBox(_ Ctx, b layout.Box) {
	vb := b.(*layout.Viewport)
	vb.Axis, vb.Offset = v.Axis, v.Offset
	if v.Ref != nil {
		v.Ref.box = vb
	}
}
func (v viewport) childWidgets() []Widget { return []Widget{v.Child} }
func (v viewport) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Viewport).Child = first(kids)
}

// Canvas is the custom-painting escape hatch: a fixed-size leaf that paints
// itself with the given function. Draw is called with the widget's rect in
// canvas coordinates.
type Canvas struct {
	W, H float32
	Draw func(c paint.Canvas, r geom.Rect)
}

func (cw Canvas) createBox(Ctx) layout.Box { return &canvasBox{} }
func (cw Canvas) updateBox(_ Ctx, b layout.Box) {
	cb := b.(*canvasBox)
	cb.w, cb.h, cb.draw = cw.W, cw.H, cw.Draw
}
func (cw Canvas) childWidgets() []Widget          { return nil }
func (cw Canvas) attach(layout.Box, []layout.Box) {}

type canvasBox struct {
	w, h float32
	draw func(c paint.Canvas, r geom.Rect)
	size geom.Size
}

func (b *canvasBox) Layout(cs layout.Constraints) geom.Size {
	b.size = cs.Constrain(geom.Size{W: b.w, H: b.h})
	return b.size
}

func (b *canvasBox) Size() geom.Size { return b.size }

func (b *canvasBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.draw != nil {
		b.draw(c, geom.Rect{Min: at, Max: at.Add(b.size.Pt())})
	}
}

func (b *canvasBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		*hits = append(*hits, layout.Hit{Box: b, Pos: p})
	}
}

// Interactive makes its child respond to input via Handler callbacks.
// It adds no visuals and takes its child's size.
type Interactive struct {
	Handler Handler
	Child   Widget
}

func (iw Interactive) createBox(ctx Ctx) layout.Box { return &InteractiveBox{} }
func (iw Interactive) updateBox(ctx Ctx, b layout.Box) {
	ib := b.(*InteractiveBox)
	ib.Handler = iw.Handler
	// Autofocus: a focusable widget mounted while nothing has focus takes it.
	if ib.Handler.focusable() && ctx.el.owner.KeyboardTarget == nil {
		ctx.el.owner.KeyboardTarget = &ib.Handler
		if ib.Handler.OnFocus != nil {
			ib.Handler.OnFocus(true)
		}
	}
}
func (iw Interactive) childWidgets() []Widget { return []Widget{iw.Child} }
func (iw Interactive) attach(b layout.Box, kids []layout.Box) {
	b.(*InteractiveBox).Child = first(kids)
}

// InteractiveBox is the render object behind Interactive. The app runner
// type-switches on it in hit paths to dispatch pointer events.
type InteractiveBox struct {
	Handler Handler
	Child   layout.Box
	size    geom.Size
}

func (b *InteractiveBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *InteractiveBox) Size() geom.Size { return b.size }

func (b *InteractiveBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *InteractiveBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X < 0 || p.Y < 0 || p.X >= b.size.W || p.Y >= b.size.H {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func first(kids []layout.Box) layout.Box {
	if len(kids) > 0 {
		return kids[0]
	}
	return nil
}
