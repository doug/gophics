package widget

import (
	"math"
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

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
	dragVel   dragSampler
	velocity  float32 // px/s along the main axis (drag direction)
	endArmed  bool
	glide     *anim.Controller
	glideFrom float32
	glideTo   float32

	// Overscroll / rubber-band state. overscroll is the signed lead the content
	// is displaced past an edge (positive = pulled past the leading edge/top,
	// negative = past the trailing edge/bottom); it drives the viewport Lead and,
	// at the top with OnRefresh set, the pull-to-refresh indicator. overRaw is
	// the raw (un-rubber-banded) drag distance behind it, so the elastic mapping
	// stays consistent frame to frame.
	overscroll float32
	overRaw    float32
	overSpring overspring // critically-damped spring that settles overscroll to 0
	refreshing bool       // latched true from trigger until the app clears it
	spin       spinner    // continuous rotation ticker for the indicator
	snap       *anim.Controller
	snapFrom   float32
	snapTo     float32

	// Scrollbar fade: 1 right after scrolling, decaying to 0 when idle.
	barFade float32
	barTick scrollbarFade

	// Thumb geometry, measured by the scrollbar's Paint and read by Build to
	// place the drag target over it. It lags by one frame, which is invisible:
	// the bar is hidden until a scroll happens, and that scroll is the frame
	// that measures it.
	barThumbLen  float32 // thumb length along the scroll axis
	barTrackLen  float32 // travel available to the thumb (extent - thumbLen)
	barDragging  bool
	barDragStart float32 // offset when the drag began

	// reveal lets a focused descendant (a TextField caret) ask to be scrolled
	// into view; provided to the child subtree and captured at paint.
	reveal *scrollReveal
}

// scrollReveal is the "scroll a descendant into view" service a Scroll provides
// to its subtree (gophics's analog of Flutter's Scrollable.ensureVisible). The
// content origin is captured each paint by revealAnchor; descendants convert a
// caret rect to this content space and call reveal, which nudges the offset the
// least amount needed to bring the rect inside the viewport (with a small
// margin). Working in content space keeps it stable across offset changes.
type scrollReveal struct {
	s      *scrollState
	origin geom.Pt // content origin (absolute), captured at paint
	have   bool
}

func (r *scrollReveal) beginContent(at geom.Pt) { r.origin, r.have = at, true }

// horizontal reports whether the owning scroll's main axis is horizontal.
func (r *scrollReveal) horizontal() bool { return r.s.W().Axis == layout.Horizontal }

// revealContentMargin is the gap kept between the revealed rect and the
// viewport edge, so the caret never sits flush against the border.
const revealContentMargin = 8

// reveal scrolls the least amount needed to bring the content-space main-axis
// span [lo, hi] within the viewport (leaving revealContentMargin at the edge).
// It no-ops when the span is already visible or there is nothing to scroll.
func (r *scrollReveal) reveal(lo, hi float32) {
	if r.s.vp.box == nil {
		return
	}
	ext := r.s.mainExtent()
	if ext <= 0 {
		return
	}
	off := r.s.offset
	target := off
	switch {
	case lo < off+revealContentMargin:
		target = lo - revealContentMargin
	case hi > off+ext-revealContentMargin:
		target = hi - ext + revealContentMargin
		// Don't scroll so far that the top of the rect leaves the viewport —
		// for a rect taller than the viewport, prefer showing its top.
		if target > lo-revealContentMargin {
			target = lo - revealContentMargin
		}
	default:
		return
	}
	if m := r.s.vp.box.MaxOffset(); target > m {
		target = m
	}
	if target < 0 {
		target = 0
	}
	if target != off {
		r.s.jumpTo(target)
	}
}

type viewportRef struct{ box *layoutbox.Viewport }

func (s *scrollState) Init(ctx Ctx) {
	s.ctx = ctx
	s.vp = &viewportRef{}
	s.fling.s = s
	s.dragVel.s = s
	s.endArmed = true
	ctx.AddTicker(&s.fling)
	ctx.AddTicker(&s.dragVel)
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
	s.overSpring.s = s
	ctx.AddTicker(&s.overSpring)
	s.barTick.s = s
	ctx.AddTicker(&s.barTick)
	s.reveal = &scrollReveal{s: s}
	if c := s.W().Controller; c != nil {
		c.s = s
	}
}

func (s *scrollState) Dispose() {
	s.ctx.RemoveTicker(&s.fling)
	s.ctx.RemoveTicker(&s.dragVel)
	s.ctx.RemoveTicker(s.glide)
	s.ctx.RemoveTicker(s.snap)
	s.ctx.RemoveTicker(&s.spin)
	s.ctx.RemoveTicker(&s.overSpring)
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

// barDrag maps a drag along the scrollbar into a content offset.
//
// The thumb travels (extent - thumbLen) while the content travels maxOffset, so
// a pixel of thumb is worth maxOffset/track pixels of content — which is why
// dragging a short thumb through a long document moves so fast, and why the
// ratio must come from the measured track rather than from the viewport size.
func (s *scrollState) barDrag(delta float32) {
	if s.vp.box == nil || s.barTrackLen <= 0 {
		return
	}
	maxOff := s.vp.box.MaxOffset()
	if maxOff <= 0 {
		return
	}
	s.fling.active = false
	next := barOffsetFor(s.offset, delta, maxOff, s.barTrackLen)
	s.SetState(func() { s.offset = next })
	s.barFade = 1 // keep the bar visible for the whole drag
	s.reportOffset()
}

// barOffsetFor is the drag arithmetic on its own, so it can be tested without
// a live widget state.
func barOffsetFor(offset, delta, maxOff, track float32) float32 {
	if track <= 0 || maxOff <= 0 {
		return offset
	}
	next := offset + delta*(maxOff/track)
	if next < 0 {
		return 0
	}
	if next > maxOff {
		return maxOff
	}
	return next
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

// pushOffset moves the offset by delta (offset space: positive scrolls further
// down/right) clamped to [0, MaxOffset], and returns the unapplied overflow
// (want − clamped): >0 past the trailing edge, <0 past the leading edge, 0 when
// the whole delta landed inside the range.
func (s *scrollState) pushOffset(delta float32) float32 {
	if delta == 0 {
		return 0
	}
	var overflow float32
	s.SetState(func() {
		want := s.offset + delta
		hi := float32(math.MaxFloat32)
		if s.vp.box != nil {
			hi = s.vp.box.MaxOffset()
		}
		switch {
		case want < 0:
			overflow, s.offset = want, 0
		case want > hi:
			overflow, s.offset = want-hi, hi
		default:
			s.offset = want
		}
	})
	s.reportOffset()
	return overflow
}

// scrollBy moves the content by delta (positive scrolls further down/right)
// and reports whether the offset hit an edge.
func (s *scrollState) scrollBy(delta float32) bool {
	return s.pushOffset(delta) != 0
}

// mainExtent is the viewport's length along the scroll axis.
func (s *scrollState) mainExtent() float32 {
	if s.vp.box == nil {
		return 0
	}
	sz := s.vp.box.Size()
	if s.W().Axis == layout.Horizontal {
		return sz.W
	}
	return sz.H
}

// rubberBand maps a raw past-the-edge drag distance to the displayed elastic
// displacement, matching the asymptotic resistance of an NSScrollView: the
// first pixels move nearly 1:1 (·rubberC), then the band stiffens and tends
// toward the viewport extent so it never runs away. Sign is preserved.
func rubberBand(dist, extent float32) float32 {
	if extent <= 0 {
		return dist
	}
	sign := float32(1)
	if dist < 0 {
		sign, dist = -1, -dist
	}
	return sign * (1 - 1/(dist*rubberC/extent+1)) * extent
}

// inverseRubberBand recovers the raw drag distance behind a displayed elastic
// displacement — used when a gesture resumes mid-spring so continued dragging
// picks up from the right place on the curve.
func inverseRubberBand(disp, extent float32) float32 {
	if extent <= 0 {
		return disp
	}
	sign := float32(1)
	if disp < 0 {
		sign, disp = -1, -disp
	}
	if disp >= extent {
		disp = extent * 0.999
	}
	return sign * extent * disp / (rubberC * (extent - disp))
}

// setOverscroll updates the displayed elastic displacement and repaints.
func (s *scrollState) setOverscroll(v float32) {
	s.SetState(func() { s.overscroll = v })
}

// scrollFinger consumes a finger/fling delta dm (down/right positive) into the
// offset, honoring Reverse, and returns the leftover finger delta that ran off
// an edge (same sign convention as dm, so a positive leftover is a pull past
// the leading edge).
func (s *scrollState) scrollFinger(dm float32) float32 {
	od := -dm
	if s.W().Reverse {
		od = dm
	}
	over := s.pushOffset(od)
	if s.W().Reverse {
		return over
	}
	return -over
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

// dragSampler turns finger movement into a release velocity, once per frame.
//
// It exists because the two halves of a fling used to read different clocks.
// Velocity was measured between pointer events with time.Now(), while the
// fling that consumes it integrates against the frame clock — so the number
// handed over was in units the thing receiving it did not share.
//
// That mattered three ways. Pointer events arrive in bursts and can be
// coalesced, so wall-clock gaps between them are erratic; the old code guarded
// with `dt > 0.001 && dt < 0.1` and *discarded* every sample outside it, which
// on a fast flick delivering several moves inside a millisecond threw away
// exactly the samples that carried the speed. A held-still finger kept its last
// velocity, because with no events there was nothing to decay it — so stopping
// dead and lifting still flung. And nothing could be tested: synthetic events
// happen microseconds apart, every sample failed the guard, and a scripted
// flick produced no fling at all.
//
// Accumulating movement and dividing by the frame's own dt fixes all three. A
// frame with no movement contributes a zero sample and decays the estimate,
// which is what a finger coming to rest should do.
type dragSampler struct {
	s      *scrollState
	accum  float32 // movement since the last frame
	active bool
	// hist is the last few frames' (dt, movement), newest last, for the
	// release estimate. Six frames is 50ms at 120Hz and 100ms at 60Hz — more
	// than the window ever reads.
	hist [6]frameSample
	n    int
}

type frameSample struct {
	dt   float32
	move float32
}

// record keeps one frame's movement in the history ring.
func (d *dragSampler) record(dt, move float32) {
	if d.n < len(d.hist) {
		d.hist[d.n] = frameSample{dt, move}
		d.n++
		return
	}
	copy(d.hist[:], d.hist[1:])
	d.hist[len(d.hist)-1] = frameSample{dt, move}
}

// releaseVelocity is the speed the fling starts from: total movement over the
// last velocityWindow seconds of frames, divided by their span.
//
// Measured, not designed. An EMA — the previous estimator — weights the
// finger's final samples most, and a finger slows in its last frame or two
// before it lifts, so the hand-off ran low: 16% under UIKit's on a recorded
// iPhone flick at the best time constant, 50% under on a Mac. Both platforms
// start momentum from the speed *before* that slowdown — the iOS recording's
// momentum began at the mean of the two frames preceding its 84px and 2px
// tail; Android's began at the finger's steady speed with no under-read at
// all. A trailing window of a few frames is that number. Swept through the
// real replay against both recordings: +6% on the iOS start, −2% on both
// platforms' momentum distance, where the EMA was −14% and −5%.
//
// The window is time, not frames, so it averages four frames at 120Hz and two
// at 60Hz and the same flick flings the same on both. The release frame's own
// partial movement is never ticked (see flinger.fresh), which is what keeps
// the lift-off sample out without a filter for it.
func (d *dragSampler) releaseVelocity() float32 {
	if d.n == 0 {
		return d.s.velocity
	}
	frames := d.hist[:d.n]
	var move, span float32
	for i := len(frames) - 1; i >= 0 && span < float32(velocityWindow); i-- {
		move += frames[i].move
		span += frames[i].dt
	}
	if span <= 0 {
		return d.s.velocity
	}
	return move / span
}

func (d *dragSampler) begin()              { d.accum, d.active, d.n = 0, true, 0 }
func (d *dragSampler) moved(delta float32) { d.accum += delta }

// end stops sampling and replaces the running estimate with the release
// estimate, which is what the fling reads.
func (d *dragSampler) end() {
	d.active = false
	d.s.velocity = d.releaseVelocity()
}

// velocityTau is how quickly the estimate follows the finger, in seconds.
//
// Expressed as a time constant rather than a per-sample weight because a
// per-sample weight is a different filter at every refresh rate: 0.3 a frame
// converges in half the wall time on a 120Hz display as on a 60Hz one, so the
// same flick would fling differently on two phones.
//
// This is the running estimate while the finger is down; the value the fling
// starts from is dragSampler.releaseVelocity, a trailing window, which
// replaced the EMA for that job after the harness measured it handing off
// 16% low on iOS. 20ms keeps the running estimate close to the finger without
// getting jittery. A var so tests can sweep it (see export_test.go).
var velocityTau = 0.020

// velocityWindow is how much trailing movement the release estimate averages
// over, in seconds. See dragSampler.releaseVelocity. A var so the harness can
// sweep it (export_test.go).
var velocityWindow = 0.032

func (d *dragSampler) Tick(dt float64) bool {
	if !d.active || dt <= 0 {
		return false
	}
	inst := d.accum / float32(dt)
	d.record(float32(dt), d.accum)
	d.accum = 0
	a := float32(1 - math.Exp(-dt/velocityTau))
	d.s.velocity += (inst - d.s.velocity) * a
	return true
}

// flinger decelerates the scroll after release: exponential friction, handing
// off to an elastic bounce when it runs into an edge with speed to spare.
type flinger struct {
	s      *scrollState
	v      float32 // px/s (finger space: down/right positive)
	active bool
	// physics is the resolved platform curve, captured at start.
	physics shell.ScrollPhysics
	// The spline model is position-driven rather than velocity-driven: it
	// knows its whole duration and distance at release and reads position off
	// a table. t is elapsed time, moved is how far it has already applied,
	// and dir carries the sign the table does not.
	t, dur, dist, moved float64
	dir                 float32
	// fresh marks the tick that shares a frame with the release. That frame
	// already moved the content by the finger's last delta, so the first
	// momentum tick must not move it again: doing so integrated one frame's
	// travel twice, and every release began with a ~1.9× jump — measured by
	// tools/uitrace as 38px in a frame whose neighbours moved 20 and 17.
	// Native scroll views start momentum from the release instant and only
	// integrate forward. The velocity still decays through the skipped tick,
	// because the time passed.
	fresh bool
}

// start begins momentum at v px/s under the owner's platform physics.
func (f *flinger) start(v float32) {
	f.v, f.active, f.fresh = v, true, true
	f.physics = f.s.ctx.el.owner.ScrollPhysics.Resolved()
	if f.physics.Model == shell.FlingSpline {
		f.dir = 1
		if v < 0 {
			f.dir = -1
		}
		f.dur, f.dist = splineFling(float64(v), f.physics.Friction)
		f.t, f.moved = 0, 0
	}
}

// decay applies dt of exponential slowdown.
func (f *flinger) decay(dt float64) {
	f.v *= float32(math.Exp(-dt / f.physics.Tau))
}

const (
	// The decay itself lives in shell.ScrollPhysics now — τ 0.5s for the
	// exponential (UIKit's 0.998/ms, measured at 0.518 on an iOS 26
	// Simulator), Android's spline for the rest — chosen by the platform.
	flingMinStart = 80 // px/s needed to start a fling
	flingMinSpeed = 20 // px/s considered at rest

	// Rubber-band + bounce tuning (see rubberBand / overspring).
	rubberC       = 0.55 // elastic resistance factor (macOS ≈ 0.55)
	overStiffness = 220  // spring constant for the settle-to-zero bounce
	maxBounceVel  = 2000 // px/s cap on velocity fed into an edge bounce
)

func (f *flinger) Tick(dt float64) bool {
	if !f.active {
		return false
	}
	if f.fresh {
		f.fresh = false
		if f.physics.Model == shell.FlingExponential {
			f.decay(dt)
		}
		return true
	}
	if f.physics.Model == shell.FlingSpline {
		return f.tickSpline(dt)
	}
	if f.s.scrollFinger(f.v*float32(dt)) != 0 {
		// Hit an edge with velocity left over: bounce instead of dead-stopping.
		f.active = false
		f.s.startSpringBack(f.v)
		return false
	}
	f.decay(dt)
	if f.v < flingMinSpeed && f.v > -flingMinSpeed {
		f.active = false
	}
	return f.active
}

// tickSpline advances Android's position-driven fling by dt.
func (f *flinger) tickSpline(dt float64) bool {
	f.t += dt
	if f.dur <= 0 {
		f.active = false
		return false
	}
	target := splineAt(f.t/f.dur) * f.dist
	step := target - f.moved
	f.moved = target
	// The instantaneous velocity, for a bounce if this step hits an edge.
	f.v = float32(step/dt) * f.dir
	if f.s.scrollFinger(float32(step)*f.dir) != 0 {
		f.active = false
		f.s.startSpringBack(f.v)
		return false
	}
	if f.t >= f.dur {
		f.active = false
	}
	return f.active
}

// overspring is a critically-damped spring that settles the elastic overscroll
// back to zero. It is seeded with an initial velocity — the leftover fling
// speed at an edge, or the finger speed at drag release — so the band shoots
// out and eases back the way a native scroll view rebounds.
type overspring struct {
	s      *scrollState
	v      float32 // px/s of the overscroll displacement
	active bool
}

func (o *overspring) Tick(dt float64) bool {
	if !o.active {
		return false
	}
	x := o.s.overscroll
	const k = overStiffness
	d := 2 * float32(math.Sqrt(k)) // critical damping: c = 2√k
	// Semi-implicit Euler (velocity first) stays stable at 60Hz.
	o.v += (-k*x - d*o.v) * float32(dt)
	x += o.v * float32(dt)
	if x > -0.4 && x < 0.4 && o.v > -6 && o.v < 6 {
		x, o.v, o.active = 0, 0, false
	}
	o.s.setOverscroll(x)
	o.s.ctx.Invalidate()
	return o.active
}

// startSpringBack settles the elastic overscroll back to zero, seeded with
// velocity v (capped): the leftover fling speed for an edge bounce, or the
// finger's release velocity for a pulled-then-let-go rebound.
func (s *scrollState) startSpringBack(v float32) {
	if v > maxBounceVel {
		v = maxBounceVel
	} else if v < -maxBounceVel {
		v = -maxBounceVel
	}
	s.overRaw = 0
	s.overSpring.v = v
	s.overSpring.active = true
	s.ctx.Invalidate()
}

// Pull-to-refresh tuning (logical px).
const (
	refreshTrigger = 64 // pull distance that fires OnRefresh on release
	refreshRest    = 56 // indicator height held while a refresh runs
)

// dragMain applies one main-axis finger delta (down = positive). Dragging
// within the content scrolls; dragging past either edge feeds a rubber-banded
// overscroll (the top edge, with OnRefresh set, also drives pull-to-refresh).
func (s *scrollState) dragMain(dm float32) {
	// While a refresh is running the indicator band is held open; just scroll
	// the content underneath and leave the band alone.
	if s.refreshing {
		s.scrollFinger(dm)
		return
	}
	ext := s.mainExtent()
	// Already past an edge: advance the raw distance and re-map elastically.
	// Crossing back through the edge spends the remainder on real scrolling.
	if s.overRaw != 0 {
		raw := s.overRaw + dm
		if (s.overRaw > 0) != (raw > 0) && raw != 0 {
			s.overRaw = 0
			s.setOverscroll(0)
			if left := s.scrollFinger(raw); left != 0 {
				s.overRaw = left
				s.setOverscroll(rubberBand(left, ext))
			}
			return
		}
		s.overRaw = raw
		s.setOverscroll(rubberBand(raw, ext))
		return
	}
	// Inside the content: scroll, routing any edge overrun into overscroll.
	if left := s.scrollFinger(dm); left != 0 {
		s.overRaw = left
		s.setOverscroll(rubberBand(left, ext))
	}
}

// animateOverscrollTo springs the overscroll to a resting value.
func (s *scrollState) animateOverscrollTo(to float32) {
	s.snapFrom, s.snapTo = s.overscroll, to
	s.snap.Jump(0)
	s.snap.Forward()
	s.ctx.Invalidate()
}

// releaseOverscroll decides, on drag release, whether a top-edge pull was far
// enough to trigger a refresh (hold the indicator) or the band should spring
// back. Bottom-edge (and non-refresh) overscroll always springs back.
func (s *scrollState) releaseOverscroll() {
	s.overRaw = 0
	if s.overscroll == 0 {
		return
	}
	if s.W().OnRefresh != nil && !s.refreshing && s.overscroll >= refreshTrigger {
		s.refreshing = true
		s.animateOverscrollTo(refreshRest)
		s.W().OnRefresh()
		return
	}
	if !s.refreshing {
		s.startSpringBack(s.velocity)
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
		Gestures: Gestures{
			DragAxis: dragAxis, // so a cross-axis swipe (Dismissible) can nest
			OnScroll: func(d geom.Pt) {
				s.fling.active = false
				if s.overSpring.active || s.overscroll != 0 {
					s.overSpring.active = false
					s.overRaw = 0
					s.setOverscroll(0)
				}
				s.contentDelta(s.mainDelta(d))
			},
			OnPress: func(geom.Pt) {
				s.fling.active = false      // grab stops the fling
				s.overSpring.active = false // ...and any in-flight bounce
				// Grabbing mid-bounce: re-derive the raw drag distance from the
				// displayed band so continued dragging resumes on the curve.
				if s.overscroll != 0 && !s.refreshing {
					s.overRaw = inverseRubberBand(s.overscroll, s.mainExtent())
				}
				s.velocity = 0
				s.dragVel.begin()
			},
			OnDrag: func(_, d geom.Pt) {
				delta := s.mainDelta(d)
				s.dragMain(delta)
				// Only accumulate. How fast that is depends on how much time
				// passed, and the frame clock is what knows.
				s.dragVel.moved(delta)
			},
			OnRelease: func() {
				s.dragVel.end()
				s.releaseOverscroll()
				// Only fling from inside the content; a released overscroll is
				// already handled by its spring-back.
				if s.overscroll == 0 && (s.velocity > flingMinStart || s.velocity < -flingMinStart) {
					s.fling.start(s.velocity)
					ctx.Invalidate()
				}
			},
		},
		Child: viewport{Axis: w.Axis, Offset: s.offset, Lead: s.overscroll, Reverse: w.Reverse, Ref: s.vp,
			// revealAnchor captures the content origin each paint; Provide exposes
			// the reveal service to descendants (TextField caret-into-view).
			Child: Provide[*scrollReveal]{Value: s.reveal, Child: revealAnchor{reveal: s.reveal, child: w.Child}}},
	}
	// Overlay the fading scroll indicator, and the pull-to-refresh spinner
	// when enabled, above the content.
	layers := []Widget{inner, scrollbar{s: s}}
	if t := scrollbarThumb(s); t != nil {
		layers = append(layers, t)
	}
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

// revealAnchor is a single-child render widget whose box records the scroll
// content's absolute origin at the start of each paint, so a descendant can map
// its caret into content space for scrollReveal.
type revealAnchor struct {
	reveal *scrollReveal
	child  Widget
}

func (w revealAnchor) createBox(Ctx) layout.Box            { return &revealAnchorBox{reveal: w.reveal} }
func (w revealAnchor) updateBox(_ Ctx, b layout.Box)       { b.(*revealAnchorBox).reveal = w.reveal }
func (w revealAnchor) childWidgets() []Widget              { return []Widget{w.child} }
func (w revealAnchor) soleChild() Widget                   { return w.child }
func (w revealAnchor) attach(b layout.Box, k []layout.Box) { b.(*revealAnchorBox).child = first(k) }

type revealAnchorBox struct {
	layoutbox.Base
	reveal *scrollReveal
	child  layout.Box
}

func (b *revealAnchorBox) Layout(cs layout.Constraints) geom.Size {
	var sz geom.Size
	if b.child != nil {
		sz = b.child.Layout(cs)
	} else {
		sz = cs.Constrain(geom.Size{})
	}
	return b.Done(cs, sz)
}

func (b *revealAnchorBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reveal != nil {
		b.reveal.beginContent(at)
	}
	if b.child != nil {
		b.child.Paint(c, at)
	}
}

func (b *revealAnchorBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.child != nil {
		b.child.AddHits(p, hits)
	}
}

func (b *revealAnchorBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.child != nil {
		visit(b.child, geom.Pt{})
	}
}

// scrollbar overlays a thin, fading position indicator on the scroll's
// trailing edge — so scrollable regions are discoverable. It reads the live
// scroll state each paint and never intercepts input.
// scrollbarThumb places a drag target over the painted thumb.
//
// It is a separate, thumb-sized widget rather than input on the bar itself: the
// scrollbar box fills the whole scroll area so it can paint at the edge, and an
// Interactive that size would swallow every tap meant for the content beneath.
// Aligning a small child positions it without any new layout machinery — the
// fraction is exactly the thumb's travel.
//
// Returns nil when there is nothing to drag, which keeps the target out of the
// tree entirely rather than leaving an invisible one to catch stray presses.
func scrollbarThumb(s *scrollState) Widget {
	if s.vp.box == nil || s.barThumbLen <= 0 || s.barFade <= 0 {
		return nil
	}
	if s.vp.box.MaxOffset() <= 0 {
		return nil
	}
	frac := float32(0)
	if s.barTrackLen > 0 {
		frac = s.offset / s.vp.box.MaxOffset()
		if frac < 0 {
			frac = 0
		}
		if frac > 1 {
			frac = 1
		}
	}
	horiz := s.W().Axis == layout.Horizontal

	// A little wider than the 4pt bar it covers: a 4-pixel drag target is a
	// miss on a mouse and unusable on touch.
	const grab = 16
	size := Sized{W: grab, H: s.barThumbLen}
	align := Align{X: 1, Y: frac}
	axis := DragVertical
	if horiz {
		size = Sized{W: s.barThumbLen, H: grab}
		align = Align{X: frac, Y: 1}
		axis = DragHorizontal
	}

	size.Child = Interactive{
		Gestures: Gestures{
			DragAxis: axis,
			OnPress:  func(geom.Pt) { s.barDragging, s.barDragStart = true, s.offset },
			OnDrag: func(_, d geom.Pt) {
				if horiz {
					s.barDrag(d.X)
				} else {
					s.barDrag(d.Y)
				}
			},
			OnRelease:  func() { s.barDragging = false },
			OnPressEnd: func() { s.barDragging = false },
		},
	}
	align.Child = size
	return align
}

type scrollbar struct{ s *scrollState }

func (b scrollbar) createBox(Ctx) layout.Box        { return &scrollbarBox{s: b.s} }
func (b scrollbar) updateBox(_ Ctx, box layout.Box) { box.(*scrollbarBox).s = b.s }
func (b scrollbar) childWidgets() []Widget          { return nil }
func (b scrollbar) attach(layout.Box, []layout.Box) {}

type scrollbarBox struct {
	layoutbox.Base
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
	s.barThumbLen, s.barTrackLen = thumbLen, extent-thumbLen
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
	layoutbox.Base
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
	for i := range refreshSpokes {
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

func (v viewport) createBox(Ctx) layout.Box { return &layoutbox.Viewport{} }
func (v viewport) updateBox(_ Ctx, b layout.Box) {
	vb := b.(*layoutbox.Viewport)
	vb.Axis, vb.Offset, vb.Lead, vb.Reverse = v.Axis, v.Offset, v.Lead, v.Reverse
	if v.Ref != nil {
		v.Ref.box = vb
	}
}
func (v viewport) childWidgets() []Widget { return []Widget{v.Child} }
func (v viewport) soleChild() Widget      { return v.Child }
func (v viewport) attach(b layout.Box, kids []layout.Box) {
	b.(*layoutbox.Viewport).Child = first(kids)
}
