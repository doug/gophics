package widget

import (
	"image"
	"math"
	"time"

	"github.com/doug/gossamer/anim"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// Text displays text: single-line by default, word-wrapped when Wrap is
// set, with optional decorations.
type Text struct {
	S         string
	Font      string  // named font family ("" = default; e.g. "bold")
	Size      float32 // 0 → 14
	Color     paint.Color
	Wrap      bool
	Strike    bool
	Underline bool
	// MaxLines caps wrapped lines (0 = unlimited); Ellipsis truncates
	// overflow with "…" (single line to width, or wrapped at MaxLines).
	MaxLines int
	Ellipsis bool
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
	tb.Text, tb.Font, tb.TextSize, tb.Color = t.S, t.Font, t.size(), t.Color
	tb.Wrap, tb.Strike, tb.Underline = t.Wrap, t.Strike, t.Underline
	tb.MaxLines, tb.Ellipsis = t.MaxLines, t.Ellipsis
}
func (t Text) childWidgets() []Widget          { return nil }
func (t Text) attach(layout.Box, []layout.Box) {}

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
// wheel/trackpad and drag with fling deceleration. The offset is retained
// widget state.
type Scroll struct {
	Axis  layout.Axis
	Child Widget
	// OnOffset reports scroll offset changes with the viewport's main-axis
	// extent (LazyList uses this to window its children).
	OnOffset func(offset, viewportExtent float32)
	// OnEndReached fires once when scrolling comes within EndThreshold of
	// the bottom — for infinite feeds. Re-arms after scrolling back up.
	OnEndReached func()
	EndThreshold float32 // 0 → 400 logical px
	// Controller, if set, exposes programmatic scrolling and the live
	// offset to the app.
	Controller *ScrollController
	// OnRefresh, if set, enables pull-to-refresh: dragging down past the top
	// reveals a spinner, and releasing past the trigger distance fires
	// OnRefresh. The app does its work, then clears Refreshing to retract.
	OnRefresh func()
	// Refreshing is app-controlled: hold it true while a refresh runs (the
	// spinner stays out), then set it false to snap the indicator away.
	Refreshing bool
	// Reverse anchors the scroll to the end (bottom/right): the newest
	// content shows first and appended content stays pinned — the chat-log
	// layout. OnEndReached then fires at the far (oldest) end, for loading
	// history. Not combined with pull-to-refresh.
	Reverse bool
}

func (s Scroll) CreateState() State { return &scrollState{} }

// ScrollController is a handle for reading and driving a Scroll's position
// from outside it. Hold one in your state, pass it to Scroll.Controller.
type ScrollController struct {
	s *scrollState
}

// Offset is the current scroll offset (0 = start).
func (c *ScrollController) Offset() float32 {
	if c.s == nil {
		return 0
	}
	return c.s.offset
}

// MaxOffset is the furthest scrollable offset as of the last layout.
func (c *ScrollController) MaxOffset() float32 {
	if c.s == nil || c.s.vp.box == nil {
		return 0
	}
	return c.s.vp.box.MaxOffset()
}

// JumpTo sets the offset immediately (clamped).
func (c *ScrollController) JumpTo(offset float32) {
	if c.s != nil {
		c.s.jumpTo(offset)
	}
}

// AnimateTo scrolls smoothly to offset over the duration.
func (c *ScrollController) AnimateTo(offset float32, d time.Duration) {
	if c.s != nil {
		c.s.animateTo(offset, d)
	}
}

type scrollState struct {
	StateBase[Scroll]
	ctx    Ctx
	offset float32
	vp     *viewportRef

	fling     flinger
	velocity  float32 // px/s along the main axis (drag direction)
	lastDrag  time.Time
	endArmed  bool
	glide     *anim.Controller
	glideFrom float32
	glideTo   float32

	// Pull-to-refresh state.
	overscroll float32 // logical px the content is pulled past the top
	refreshing bool    // latched true from trigger until the app clears it
	spin       spinner // continuous rotation ticker for the indicator
	snap       *anim.Controller
	snapFrom   float32
	snapTo     float32

	// Scrollbar fade: 1 right after scrolling, decaying to 0 when idle.
	barFade float32
	barTick scrollbarFade
}

type viewportRef struct{ box *layout.Viewport }

func (s *scrollState) Init(ctx Ctx) {
	s.ctx = ctx
	s.vp = &viewportRef{}
	s.fling.s = s
	s.endArmed = true
	ctx.AddTicker(&s.fling)
	s.glide = &anim.Controller{Curve: anim.EaseInOut, OnChange: func() {
		s.SetState(func() { s.offset = geom.LerpFloat(s.glideFrom, s.glideTo, s.glide.Value()) })
		s.reportOffset()
	}}
	ctx.AddTicker(s.glide)
	s.snap = &anim.Controller{Curve: anim.EaseOut, OnChange: func() {
		s.SetState(func() { s.overscroll = geom.LerpFloat(s.snapFrom, s.snapTo, s.snap.Value()) })
	}}
	ctx.AddTicker(s.snap)
	s.spin.s = s
	ctx.AddTicker(&s.spin)
	s.barTick.s = s
	ctx.AddTicker(&s.barTick)
	if c := s.W().Controller; c != nil {
		c.s = s
	}
}

func (s *scrollState) Dispose() {
	s.ctx.RemoveTicker(&s.fling)
	s.ctx.RemoveTicker(s.glide)
	s.ctx.RemoveTicker(s.snap)
	s.ctx.RemoveTicker(&s.spin)
	s.ctx.RemoveTicker(&s.barTick)
}

// scrollbarFade decays the scrollbar's opacity toward 0 when scrolling stops
// (reportOffset resets it to 1 on each move), so the indicator shows during
// scrolling and quietly fades out when idle.
type scrollbarFade struct{ s *scrollState }

func (b *scrollbarFade) Tick(dt float64) bool {
	if b.s.barFade <= 0 {
		return false
	}
	b.s.barFade -= float32(dt) / 0.7 // ~0.7s fade-out
	if b.s.barFade < 0 {
		b.s.barFade = 0
	}
	b.s.ctx.Invalidate()
	return b.s.barFade > 0
}

func (s *scrollState) jumpTo(offset float32) {
	s.fling.active = false
	s.glide.Jump(1)
	s.SetState(func() {
		s.offset = offset
		if s.vp.box != nil {
			if m := s.vp.box.MaxOffset(); s.offset > m {
				s.offset = m
			}
		}
		if s.offset < 0 {
			s.offset = 0
		}
	})
	s.reportOffset()
}

func (s *scrollState) animateTo(offset float32, d time.Duration) {
	s.fling.active = false
	s.glideFrom = s.offset
	s.glideTo = offset
	s.glide.Duration = d
	s.glide.Jump(0)
	s.glide.Forward()
	s.ctx.Invalidate()
}

// reportOffset notifies OnOffset and fires OnEndReached near the bottom.
func (s *scrollState) reportOffset() {
	s.barFade = 1 // scrolling happened: show the scrollbar, then it fades
	w := s.W()
	var extent float32
	if s.vp.box != nil {
		sz := s.vp.box.Size()
		if w.Axis == layout.Horizontal {
			extent = sz.W
		} else {
			extent = sz.H
		}
	}
	if cb := w.OnOffset; cb != nil {
		cb(s.offset, extent)
	}
	if w.OnEndReached != nil && s.vp.box != nil {
		thr := w.EndThreshold
		if thr <= 0 {
			thr = 400
		}
		remaining := s.vp.box.MaxOffset() - s.offset
		if remaining <= thr {
			if s.endArmed {
				s.endArmed = false
				w.OnEndReached()
			}
		} else if remaining > thr*1.5 {
			s.endArmed = true // re-arm after scrolling back up
		}
	}
}

func (s *scrollState) mainDelta(d geom.Pt) float32 {
	if s.W().Axis == layout.Horizontal {
		return d.X
	}
	return d.Y
}

// scrollBy moves the content by delta (positive scrolls further down/right)
// and reports whether the offset hit an edge.
func (s *scrollState) scrollBy(delta float32) bool {
	if delta == 0 {
		return false
	}
	clamped := false
	s.SetState(func() {
		s.offset += delta
		if s.vp.box != nil {
			if m := s.vp.box.MaxOffset(); s.offset > m {
				s.offset, clamped = m, true
			}
		}
		if s.offset < 0 {
			s.offset, clamped = 0, true
		}
	})
	s.reportOffset()
	return clamped
}

// contentDelta moves the content to follow a finger/wheel/fling delta dm
// (down/right positive), converting to an offset change that respects the
// scroll direction. Returns whether an edge was hit.
func (s *scrollState) contentDelta(dm float32) bool {
	if s.W().Reverse {
		return s.scrollBy(dm)
	}
	return s.scrollBy(-dm)
}

// flinger decelerates the scroll after release: exponential friction,
// stopping at rest or at an edge.
type flinger struct {
	s      *scrollState
	v      float32 // px/s
	active bool
}

const (
	flingFriction = 3.5 // 1/s decay rate
	flingMinStart = 80  // px/s needed to start a fling
	flingMinSpeed = 20  // px/s considered at rest
)

func (f *flinger) Tick(dt float64) bool {
	if !f.active {
		return false
	}
	if f.s.contentDelta(f.v * float32(dt)) {
		f.active = false // edge reached
		return false
	}
	f.v *= float32(math.Exp(-flingFriction * dt))
	if f.v < flingMinSpeed && f.v > -flingMinSpeed {
		f.active = false
	}
	return f.active
}

// Pull-to-refresh tuning (logical px).
const (
	overscrollResist = 0.5 // rubber-band factor while pulling past the top
	refreshTrigger   = 64  // pull distance that fires OnRefresh on release
	refreshRest      = 56  // indicator height held while a refresh runs
)

// dragMain applies one main-axis finger delta (down = positive). While
// pull-to-refresh is enabled and the content is at the top, downward drag
// feeds a rubber-banded overscroll instead of scrolling.
func (s *scrollState) dragMain(dm float32) {
	w := s.W()
	if w.OnRefresh != nil && !s.refreshing {
		if s.overscroll > 0 {
			ns := s.overscroll + dm*overscrollResist
			if ns <= 0 {
				s.SetState(func() { s.overscroll = 0 })
				s.scrollBy(-(ns / overscrollResist)) // spend the leftover on scroll
			} else {
				s.SetState(func() { s.overscroll = ns })
			}
			return
		}
		if s.offset <= 0 && dm > 0 {
			s.SetState(func() { s.overscroll = dm * overscrollResist })
			return
		}
	}
	s.contentDelta(dm)
}

// animateOverscrollTo springs the overscroll to a resting value.
func (s *scrollState) animateOverscrollTo(to float32) {
	s.snapFrom, s.snapTo = s.overscroll, to
	s.snap.Jump(0)
	s.snap.Forward()
	s.ctx.Invalidate()
}

// releaseRefresh decides, on drag release, whether the pull was far enough
// to trigger a refresh (hold the indicator) or should spring back.
func (s *scrollState) releaseRefresh() {
	if s.overscroll <= 0 {
		return
	}
	if s.W().OnRefresh != nil && !s.refreshing && s.overscroll >= refreshTrigger {
		s.refreshing = true
		s.animateOverscrollTo(refreshRest)
		s.W().OnRefresh()
	} else if !s.refreshing {
		s.animateOverscrollTo(0)
	}
}

// spinner drives the pull-to-refresh indicator's continuous rotation; it
// ticks only while the indicator is on screen.
type spinner struct {
	s     *scrollState
	phase float32 // 0..1 rotation
}

func (sp *spinner) Tick(dt float64) bool {
	if sp.s.refreshing {
		sp.phase += float32(dt) // ~1 revolution / second
		if sp.phase >= 1 {
			sp.phase -= 1
		}
		return true
	}
	return sp.s.overscroll > 0 // stay alive while springing back
}

func (s *scrollState) Build(ctx Ctx) Widget {
	w := s.W()
	// The app cleared Refreshing: retract the indicator.
	if s.refreshing && !w.Refreshing {
		s.refreshing = false
		s.animateOverscrollTo(0)
	}
	dragAxis := DragVertical
	if w.Axis == layout.Horizontal {
		dragAxis = DragHorizontal
	}
	inner := Interactive{
		Handler: Handler{
			DragAxis: dragAxis, // so a cross-axis swipe (Dismissible) can nest
			OnScroll: func(d geom.Pt) {
				s.fling.active = false
				s.contentDelta(s.mainDelta(d))
			},
			OnPress: func(geom.Pt) {
				s.fling.active = false // grab stops the fling
				s.velocity, s.lastDrag = 0, time.Now()
			},
			OnDrag: func(_, d geom.Pt) {
				delta := s.mainDelta(d)
				s.dragMain(delta)
				now := time.Now()
				dt := now.Sub(s.lastDrag).Seconds()
				s.lastDrag = now
				if dt > 0.001 && dt < 0.1 {
					inst := delta / float32(dt)
					s.velocity = s.velocity*0.7 + inst*0.3
				}
			},
			OnRelease: func() {
				s.releaseRefresh()
				if s.velocity > flingMinStart || s.velocity < -flingMinStart {
					s.fling.v = s.velocity
					s.fling.active = true
					ctx.Invalidate()
				}
			},
		},
		Child: viewport{Axis: w.Axis, Offset: s.offset, Lead: s.overscroll, Reverse: w.Reverse, Ref: s.vp, Child: w.Child},
	}
	// Overlay the fading scroll indicator, and the pull-to-refresh spinner
	// when enabled, above the content.
	layers := []Widget{inner, scrollbar{s: s}}
	if w.OnRefresh != nil {
		layers = append(layers, Align{X: 0.5, Y: 0, Child: refreshIndicator{
			extent:   s.overscroll,
			progress: s.overscroll / refreshTrigger,
			spinning: s.refreshing,
			phase:    s.spin.phase,
		}})
	}
	return Stack{Children: layers}
}

// scrollbar overlays a thin, fading position indicator on the scroll's
// trailing edge — so scrollable regions are discoverable. It reads the live
// scroll state each paint and never intercepts input.
type scrollbar struct{ s *scrollState }

func (b scrollbar) createBox(Ctx) layout.Box        { return &scrollbarBox{s: b.s} }
func (b scrollbar) updateBox(_ Ctx, box layout.Box) { box.(*scrollbarBox).s = b.s }
func (b scrollbar) childWidgets() []Widget          { return nil }
func (b scrollbar) attach(layout.Box, []layout.Box) {}

type scrollbarBox struct {
	layout.Base
	s    *scrollState
	size geom.Size
}

func (b *scrollbarBox) Layout(cs layout.Constraints) geom.Size {
	b.size = cs.Constrain(cs.Max) // fill the scroll area
	return b.size
}

func (b *scrollbarBox) Size() geom.Size                { return b.size }
func (b *scrollbarBox) AddHits(geom.Pt, *[]layout.Hit) {} // decorative only

func (b *scrollbarBox) Paint(c paint.Canvas, at geom.Pt) {
	s := b.s
	if s == nil || s.vp.box == nil {
		return
	}
	maxOff := s.vp.box.MaxOffset()
	if maxOff <= 0 || s.barFade <= 0 {
		return // not scrollable, or faded out
	}
	horiz := s.W().Axis == layout.Horizontal
	extent := b.size.H
	if horiz {
		extent = b.size.W
	}
	content := extent + maxOff
	frac := extent / content
	thumbLen := extent * frac
	if thumbLen < 28 {
		thumbLen = 28
	}
	pos := (s.offset / maxOff) * (extent - thumbLen)
	const th, pad = 4, 2
	alpha := s.barFade
	if alpha > 1 {
		alpha = 1
	}
	col := paint.Color{R: 0.6, G: 0.6, B: 0.62, A: alpha * 0.6}
	var r geom.Rect
	if horiz {
		r = geom.RectXYWH(at.X+pos, at.Y+b.size.H-th-pad, thumbLen, th)
	} else {
		r = geom.RectXYWH(at.X+b.size.W-th-pad, at.Y+pos, th, thumbLen)
	}
	c.FillRRect(r, th/2, col)
}

// refreshIndicator draws the spoke spinner centered in the pulled-open band.
type refreshIndicator struct {
	extent   float32 // band height (== overscroll)
	progress float32 // 0..1 pull toward trigger
	spinning bool
	phase    float32
}

func (r refreshIndicator) createBox(Ctx) layout.Box { return &refreshBox{} }
func (r refreshIndicator) updateBox(_ Ctx, b layout.Box) {
	rb := b.(*refreshBox)
	rb.extent, rb.progress, rb.spinning, rb.phase = r.extent, r.progress, r.spinning, r.phase
}
func (r refreshIndicator) childWidgets() []Widget          { return nil }
func (r refreshIndicator) attach(layout.Box, []layout.Box) {}

type refreshBox struct {
	layout.Base
	extent   float32
	progress float32
	spinning bool
	phase    float32
	size     geom.Size
}

func (b *refreshBox) Layout(cs layout.Constraints) geom.Size {
	w := cs.Max.W
	if !cs.BoundedW() {
		w = 0
	}
	b.size = cs.Constrain(geom.Size{W: w, H: b.extent})
	return b.size
}

func (b *refreshBox) Size() geom.Size { return b.size }

func (b *refreshBox) AddHits(geom.Pt, *[]layout.Hit) {} // never interactive

const refreshSpokes = 12

func (b *refreshBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.extent <= 1 {
		return
	}
	cx := at.X + b.size.W/2
	cy := at.Y + b.size.H/2
	// Radius grows with the pull, capped so it fits the band.
	rad := b.size.H * 0.32
	if rad > 11 {
		rad = 11
	}
	ri, ro := rad*0.5, rad
	head := b.phase * refreshSpokes
	fadeIn := b.progress
	if fadeIn > 1 || b.spinning {
		fadeIn = 1
	}
	for i := 0; i < refreshSpokes; i++ {
		ang := float64(i)/refreshSpokes*2*math.Pi - math.Pi/2
		cos, sin := float32(math.Cos(ang)), float32(math.Sin(ang))
		var alpha float32
		if b.spinning {
			// Comet trail: brightest at the head, fading behind it.
			t := float32(math.Mod(float64(head-float32(i)+refreshSpokes), refreshSpokes)) / refreshSpokes
			alpha = 1 - t
		} else {
			// Reveal spokes in order as the pull approaches the trigger.
			if float32(i)/refreshSpokes <= b.progress {
				alpha = 1
			} else {
				alpha = 0.15
			}
		}
		alpha *= fadeIn
		if alpha <= 0.02 {
			continue
		}
		col := paint.Color{R: 0.55, G: 0.55, B: 0.55, A: alpha}
		c.Line(geom.Pt{X: cx + ri*cos, Y: cy + ri*sin},
			geom.Pt{X: cx + ro*cos, Y: cy + ro*sin}, 2, col)
	}
}

// viewport is the internal render widget behind Scroll.
type viewport struct {
	Axis    layout.Axis
	Offset  float32
	Lead    float32
	Reverse bool
	Ref     *viewportRef
	Child   Widget
}

func (v viewport) createBox(Ctx) layout.Box { return &layout.Viewport{} }
func (v viewport) updateBox(_ Ctx, b layout.Box) {
	vb := b.(*layout.Viewport)
	vb.Axis, vb.Offset, vb.Lead, vb.Reverse = v.Axis, v.Offset, v.Lead, v.Reverse
	if v.Ref != nil {
		v.Ref.box = vb
	}
}
func (v viewport) childWidgets() []Widget { return []Widget{v.Child} }
func (v viewport) attach(b layout.Box, kids []layout.Box) {
	b.(*layout.Viewport).Child = first(kids)
}

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
	layout.Base
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

// Semantics overrides or supplies the semantic description of its subtree
// (label decorative graphics, hide ornaments, group controls). Zero-value
// fields defer to derived semantics.
type Semantics struct {
	Role   layout.Role
	Label  string
	Hidden bool
	Child  Widget
}

func (sw Semantics) createBox(Ctx) layout.Box { return &semBox{} }
func (sw Semantics) updateBox(_ Ctx, b layout.Box) {
	sb := b.(*semBox)
	sb.info = layout.SemInfo{Role: sw.Role, Label: sw.Label, Hidden: sw.Hidden}
	if sb.info.Role == layout.RoleNone && (sw.Label != "" || sw.Hidden) {
		sb.info.Role = layout.RoleGroup
	}
}
func (sw Semantics) childWidgets() []Widget { return []Widget{sw.Child} }
func (sw Semantics) attach(b layout.Box, kids []layout.Box) {
	b.(*semBox).Child = first(kids)
}

type semBox struct {
	info  layout.SemInfo
	Child layout.Box
	size  geom.Size
}

func (b *semBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *semBox) Size() geom.Size { return b.size }

func (b *semBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *semBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil && p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		b.Child.AddHits(p, hits)
	}
}

func (b *semBox) Semantics() layout.SemInfo { return b.info }

func (b *semBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
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
	// A zero dimension fills the available space (bounded), so a Canvas can
	// be a full-width control strip.
	w, h := b.w, b.h
	if w == 0 && cs.BoundedW() {
		w = cs.Max.W
	}
	if h == 0 && cs.BoundedH() {
		h = cs.Max.H
	}
	b.size = cs.Constrain(geom.Size{W: w, H: h})
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

// Semantics derives a role from the handlers: keyboard handlers make a
// text field, tap handlers a button whose activation runs OnTap.
func (b *InteractiveBox) Semantics() layout.SemInfo {
	switch {
	case b.Handler.OnText != nil || b.Handler.OnKey != nil:
		return layout.SemInfo{Role: layout.RoleTextField}
	case b.Handler.OnTap != nil:
		return layout.SemInfo{Role: layout.RoleButton, OnActivate: b.Handler.OnTap}
	}
	return layout.SemInfo{}
}

func (b *InteractiveBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
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
