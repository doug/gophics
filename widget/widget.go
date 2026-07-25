// Package widget implements gossamer's declarative layer: immutable widget
// values describing the UI, reconciled into a retained element tree that owns
// layout boxes. It is Flutter's widgets/ analog (PLAN.md M3, §4).
//
// The three widget kinds (mirroring Flutter, expressed as Go interfaces):
//
//   - Stateless: Build(ctx) describes content from configuration alone.
//   - Stateful: CreateState() yields mutable State whose Build runs on
//     SetState; state survives reconciliation while the widget type and key
//     match.
//   - render widgets (Text, Padding, Row, ...): bridge to layout.Box render
//     objects. In v1 these are defined only inside this package; the public
//     extension API for custom render widgets comes with the M3 ADRs.
//
// Widgets are compared by concrete type and key during reconciliation.
// Attach a key with WithKey to preserve element/state identity across list
// reorders.
package widget

import (
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
)

// Widget is an immutable description of part of the UI. A widget must be a
// Stateless, a Stateful, or one of this package's render widgets.
type Widget = any

// Stateless widgets build their content from configuration alone.
type Stateless interface {
	Build(ctx Ctx) Widget
}

// Stateful widgets create mutable State.
type Stateful interface {
	CreateState() State
}

// State is the mutable companion of a Stateful widget.
type State interface {
	Build(ctx Ctx) Widget
	internalState
}

// Initer is implemented by State that wants a lifecycle hook after mount.
type Initer interface{ Init(ctx Ctx) }

// Disposer is implemented by State that wants a hook before unmount.
type Disposer interface{ Dispose() }

// StateBase provides the framework plumbing for a State implementation.
// Embed it with the widget's concrete type:
//
//	type counterState struct {
//	    widget.StateBase[Counter]
//	    n int
//	}
//
// W() returns the current widget configuration; SetState applies a mutation
// and schedules a rebuild.
type StateBase[W Widget] struct {
	el *element
	w  W
}

// W returns the current widget configuration.
func (s *StateBase[W]) W() W { return s.w }

// SetState runs fn and marks this widget's subtree for rebuild before the
// next frame. Call it only from the UI goroutine.
func (s *StateBase[W]) SetState(fn func()) {
	if fn != nil {
		fn()
	}
	s.el.markDirty()
}

func (s *StateBase[W]) setElement(el *element) { s.el = el }
func (s *StateBase[W]) setWidget(w Widget)     { s.w = w.(W) }

type internalState interface {
	setElement(el *element)
	setWidget(w Widget)
}

// Ctx is the build context: a widget's view of its place in the tree and of
// app services.
type Ctx struct {
	el *element
}

// Painter returns the app's painter (fonts, text measurement).
func (c Ctx) Painter() *paint.Painter { return c.el.owner.Painter }

// Invalidate requests a new frame.
func (c Ctx) Invalidate() { c.el.owner.requestFrame() }

// Clipboard is the platform clipboard surface available to widgets.
type Clipboard interface {
	ClipboardRead() (string, error)
	ClipboardWrite(text string) error
}

// Clipboard returns the platform clipboard, or nil before the app runner
// provides one.
func (c Ctx) Clipboard() Clipboard { return c.el.owner.Clipboard }

// Post schedules fn onto the UI goroutine before the next build. Use it to
// deliver results from background goroutines (network fetches, file IO):
//
//	go func() {
//	    stories := fetch()
//	    post(func() { s.SetState(func() { s.stories = stories }) })
//	}()
//
// Capture the func from Ctx during Build/Init; it is safe to call from any
// goroutine (unlike SetState directly).
func (c Ctx) Post() func(fn func()) { return c.el.owner.Post }

// AddTicker registers per-frame animation work (see Owner.AddTicker).
func (c Ctx) AddTicker(t Ticker) { c.el.owner.AddTicker(t) }

// RemoveTicker unregisters a ticker; call from Dispose.
func (c Ctx) RemoveTicker(t Ticker) { c.el.owner.RemoveTicker(t) }

// WithKey attaches a reconciliation key to its child. Keys preserve element
// and state identity when children move within a list.
type WithKey struct {
	Key   any
	Child Widget
}

func (w WithKey) Build(Ctx) Widget { return w.Child }

// Keyed is implemented by widgets carrying a reconciliation key.
type Keyed interface{ WidgetKey() any }

func (w WithKey) WidgetKey() any { return w.Key }

func keyOf(w Widget) any {
	if k, ok := w.(Keyed); ok {
		return k.WidgetKey()
	}
	return nil
}

// Handler bundles the interaction callbacks of Interactive widgets.
// All callbacks are optional. Positions are in the widget's local
// coordinates.
type Handler struct {
	OnTap   func()
	OnEnter func()
	OnExit  func()
	// OnPress fires on pointer-down over this widget with the local
	// position (before tap/drag disambiguation).
	OnPress func(pos geom.Pt)
	// OnDrag receives pointer movement while pressed on this widget. Once a
	// drag exceeds the tap slop, a pending tap is cancelled.
	OnDrag func(pos, delta geom.Pt)
	// OnRelease fires on pointer-up after this widget received the press
	// (regardless of drag distance) — e.g. to start fling deceleration.
	OnRelease func()
	// OnScroll receives wheel/trackpad deltas while the pointer is over
	// this widget.
	OnScroll func(delta geom.Pt)
	// OnText and OnKey receive keyboard input while focused. A widget with
	// either becomes focusable: it gains focus when tapped (or when mounted
	// while nothing has focus) and OnFocus reports transitions.
	OnText func(s string)
	OnKey  func(k shell.Key)
	// OnComposition receives IME preedit events while focused.
	OnComposition func(c shell.Composition)
	OnFocus       func(focused bool)
}

func (h *Handler) focusable() bool { return h.OnText != nil || h.OnKey != nil }
