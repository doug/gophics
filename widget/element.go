package widget

import (
	"fmt"
	"reflect"
	"sort"

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

	root  *element
	dirty []*element
}

func (o *Owner) requestFrame() {
	if o.RequestFrame != nil {
		o.RequestFrame()
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
		el.child = o.updateChild(el, nil, el.state.Build(el.ctx()))
	case Stateless:
		el.child = o.updateChild(el, nil, w.Build(el.ctx()))
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
		el.child = el.owner.updateChild(el, el.child, el.state.Build(el.ctx()))
	case Stateless:
		el.child = el.owner.updateChild(el, el.child, w.Build(el.ctx()))
	case renderWidget:
		w.updateBox(el.ctx(), el.box)
		el.reconcileRenderChildren(w)
	}
	el.dirty = false
}

// rebuild re-runs Build for a dirty composite element.
func (el *element) rebuild() {
	el.dirty = false
	switch w := el.widget.(type) {
	case Stateful:
		el.child = el.owner.updateChild(el, el.child, el.state.Build(el.ctx()))
	case Stateless:
		el.child = el.owner.updateChild(el, el.child, w.Build(el.ctx()))
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
