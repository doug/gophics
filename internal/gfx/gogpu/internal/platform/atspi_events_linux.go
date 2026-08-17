//go:build linux

// Change notification.
//
// Serving the tree on demand is only half of it. A screen reader reads a node
// once and then relies on being told when it changes; without events it will
// happily announce a checkbox as unchecked long after the user checked it, and
// will not notice a node appear at all. AT-SPI carries those notifications as
// D-Bus signals on org.a11y.atspi.Event.Object.
//
// Every event has the same shape, (siiv(so)): a detail string, two integers
// whose meaning depends on the event, a variant payload, and a reference to the
// application the event came from. What varies is the member name and how those
// slots are filled.
//
// Events are emitted by diffing the published tree against the previous one
// rather than on every republish. gophics rebuilds and republishes whenever the
// widget tree changes, which for an animating UI is every frame; broadcasting
// an unchanged tree at 60Hz would drown the bus and make a screen reader
// unusable — the opposite of the point.

package platform

import "strconv"

const (
	ifaceEventObject = "org.a11y.atspi.Event.Object"
	// Window events live on their own interface. Orca watches these to decide
	// which window it is reading.
	ifaceEventWindow = "org.a11y.atspi.Event.Window"
)

// AT-SPI live-region politeness, used by announcements.
const (
	livePolite    int32 = 1
	liveAssertive int32 = 2
)

// emit broadcasts one AT-SPI event.
//
// source is the object the event is about; appRef identifies this application
// and goes in the trailing (so). The variant payload is written by writeAny,
// which most events leave as an empty string.
func (b *atspiBridge) emit(sourcePath, member, detail string, d1, d2 int32, writeAny func(*msgBuf)) {
	b.emitOn(ifaceEventObject, sourcePath, member, detail, d1, d2, writeAny)
}

// emitOn is emit with an explicit interface, for the window events that do not
// live under Event.Object.
func (b *atspiBridge) emitOn(iface, sourcePath, member, detail string, d1, d2 int32, writeAny func(*msgBuf)) {
	body := newMsgBuf(0)
	body.str(detail)
	body.i32(d1)
	body.i32(d2)
	if writeAny != nil {
		writeAny(body)
	} else {
		body.variant("s", func() { body.str("") })
	}
	body.ref(b.root())

	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.conn.serial++
	raw := dbusEncodeSignal(b.conn.serial, sourcePath, iface, member, "siiv(so)", body.data)
	b.conn.rw.Write(raw)
}

// stateNames maps the node flags that can change at run time onto the state
// names AT-SPI uses in a state-changed event's detail slot. Bounds and labels
// are not states and are reported separately.
var trackedStates = []struct {
	name string
	of   func(A11yNode) bool
}{
	{"checked", func(n A11yNode) bool { return n.Checked }},
	{"focused", func(n A11yNode) bool { return n.Focused }},
	{"selected", func(n A11yNode) bool { return n.Selected }},
	// Disabled is inverted twice over: it clears two states, and both are
	// reported, because a reader may be tracking either.
	{"enabled", func(n A11yNode) bool { return !n.Disabled }},
	{"sensitive", func(n A11yNode) bool { return !n.Disabled }},
}

// emitDiff broadcasts what changed between two published trees.
//
// Structural change is reported coarsely: one children-changed on the parent
// rather than a precise minimal edit script. A screen reader responds to it by
// re-reading that subtree, which is cheap here — the tree is already in memory
// and answering a query costs a D-Bus round trip, not a layout pass.
func (b *atspiBridge) emitDiff(old, cur *a11yTree) {
	if old == nil {
		// First publication: the application gained its whole tree, and its
		// window became the active one. Orca needs the second event to start
		// reading; without it the frame is listed but never entered.
		b.emit(atspiRootPath, "ChildrenChanged", "add", 0, 0, nil)
		for _, id := range cur.roots {
			b.emitOn(ifaceEventWindow, atspiNodePrefix+strconv.Itoa(id), "Activate", "", 0, 0, nil)
		}
		return
	}

	for _, n := range cur.nodes {
		prev, existed := old.byID[n.ID]
		if !existed {
			b.emitChildrenChanged(cur, n, "add")
			continue
		}
		if prev.Label != n.Label || prev.Value != n.Value {
			// accessible-name is the property name AT-SPI uses; the reader
			// re-reads Name rather than trusting anything in the payload.
			b.emit(atspiNodePrefix+strconv.Itoa(n.ID), "PropertyChange",
				"accessible-name", 0, 0, nil)
		}
		for _, st := range trackedStates {
			was, now := st.of(prev), st.of(n)
			if was == now {
				continue
			}
			var d1 int32
			if now {
				d1 = 1
			}
			b.emit(atspiNodePrefix+strconv.Itoa(n.ID), "StateChanged", st.name, d1, 0, nil)
		}
	}

	for _, n := range old.nodes {
		if _, still := cur.byID[n.ID]; !still {
			b.emitChildrenChanged(old, n, "remove")
		}
	}
}

// emitChildrenChanged reports that a node appeared under or vanished from its
// parent. detail1 carries the child's index, which is what a reader uses to
// splice its own model without re-reading every sibling.
func (b *atspiBridge) emitChildrenChanged(t *a11yTree, n A11yNode, detail string) {
	parentPath := atspiRootPath
	if n.ParentID != -1 {
		if _, ok := t.byID[n.ParentID]; ok {
			parentPath = atspiNodePrefix + strconv.Itoa(n.ParentID)
		}
	}
	idx := t.indexInParent(n.ID)
	child := b.nodeRef(n.ID)
	b.emit(parentPath, "ChildrenChanged", detail, idx, 0, func(body *msgBuf) {
		body.variant("(so)", func() { body.ref(child) })
	})
}

// announce speaks a transient message — the live-region idiom, for things a
// user needs told but that are not part of the tree ("5 results").
//
// AT-SPI delivers these as an ordinary object event, which is why this could
// not exist until events did.
func (b *atspiBridge) announce(message string, assertive bool) {
	politeness := livePolite
	if assertive {
		politeness = liveAssertive
	}
	b.emit(atspiRootPath, "Announcement", "", politeness, 0, func(body *msgBuf) {
		body.variant("s", func() { body.str(message) })
	})
}
