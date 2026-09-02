package widget

import (
	"context"
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sort"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/input"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// renderWidget is the internal bridge from widgets to layout boxes.
// Intentionally unexported: only this package defines render widgets —
// see the extension policy in the package doc (doc.go).
type renderWidget interface {
	createBox(ctx Ctx) layout.Box
	updateBox(ctx Ctx, box layout.Box)
	childWidgets() []Widget
	attach(box layout.Box, kids []layout.Box)
}

// singleChildWidget is an optional refinement of renderWidget for the very
// common wrapper shape (Padding, Decorated, Sized, ...) that holds exactly
// one child. widgetsOf uses it to read that child without going through
// childWidgets(), which would box it into a freshly allocated []Widget every
// frame just to hold one element.
type singleChildWidget interface {
	renderWidget
	soleChild() Widget
}

// Owner owns an element tree: the build list, root, and app services.
// It is Flutter's BuildOwner analog. All methods run on the UI goroutine.
type Owner struct {
	Painter *paint.Painter
	// RequestFrame is called when state changes require a new frame.
	RequestFrame func()
	// KeyboardTarget is the current Gestures receiving OnText/OnKey
	// (pre-M4 focus model; see Gestures).
	KeyboardTarget *Gestures
	// Clipboard is the platform clipboard, set by the app runner.
	//
	// A field rather than a generated capability, on purpose. Capabilities are
	// optional integrations a platform may lack, gated by nil and the coverage
	// test; ClipboardRead/Write are methods on the base shell.Window contract,
	// mandatory on every shell, and the compiler enforces that more strongly
	// than any test could. They are also synchronous, so the Posted wrapper
	// machinery — the other thing capabilities buy — has nothing to wrap. The
	// same reasoning covers OpenURL below, whose inbound counterpart (Links)
	// is a capability precisely because launch URLs genuinely do not exist on
	// every platform.
	Clipboard Clipboard
	// Post schedules fn onto the UI goroutine (set by the app runner).
	// The only safe way to touch widget state from other goroutines.
	Post func(fn func())
	// OpenURL opens a URL in the system browser (set by the app runner).
	OpenURL func(url string) error
	// DarkMode reflects the platform color-scheme preference (kept fresh by
	// the app runner; a change rebuilds the whole tree).
	DarkMode bool
	// ReduceMotion reflects the platform "reduce motion" accessibility
	// preference (set by the app runner; default false). Non-essential
	// animations — like the text caret blink — should go solid when set.
	ReduceMotion bool
	// OnBuildPanic observes recovered Build panics (default logs with a
	// stack trace). The panicking subtree renders a BuildError instead.
	OnBuildPanic func(recovered any)
	// SafeInsets are platform-obstructed edges (status bars, notches,
	// on-screen keyboards) in logical pixels, set by the runner; apps pad
	// content by them (Ctx.SafeInsets).
	SafeInsets geom.Insets
	// KeyboardInset is the on-screen keyboard's height in logical pixels, 0 when
	// hidden (Ctx.KeyboardInset).
	KeyboardInset float32
	// Capabilities are the optional platform capabilities (Camera, Haptic,
	// FilePicker, …) a Window may expose. The app runner publishes each one the
	// window implements via wireCapabilities. Fields are promoted, so Ctx.<Cap>()
	// reads owner.<Cap>. This struct and the accessors are GENERATED from the
	// shell.<X>Window interfaces — see capabilities_gen.go, gen.go, and
	// internal/capgen/README.md. Do not add capability fields here by hand.
	Capabilities
	// Input is per-frame poll-style input state for games (held keys, pointer),
	// fed by the app runner and read via Ctx.Input().
	Input *input.State

	root    *element
	dirty   []*element
	tickers []Ticker
}

// Ticker is per-frame animation work. Tick reports whether the ticker is
// still active; inactive tickers stay registered but cost one call per
// animated frame. anim.Controller implements this.
type Ticker interface {
	Tick(dt float64) bool
}

// AddTicker registers t to be advanced each frame while any animation runs.
func (o *Owner) AddTicker(t Ticker) {
	o.tickers = append(o.tickers, t)
	o.requestFrame()
}

// RemoveTicker unregisters t (call from State.Dispose).
func (o *Owner) RemoveTicker(t Ticker) {
	for i, x := range o.tickers {
		if x == t {
			o.tickers = append(o.tickers[:i], o.tickers[i+1:]...)
			return
		}
	}
}

// TickerCount reports how many tickers are currently registered.
//
// It exists to make one specific leak testable. A State that calls
// ctx.AddTicker in Init and has no matching RemoveTicker in Dispose leaves its
// controller registered after the widget is gone: it is ticked on every
// animated frame forever, and it keeps requesting frames, so a list that
// scrolls tappable rows in and out grows this slice without bound and pins the
// frame loop awake. Nothing about that is visible in the output — the UI looks
// right while the cost climbs.
//
// Mount a widget, unmount it, and assert this returns to the count you started
// with. TestTickersAreReleasedOnUnmount does exactly that for the catalog, and
// the same three lines work for a widget defined outside this repo.
func (o *Owner) TickerCount() int { return len(o.tickers) }

// TickersActive reports whether any registered ticker is mid-animation,
// without advancing anything.
//
// The frame pipeline ticks animations before it builds, so an animation that a
// Build starts — which is the normal way a control reacts to its own state
// changing — is invisible to the tick that already ran. On a device with no
// hover events nothing then asks for another frame, and the animation sits at
// its start value while the rest of the UI shows the new state: a switch whose
// dependent content updates but whose knob never slides. Asking again after
// the build is what closes that gap.
//
// Tickers that do not report their state are assumed idle; anim.Controller,
// which is what animates the widget catalog, reports it.
func (o *Owner) TickersActive() bool {
	for _, t := range o.tickers {
		if r, ok := t.(interface{ Running() bool }); ok && r.Running() {
			return true
		}
	}
	return false
}

// TickAll advances all tickers and reports whether any is still running.
func (o *Owner) TickAll(dt float64) bool {
	active := false
	for _, t := range o.tickers {
		if t.Tick(dt) {
			active = true
		}
	}
	return active
}

func (o *Owner) requestFrame() {
	if o.RequestFrame != nil {
		o.RequestFrame()
	}
}

// RequestFrameThreadSafe requests a frame from any goroutine. Shell
// Invalidate implementations must be callable off the UI goroutine.
func (o *Owner) RequestFrameThreadSafe() { o.requestFrame() }

// RebuildAll marks the whole tree dirty (theme/color-scheme changes).
func (o *Owner) RebuildAll() {
	if o.root != nil {
		o.root.markDirty()
	}
}

// SetRoot mounts (or reconciles to) the given root widget.
func (o *Owner) SetRoot(w Widget) {
	o.root = o.updateChild(nil, o.root, w)
}

// RootBox returns the root render box, after builds are flushed.
func (o *Owner) RootBox() layout.Box {
	o.FlushBuilds()
	if o.root == nil {
		return nil
	}
	return o.root.renderBox()
}

// NeedsBuild reports whether any element is awaiting a rebuild — e.g. a
// LayoutBuilder that captured new constraints during layout and must rebuild
// its child before the frame is presented.
func (o *Owner) NeedsBuild() bool { return len(o.dirty) > 0 }

// FlushBuilds rebuilds all dirty elements, parents before children.
func (o *Owner) FlushBuilds() {
	for len(o.dirty) > 0 {
		batch := o.dirty
		o.dirty = nil
		sort.Slice(batch, func(i, j int) bool { return batch[i].depth < batch[j].depth })
		for _, el := range batch {
			if el.dirty && el.mounted {
				el.rebuild()
			}
		}
	}
}

// element is a retained node in the tree: the reconciliation unit pairing a
// widget with its state and/or render box.
type element struct {
	owner   *Owner
	parent  *element
	depth   int
	widget  Widget
	state   State        // non-nil for stateful
	box     layout.Box   // non-nil for render widgets
	child   *element     // composite (stateless/stateful) child
	kids    []*element   // render-widget children
	kidsBuf []*element   // previous frame's kids array, reused by reconcilePositional
	boxBuf  []layout.Box // reused scratch buffer for attachKids
	dirty   bool
	mounted bool

	// lifeCtx is this element's lifetime as a context, created on first use by
	// lifetime() and cancelled by unmount. Children derive from their parent,
	// so cancelling an element cancels everything below it.
	lifeCtx    context.Context
	lifeCancel context.CancelFunc

	// oneKid is scratch space widgetsOf reuses to hand a singleChildWidget's
	// child to the reconciler as a []Widget without allocating one.
	oneKid [1]Widget
}

// widgetsOf returns w's children as a slice. For the single-child case (the
// overwhelming majority of render widgets — Padding, Decorated, Sized, ...)
// it reuses el.oneKid instead of calling w.childWidgets(), which would
// allocate a fresh one-element slice every frame for every such node.
func (el *element) widgetsOf(w renderWidget) []Widget {
	if sc, ok := w.(singleChildWidget); ok {
		el.oneKid[0] = sc.soleChild()
		return el.oneKid[:]
	}
	return w.childWidgets()
}

func (el *element) ctx() Ctx { return Ctx{el: el} }

// lifetime returns a context cancelled when this element leaves the tree. It
// derives from the parent element's lifetime, so unmounting a subtree cancels
// the work started anywhere inside it; the root derives from Background.
//
// Created on demand: an element whose widget never starts background work
// never allocates one. An element that has already unmounted gets a context
// that is cancelled before it is returned, so late callers cannot start work
// that nothing will ever stop.
func (el *element) lifetime() context.Context {
	if el.lifeCtx != nil {
		return el.lifeCtx
	}
	parent := context.Background()
	if el.parent != nil {
		parent = el.parent.lifetime()
	}
	el.lifeCtx, el.lifeCancel = context.WithCancel(parent)
	if !el.mounted {
		el.lifeCancel()
	}
	return el.lifeCtx
}

func (el *element) markDirty() {
	if el.dirty || !el.mounted {
		return
	}
	el.dirty = true
	el.owner.dirty = append(el.owner.dirty, el)
	el.owner.requestFrame()
}

// renderBox returns the nearest render box at or below el.
func (el *element) renderBox() layout.Box {
	if el.box != nil {
		return el.box
	}
	if el.child != nil {
		return el.child.renderBox()
	}
	return nil
}

func canUpdate(old Widget, new Widget) bool {
	return reflect.TypeOf(old) == reflect.TypeOf(new) && keyOf(old) == keyOf(new)
}

// safeBuild runs a widget's Build, converting a panic into an inline error
// widget so one failing subtree cannot take down the app.
// The panic is reported through OnBuildPanic (default: log).
func (o *Owner) safeBuild(build func() Widget) (w Widget) {
	defer func() {
		if r := recover(); r != nil {
			if o.OnBuildPanic != nil {
				o.OnBuildPanic(r)
			} else {
				log.Printf("widget: recovered build panic: %v\n%s", r, debug.Stack())
			}
			w = BuildError{Message: fmt.Sprint(r)}
		}
	}()
	return build()
}

// BuildError is the inline substitute rendered for a subtree whose Build
// panicked.
type BuildError struct{ Message string }

func (e BuildError) Build(Ctx) Widget {
	return Decorated{
		Color: paint.Color{R: 0.75, G: 0.15, B: 0.15, A: 1}, Radius: 4,
		Child: Padding{All: 8, Child: Text{
			Value: "build failed: " + e.Message, Size: 12, Wrap: true,
			Color: paint.RGB(1, 1, 1),
		}},
	}
}

// updateChild is the reconciler entry point: reuse, replace, or remove.
func (o *Owner) updateChild(parent *element, old *element, w Widget) *element {
	if w == nil {
		if old != nil {
			old.unmount()
		}
		return nil
	}
	if old != nil && canUpdate(old.widget, w) {
		old.update(w)
		return old
	}
	if old != nil {
		old.unmount()
	}
	return o.mount(parent, w)
}

func (o *Owner) mount(parent *element, w Widget) *element {
	w = o.asWidget(w)
	el := &element{owner: o, parent: parent, widget: w, mounted: true}
	if parent != nil {
		el.depth = parent.depth + 1
	}
	switch w := w.(type) {
	case Stateful:
		el.state = w.CreateState()
		el.state.setElement(el)
		el.state.setWidget(w)
		if init, ok := el.state.(Initer); ok {
			init.Init(el.ctx())
		}
		el.child = o.updateChild(el, nil, o.safeBuild(func() Widget { return el.state.Build(el.ctx()) }))
	case Stateless:
		el.child = o.updateChild(el, nil, o.safeBuild(func() Widget { return w.Build(el.ctx()) }))
	case renderWidget:
		el.box = w.createBox(el.ctx())
		w.updateBox(el.ctx(), el.box)
		el.mountRenderChildren(w)
	}
	return el
}

// asWidget returns w, or a BuildError standing in for a value that is not a
// widget at all.
//
// Widget is any, so a stray value — Padding{Child: "hello"}, a slice of the
// wrong element type — type-checks and only fails here. It used to fail by
// panicking, and that panic landed badly: safeBuild has already returned by
// the time mount runs, so it escaped to the frame-level recover, which drops
// the frame and logs at most once every five seconds. Every subsequent frame
// did the same. The app did not crash and did not show an error; it simply
// stopped changing, which reads as a hang and sends you looking anywhere but
// at the widget you mistyped.
//
// Substituting is the policy safeBuild already states for a subtree that
// fails: one bad subtree must not take down the app. The rest of the tree
// keeps rendering and the broken part says what it is, in place.
func (o *Owner) asWidget(w Widget) Widget {
	switch w.(type) {
	case Stateful, Stateless, renderWidget:
		return w
	}
	msg := fmt.Sprintf("%T implements none of Stateless, Stateful, or a render widget", w)
	if o.OnBuildPanic != nil {
		o.OnBuildPanic(msg)
	} else {
		log.Printf("widget: %s", msg)
	}
	return BuildError{Message: msg}
}

func (el *element) mountRenderChildren(w renderWidget) {
	el.markBoxChainDirty()
	widgets := el.widgetsOf(w)
	el.kids = el.kids[:0]
	for _, cw := range widgets {
		if cw == nil {
			continue
		}
		el.kids = append(el.kids, el.owner.mount(el, cw))
	}
	el.attachKids(w)
}

// attachKids hands each attach implementation the children's render boxes.
// boxBuf is retained on el and reused frame over frame: every attach
// implementation we have copies out of the slice it's given (into its own
// box's Children, or by extracting a single child), so nothing outlives this
// call that would alias a mutated buffer.
//
// That is an invariant nothing enforces, and violating it fails quietly — a
// retained slice reads the *next* frame's boxes, so a tree lays out against
// children it no longer has, with no error. scrubAttachBuf exists to make it
// loud instead; see its comment.
func (el *element) attachKids(w renderWidget) {
	el.boxBuf = el.boxBuf[:0]
	for _, k := range el.kids {
		el.boxBuf = append(el.boxBuf, k.renderBox())
	}
	w.attach(el.box, el.boxBuf)
	if scrubAttachBuf {
		// The contract is that attach has finished with the slice by now, so
		// nulling it changes nothing — unless someone kept it, in which case
		// the next layout walks into nils and the guard test says so. One
		// branch per attach, off in every normal run.
		for i := range el.boxBuf {
			el.boxBuf[i] = nil
		}
	}
}

// scrubAttachBuf turns on the check described in attachKids. Tests set it; it
// is never on in a running app.
var scrubAttachBuf bool

// update reconciles el (already type/key-matched) to the new widget value.
func (el *element) update(w Widget) {
	el.widget = w
	switch w := w.(type) {
	case Stateful:
		el.state.setWidget(w)
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return el.state.Build(el.ctx()) }))
	case Stateless:
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return w.Build(el.ctx()) }))
	case renderWidget:
		w.updateBox(el.ctx(), el.box)
		el.reconcileRenderChildren(w)
		// Updated configuration may change layout: invalidate this box and
		// every ancestor box's skip-cache. Untouched sibling subtrees stay
		// clean and skip their layout entirely.
		el.markBoxChainDirty()
	}
	el.dirty = false
}

func (el *element) markBoxChainDirty() {
	for e := el; e != nil; e = e.parent {
		if e.box != nil {
			layoutbox.MarkDirty(e.box)
		}
	}
}

// rebuild re-runs Build for a dirty composite element (SetState path).
// If the rebuild swaps the element's root render box — e.g. a Stateful
// widget returning a placeholder one frame and content the next — the
// nearest ancestor render box still holds the stale child pointer, so we
// re-attach it. Without this, loading→content transitions never repaint.
func (el *element) rebuild() {
	el.dirty = false
	before := el.renderBox()
	switch w := el.widget.(type) {
	case Stateful:
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return el.state.Build(el.ctx()) }))
	case Stateless:
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return w.Build(el.ctx()) }))
	}
	if el.renderBox() != before {
		el.reattachHost()
	}
}

// reattachHost re-runs attach on the nearest ancestor render element so its
// child-box slots pick up this composite's (possibly new) render box.
func (el *element) reattachHost() {
	for a := el.parent; a != nil; a = a.parent {
		if a.box == nil {
			continue
		}
		if rw, ok := a.widget.(renderWidget); ok {
			a.attachKids(rw)
		}
		a.markBoxChainDirty()
		return
	}
}

// dupKeyWarned tracks reconciliation keys already reported as duplicated, so
// the diagnostic fires once per key value instead of every frame. UI-goroutine
// only, like the rest of reconciliation.
var dupKeyWarned = map[any]bool{}

// warnDuplicateKey reports two siblings sharing a reconciliation key — a data
// bug in the app (e.g. list items keyed by a non-unique field). The tree stays
// alive (the duplicate is mounted fresh instead of reusing the same element
// twice), but state cannot be preserved for the duplicate.
func warnDuplicateKey(key any) {
	if dupKeyWarned[key] {
		return
	}
	dupKeyWarned[key] = true
	log.Printf("widget: duplicate reconciliation key %#v: two children in the same list share this key; "+
		"the duplicate is mounted as a fresh element and its state is not preserved. "+
		"Give each sibling a unique key.", key)
}

// reconcileRenderChildren matches new child widgets against old elements:
// keyed children match by (type, key) anywhere in the old list; unkeyed
// children match by position when types agree. A simplification of
// Flutter's updateChildren; revisit with the M3 ADRs.
//
// Most child lists never use keys (Column(rows...) with plain widgets), so
// the general algorithm below — which needs two maps to support arbitrary
// key reordering — is skipped in favor of a map-free positional pass whenever
// neither side carries a key. That pass is exactly the general algorithm
// specialized to "no key ever matches, no element is ever marked used out of
// position order": matches only ever happen at the current cursor, so the
// cursor's final value is the whole matched prefix and everything after it is
// stale.
func (el *element) reconcileRenderChildren(w renderWidget) {
	widgets := el.widgetsOf(w)

	// Fast path: nothing in play (old or new) carries a reconciliation key,
	// so matching reduces to a single positional cursor with no need for the
	// byKey/used maps below. This is the overwhelmingly common case (plain
	// unkeyed child lists), and skipping the maps avoids two allocations per
	// render-widget per frame.
	if !anyKeyedElements(el.kids) && !anyKeyedWidgets(widgets) {
		el.reconcilePositional(w, widgets)
		return
	}

	var byKey map[any]*element
	for _, old := range el.kids {
		if k := keyOf(old.widget); k != nil {
			if byKey == nil {
				byKey = map[any]*element{}
			}
			byKey[k] = old
		}
	}

	used := map[*element]bool{}
	newKids := make([]*element, 0, len(widgets))
	var seenKeys map[any]bool // keys already claimed by an earlier new child
	pos := 0                  // position cursor over old unkeyed matching
	for _, cw := range widgets {
		if cw == nil {
			continue
		}
		var match *element
		if k := keyOf(cw); k != nil {
			if seenKeys[k] {
				// A sibling earlier in this list carries the same key. Reusing
				// the same *element twice would double-paint it and corrupt
				// unmount, so the duplicate gets a fresh element (state is not
				// preserved for it) and we say so loudly, once per key.
				warnDuplicateKey(k)
			} else {
				if seenKeys == nil {
					seenKeys = map[any]bool{}
				}
				seenKeys[k] = true
				if old, ok := byKey[k]; ok && canUpdate(old.widget, cw) && !used[old] {
					match = old
				}
			}
		} else {
			// advance cursor past keyed/used elements
			for pos < len(el.kids) && (used[el.kids[pos]] || keyOf(el.kids[pos].widget) != nil) {
				pos++
			}
			if pos < len(el.kids) && canUpdate(el.kids[pos].widget, cw) {
				match = el.kids[pos]
				pos++
			}
		}
		if match != nil {
			used[match] = true
			match.update(cw)
			newKids = append(newKids, match)
		} else {
			newKids = append(newKids, el.owner.mount(el, cw))
		}
	}
	for _, old := range el.kids {
		if !used[old] {
			old.unmount()
		}
	}
	el.kids = newKids
	el.attachKids(w)
}

func anyKeyedElements(kids []*element) bool {
	for _, k := range kids {
		if keyOf(k.widget) != nil {
			return true
		}
	}
	return false
}

func anyKeyedWidgets(widgets []Widget) bool {
	for _, w := range widgets {
		if w != nil && keyOf(w) != nil {
			return true
		}
	}
	return false
}

// reconcilePositional is reconcileRenderChildren without keys anywhere in
// play: every match happens at the current cursor position, so the matched
// set is always the prefix el.kids[:pos] and the rest is stale. No map is
// needed to track "used" or to look children up by key.
//
// It also skips attachKids — which allocates a fresh boxes slice — when
// nothing a parent box cares about actually changed: same count, and every
// matched kid's renderBox() pointer is exactly what it was before update(cw)
// ran. A config-only change (e.g. a color field) mutates the existing box in
// place and keeps its pointer, same as the box-swap check rebuild() already
// does for the SetState path; only a kid whose root box type itself changed,
// or an add/remove, needs the parent's box slice rebuilt.
//
// newKids itself is allocated lazily, only once the shape actually diverges
// from last frame (a mismatch, or fewer live widgets than kids). The common
// steady-state frame — same widgets, same order, only config fields changed —
// matches every kid in place and never allocates: el.kids is already the
// right slice with the right pointers. When the shape does diverge because of
// an insert or type change, newKids is built into el.kidsBuf — the array
// el.kids pointed to two frames ago — instead of a fresh allocation. It is a
// distinct backing array from el.kids for the whole loop (the two are only
// swapped at the end, and only when newKids was actually built from
// kidsBuf), so appending here never clobbers the el.kids reads above,
// including the el.kids[pos:] unmount loop below. A pure shrink needs no
// buffer at all: el.kids[:pos:pos] is already the right slice.
func (el *element) reconcilePositional(w renderWidget, widgets []Widget) {
	var newKids []*element
	usedBuf := false
	pos := 0
	boxesChanged := false
	for _, cw := range widgets {
		if cw == nil {
			continue
		}
		if pos < len(el.kids) && canUpdate(el.kids[pos].widget, cw) {
			kid := el.kids[pos]
			before := kid.renderBox()
			kid.update(cw)
			if kid.renderBox() != before {
				boxesChanged = true
			}
			if newKids != nil {
				newKids = append(newKids, kid)
			}
			pos++
			continue
		}
		if newKids == nil {
			newKids = append(el.kidsBuf[:0], el.kids[:pos]...)
			usedBuf = true
		}
		newKids = append(newKids, el.owner.mount(el, cw))
		boxesChanged = true
	}
	if pos != len(el.kids) {
		for _, old := range el.kids[pos:] {
			old.unmount()
		}
		boxesChanged = true
		if newKids == nil {
			// Pure truncation: no mismatch occurred, so this subslice shares
			// el.kids' own backing array. It must not be swapped into
			// el.kidsBuf below — that would alias el.kids and el.kidsBuf.
			newKids = el.kids[:pos:pos]
		}
	}
	if newKids != nil {
		if usedBuf {
			el.kids, el.kidsBuf = newKids, el.kids
		} else {
			el.kids = newKids
		}
	}
	if boxesChanged {
		el.attachKids(w)
	}
}

func (el *element) unmount() {
	if !el.mounted {
		return
	}
	el.mounted = false
	// Cancel before anything else: Dispose hooks and the child walk below can
	// take a while, and work started by this subtree should stop now rather
	// than run on against a tree it can no longer touch.
	if el.lifeCancel != nil {
		el.lifeCancel()
	}
	// Release keyboard focus if this element's box holds it, so key/text events
	// stop routing to a detached handler and a newly-mounted focusable can take
	// focus (autofocus only fires when KeyboardTarget is nil, and text-capturing
	// would otherwise stay stuck on).
	if el.owner != nil && el.owner.KeyboardTarget != nil {
		if ib, ok := el.box.(*InteractiveBox); ok && el.owner.KeyboardTarget == &ib.Gestures {
			el.owner.KeyboardTarget = nil
			if ib.Gestures.OnFocus != nil {
				ib.Gestures.OnFocus(false)
			}
		}
	}
	if el.state != nil {
		if d, ok := el.state.(Disposer); ok {
			d.Dispose()
		}
	}
	if el.child != nil {
		el.child.unmount()
	}
	for _, k := range el.kids {
		k.unmount()
	}
}
