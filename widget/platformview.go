package widget

import (
	"errors"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
)

// PlatformView embeds a real native view — a web view, a map, a video surface
// — in the widget tree, at a rect the layout decides.
//
// It is the widget face of shell.PlatformViews, and reading that capability's
// doc comment is not optional before using this: it explains the model and the
// two things about a platform view that are permanently impossible. Both show
// up here as surprises if they are not expected:
//
//   - The view is composited by the OS above everything gophics draws. A
//     widget painted "over" a PlatformView — a sheet, a dialog, a tooltip, a
//     drag ghost, the scrim behind a modal — is painted *under* the native
//     view and is invisible. Nothing in this widget can fix that; move the
//     overlay out from over the view, or close the view while the overlay is
//     up.
//   - The view never appears in a snapshot. apptest goldens and
//     Bridge.Snapshot rasterise on the CPU with no OS compositor involved, so
//     a golden image of a screen containing a PlatformView shows the
//     Placeholder (or whatever is behind the hole) where the view is. That is
//     not a bug to file. Give the view a Placeholder so the hole is something
//     deliberate.
//
// What this widget adds over calling the capability by hand is the two things
// that are tedious and leak when done by hand: it owns the handle for its
// element's lifetime and closes it on unmount, and it pushes bounds from the
// paint pass, so the view follows a scroll, a resize, an animation or a
// re-layout without the app computing a rect.
//
// Sizing follows Canvas: by default the view fills the space its parent offers;
// set W and/or H to request a fixed extent on that axis.
//
// Clipping is a rect intersection with the ancestor clip, and only that. A
// PlatformView inside a Scroll is clipped to the viewport and hidden entirely
// once scrolled out of it, keeping its state — a half-filled form, a playing
// video — for when it comes back. What a rect cannot express it does not get:
// rounded corners, a non-rectangular clip, opacity and transforms do not apply
// to the native view. Under a Transform the view is positioned as if
// untransformed, which is wrong rather than approximate; do not put one under
// a rotating or scaling ancestor. (A platform *can* mask a native view, per
// OS, and that is a possible future — it is not promised here.)
//
// A nil capability is the normal case, not an error. ctx.PlatformViews() is nil
// wherever the platform cannot composite a foreign view, and on mobile it is
// nil until the host app registers a PlatformViewHost — which most apps never
// do. When it is nil, or when Kind is a kind the host cannot build, this widget
// lays out and paints exactly as Placeholder does and opens nothing. That is
// why Placeholder should be a real fallback (a static map image, a thumbnail
// with a play button that calls OpenURL) rather than a spinner: on most builds
// it is what the user sees.
type PlatformView struct {
	// Kind names the view to build — shell.PlatformViewWebView,
	// shell.PlatformViewMap, shell.PlatformViewVideo, or any kind the host
	// registered. A kind the host does not offer opens nothing and reports
	// OnError once.
	Kind string
	// Params configures the view and is interpreted by the host for that kind:
	// a URL for a web view, "lat,lon,zoom" for a map. Changing Kind or Params
	// closes the current view and opens a new one, so a URL that changes per
	// keystroke should be debounced by the caller.
	Params string

	// W, H request a fixed extent; zero fills the space the parent offers
	// (collapsing when the parent does not bound that axis), as Canvas does.
	W, H float32

	// Placeholder is painted in the view's rect by the widget tree. It shows
	// before the view is ready, wherever the capability is nil, and in every
	// snapshot and golden image. See the type's doc comment.
	Placeholder Widget

	// Controller, when set, exposes Send and the view's readiness to the app.
	// It is adopted on mount and released on unmount, like ScrollController.
	Controller *PlatformViewController

	// OnReady fires once when the view has content to show; OnError fires
	// instead if it could not be created or loaded, including when the host
	// does not offer Kind. Exactly one of the two fires per opened view, on
	// the UI goroutine.
	OnReady func()
	OnError func(err error)
	// OnMessage carries a message from the view — a navigation finishing, a
	// marker tapped, a video ending. Its format is the host's business for
	// that kind.
	OnMessage func(msg string)
}

// ErrPlatformViewKind is reported to OnError when the running build cannot
// build the requested Kind — either because ctx.PlatformViews() offers other
// kinds, or (via the handle being nil) because the shell refused it. It is
// distinguishable from a load failure so an app can fall back permanently
// rather than retrying.
var ErrPlatformViewKind = errors.New("widget: this build cannot embed a platform view of that kind")

// PlatformViewController reaches a live PlatformView from outside its widget:
// it sends messages to the view and reports whether there is one. Create it
// once (in a State, not in Build) and hand the same pointer to the widget.
//
// Every method is safe before mount, after unmount and on a build where the
// capability is nil — Send drops the message and Ready reports false. That is
// deliberate: on most builds there is no view, and an app should not have to
// branch on it to send an optional "pause".
type PlatformViewController struct {
	handle shell.PlatformViewHandle
	ready  bool
}

// Send passes msg to the view, or drops it when there is no view.
func (c *PlatformViewController) Send(msg string) {
	if c.handle != nil {
		c.handle.Send(msg)
	}
}

// Ready reports whether a view is open and has content. False covers all of
// "not mounted", "still loading", "failed" and "this build has no platform
// views", which a caller cannot act on differently anyway — use OnError to
// tell a failure from an absence.
func (c *PlatformViewController) Ready() bool { return c.ready }

func (p PlatformView) CreateState() State { return &platformViewState{} }

type platformViewState struct {
	StateBase[PlatformView]

	views  shell.PlatformViews
	handle shell.PlatformViewHandle
	ctl    *PlatformViewController

	// post delivers the one error this widget raises itself. The shell's own
	// callbacks are already marshalled by the generated posted wrapper, but
	// ErrPlatformViewKind is discovered here — possibly inside paint, since
	// that is where the rect arrives — and app code must not run there.
	post func(func())

	// kind/params are what the open handle was created with, not what the
	// widget currently says: comparing them is how a changed URL becomes a
	// close-and-reopen rather than a silently stale view.
	kind, params string

	// rect is the last clipped rect the box reported, in the coordinates the
	// shell expects. haveRect distinguishes "offscreen" from "never painted",
	// which matters because the view is opened on the first rect and an empty
	// first rect must not count as one.
	rect     geom.Rect
	haveRect bool

	// failed latches a kind the host cannot build, so OnError fires once
	// rather than on every frame that re-tries the open.
	failed bool
}

func (s *platformViewState) Build(ctx Ctx) Widget {
	// Re-read the capability every build. It is nil during Init and on the
	// first build (see State), and the runner marks the tree dirty when the
	// shell arrives — so a widget that decided once would decide "no platform
	// views" forever. See app/present.go's wireWindow.
	s.views = ctx.PlatformViews()
	s.post = ctx.Post()

	w := s.W()
	if s.ctl != w.Controller {
		if s.ctl != nil {
			s.ctl.handle, s.ctl.ready = nil, false
		}
		s.ctl = w.Controller
		s.publish()
	}
	// A changed Kind or Params is a different view. The condition covers the
	// failed case as well as the open one: without that, a widget whose Kind
	// the host could not build would keep the latch forever and never try the
	// kind the app switched to.
	if (s.handle != nil || s.failed) && (s.kind != w.Kind || s.params != w.Params) {
		s.close()
	}
	// Opening here as well as from the paint report is what makes a late
	// capability work: the rect arrives at paint, the capability arrives at
	// the build after the shell is wired, and either can be second. open is
	// idempotent, so running it from both is cheaper than coordinating them.
	s.open()

	return platformViewBox{
		w: w.W, h: w.H, child: w.Placeholder,
		onRect: s.setRect,
	}
}

func (s *platformViewState) Dispose() {
	// The whole reason to prefer this widget over the capability: a handle
	// that outlives its element leaves a native view on screen, composited
	// over whatever replaced it, with nothing left holding a reference to
	// close it.
	s.close()
	if s.ctl != nil {
		s.ctl.handle, s.ctl.ready = nil, false
	}
}

// setRect is called from the box's paint with the view's rect already clipped
// to its ancestors, or an empty rect when it is clipped away entirely. It is
// called only when the rect changes: a scene is painted far more often than it
// moves, and SetBounds crosses to the platform.
func (s *platformViewState) setRect(r geom.Rect) {
	s.rect, s.haveRect = r, true
	if s.handle != nil {
		s.handle.SetBounds(r)
		return
	}
	s.open()
}

// open creates the view if everything it needs is present, and does nothing
// otherwise. Everything it needs: a capability, a non-empty rect (a view with
// no room, or scrolled out of view before it was ever shown, is not worth
// creating), a kind the host offers, and no previous failure for this kind.
func (s *platformViewState) open() {
	if s.handle != nil || s.failed || s.views == nil || !s.haveRect || s.rect.IsEmpty() {
		return
	}
	w := s.W()
	if w.Kind == "" {
		return
	}
	// Record what this attempt is for before anything can fail, so a later
	// Build that changes Kind or Params can tell it is asking for something
	// different and clear the latch.
	s.kind, s.params = w.Kind, w.Params
	if !s.offers(w.Kind) {
		s.failed = true
		s.reportKindError()
		return
	}
	h := s.views.Open(w.Kind, w.Params, s.rect, shell.PlatformViewHandlers{
		// The callbacks read s.W() at delivery rather than capturing the
		// widget: a rebuild between Open and OnReady would otherwise deliver
		// to a closure the app has already replaced.
		OnReady: func() {
			if s.ctl != nil {
				s.ctl.ready = true
			}
			if cb := s.W().OnReady; cb != nil {
				cb()
			}
		},
		OnError: func(err error) {
			if s.ctl != nil {
				s.ctl.ready = false
			}
			if cb := s.W().OnError; cb != nil {
				cb(err)
			}
		},
		OnMessage: func(msg string) {
			if cb := s.W().OnMessage; cb != nil {
				cb(msg)
			}
		},
	})
	if h == nil {
		// A shell may refuse a kind it did not list; treat it exactly as an
		// unoffered kind rather than leaving a nil handle to guard against.
		s.failed = true
		s.reportKindError()
		return
	}
	s.handle = h
	s.publish()
}

// reportKindError delivers the one error this widget raises itself, through
// Post rather than inline. open can run from paint — that is where the rect
// arrives — and app code must not run inside paint. Without a runner (a bare
// Owner in a unit test) it is delivered directly, the way LayoutObserver
// handles the same case.
func (s *platformViewState) reportKindError() {
	cb := s.W().OnError
	if cb == nil {
		return
	}
	if s.post == nil {
		cb(ErrPlatformViewKind)
		return
	}
	s.post(func() { cb(ErrPlatformViewKind) })
}

func (s *platformViewState) offers(kind string) bool {
	for _, k := range s.views.Kinds() {
		if k == kind {
			return true
		}
	}
	return false
}

func (s *platformViewState) close() {
	if s.handle != nil {
		s.handle.Close()
		s.handle = nil
	}
	s.kind, s.params, s.failed = "", "", false
	s.publish()
}

func (s *platformViewState) publish() {
	if s.ctl == nil {
		return
	}
	s.ctl.handle = s.handle
	if s.handle == nil {
		s.ctl.ready = false
	}
}

// platformViewBox is the hole: it lays out and paints the placeholder, and
// reports where the native view should go. It is a separate internal widget
// rather than logic in the State because the rect is only known at paint —
// layout learns sizes, paint learns origins — which is the same reason
// LayoutObserver is shaped this way.
type platformViewBox struct {
	w, h   float32
	child  Widget
	onRect func(geom.Rect)
}

func (p platformViewBox) createBox(Ctx) layout.Box { return &pvBox{} }
func (p platformViewBox) updateBox(_ Ctx, b layout.Box) {
	pb := b.(*pvBox)
	pb.w, pb.h, pb.onRect = p.w, p.h, p.onRect
}
func (p platformViewBox) childWidgets() []Widget { return []Widget{p.child} }
func (p platformViewBox) soleChild() Widget      { return p.child }
func (p platformViewBox) attach(b layout.Box, kids []layout.Box) {
	b.(*pvBox).Child = first(kids)
}

type pvBox struct {
	w, h   float32
	onRect func(geom.Rect)
	Child  layout.Box
	size   geom.Size

	reported     geom.Rect
	haveReported bool
}

// Layout sizes like Canvas: a zero dimension fills the available space when
// the parent bounds it, so a full-bleed map is the default rather than
// something to spell out.
func (b *pvBox) Layout(cs layout.Constraints) geom.Size {
	w, h := b.w, b.h
	if w == 0 && cs.BoundedW() {
		w = cs.Max.W
	}
	if h == 0 && cs.BoundedH() {
		h = cs.Max.H
	}
	b.size = cs.Constrain(geom.Size{W: w, H: h})
	if b.Child != nil {
		b.Child.Layout(layout.Tight(b.size))
	}
	return b.size
}

func (b *pvBox) Size() geom.Size { return b.size }

// Paint draws the placeholder and reports the view's rect, clipped to whatever
// the ancestors clipped to. The intersection is the whole clipping contract:
// a rect is all an OS-composited view can be given.
func (b *pvBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
	rect := geom.Rect{Min: at, Max: at.Add(b.size.Pt())}.Intersect(c.ClipBounds())
	if rect.IsEmpty() {
		// Normalise every way of being invisible to one value, so the
		// change-detection below does not push a sequence of differently
		// empty rects across the platform boundary while a view scrolls past.
		rect = geom.Rect{}
	}
	if b.onRect == nil || (b.haveReported && rect == b.reported) {
		return
	}
	b.reported, b.haveReported = rect, true
	// Delivered inline, not posted. SetBounds has to land in the same frame
	// the layout moved: a posted update arrives a frame late, and a native
	// view one frame behind the widget tree visibly slides during a scroll.
	// The callback only calls into the shell and never into app code, so
	// there is nothing here that may not run inside paint.
	b.onRect(rect)
}

// InkBounds reports Unbounded so a viewport never culls this box.
//
// Culling means Paint does not run, and Paint is the only thing that moves or
// hides the native view — a culled PlatformView would be left sitting at its
// last position, composited over whatever is there now. Declining to be culled
// costs one box's paint per frame and is what makes "scrolled out of view
// hides it" true rather than nearly true.
func (b *pvBox) InkBounds() geom.Rect { return geom.Unbounded }

// AddHits claims the rect. The native view is above the surface and takes the
// touch before gophics ever sees it, so in practice these hits only arrive
// where the view is absent (a nil capability, a kind the host lacks, or a
// still-loading view on a platform whose host does not swallow touches) —
// which is exactly when the placeholder should be tappable.
func (b *pvBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if p.X < 0 || p.Y < 0 || p.X >= b.size.W || p.Y >= b.size.H {
		return
	}
	if b.Child != nil {
		b.Child.AddHits(p, hits)
	}
	*hits = append(*hits, layout.Hit{Box: b, Pos: p})
}

func (b *pvBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}
