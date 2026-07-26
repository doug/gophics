package widget

import (
	"fmt"
	"log"
	"reflect"
	"runtime/debug"
	"sort"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
)

// renderWidget is the internal bridge from widgets to layout boxes.
// v1: implemented only by widgets in this package (see package doc).
type renderWidget interface {
	createBox(ctx Ctx) layout.Box
	updateBox(ctx Ctx, box layout.Box)
	childWidgets() []Widget
	attach(box layout.Box, kids []layout.Box)
}

// Owner owns an element tree: the build list, root, and app services.
// It is Flutter's BuildOwner analog. All methods run on the UI goroutine.
type Owner struct {
	Painter *paint.Painter
	// RequestFrame is called when state changes require a new frame.
	RequestFrame func()
	// KeyboardTarget is the current Handler receiving OnText/OnKey
	// (pre-M4 focus model; see Handler).
	KeyboardTarget *Handler
	// Clipboard is the platform clipboard, set by the app runner.
	Clipboard Clipboard
	// Post schedules fn onto the UI goroutine (set by the app runner).
	// The only safe way to touch widget state from other goroutines.
	Post func(fn func())
	// OpenURL opens a URL in the system browser (set by the app runner).
	OpenURL func(url string) error
	// DarkMode reflects the platform color-scheme preference (kept fresh by
	// the app runner; a change rebuilds the whole tree).
	DarkMode bool
	// OnBuildPanic observes recovered Build panics (default logs with a
	// stack trace). The panicking subtree renders a BuildError instead.
	OnBuildPanic func(recovered any)
	// SafeInsets are platform-obstructed edges (status bars, notches,
	// on-screen keyboards) in logical pixels, set by the runner; apps pad
	// content by them (Ctx.SafeInsets).
	SafeInsets geom.Insets

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
	dirty   bool
	mounted bool
}

func (el *element) ctx() Ctx { return Ctx{el: el} }

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
// widget so one failing subtree cannot take down the app (PLAN.md §4.7).
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
			S: "build failed: " + e.Message, Size: 12, Wrap: true,
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
	default:
		panic(fmt.Sprintf("widget: %T implements none of Stateless, Stateful, or a render widget", w))
	}
	return el
}

func (el *element) mountRenderChildren(w renderWidget) {
	el.markBoxChainDirty()
	widgets := w.childWidgets()
	el.kids = el.kids[:0]
	for _, cw := range widgets {
		if cw == nil {
			continue
		}
		el.kids = append(el.kids, el.owner.mount(el, cw))
	}
	el.attachKids(w)
}

func (el *element) attachKids(w renderWidget) {
	boxes := make([]layout.Box, len(el.kids))
	for i, k := range el.kids {
		boxes[i] = k.renderBox()
	}
	w.attach(el.box, boxes)
}

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
			layout.MarkDirty(e.box)
		}
	}
}

// rebuild re-runs Build for a dirty composite element.
func (el *element) rebuild() {
	el.dirty = false
	switch w := el.widget.(type) {
	case Stateful:
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return el.state.Build(el.ctx()) }))
	case Stateless:
		el.child = el.owner.updateChild(el, el.child, el.owner.safeBuild(func() Widget { return w.Build(el.ctx()) }))
	}
}

// reconcileRenderChildren matches new child widgets against old elements:
// keyed children match by (type, key) anywhere in the old list; unkeyed
// children match by position when types agree. A simplification of
// Flutter's updateChildren; revisit with the M3 ADRs.
func (el *element) reconcileRenderChildren(w renderWidget) {
	widgets := w.childWidgets()

	byKey := map[any]*element{}
	for _, old := range el.kids {
		if k := keyOf(old.widget); k != nil {
			byKey[k] = old
		}
	}

	used := map[*element]bool{}
	newKids := make([]*element, 0, len(widgets))
	pos := 0 // position cursor over old unkeyed matching
	for _, cw := range widgets {
		if cw == nil {
			continue
		}
		var match *element
		if k := keyOf(cw); k != nil {
			if old, ok := byKey[k]; ok && canUpdate(old.widget, cw) {
				match = old
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

func (el *element) unmount() {
	if !el.mounted {
		return
	}
	el.mounted = false
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
