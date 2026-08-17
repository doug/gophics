//go:build linux

// The AT-SPI server: gophics answering a screen reader's questions.
//
// AT-SPI is a pull model, like AppKit's. The application does not push a tree
// and walk away; it exports an object per accessible node and answers calls —
// GetChildAtIndex, GetRole, GetExtents — whenever the screen reader asks. So
// this owns a connection to the accessibility bus, a goroutine reading it, and
// a dispatch table keyed by (object path, interface, member).
//
// Two objects exist per published node: the application object at
// .../accessible/root, which is what the desktop sees, and one object per
// A11yNode beneath it. The application object is not a gophics node — it is the
// container AT-SPI requires every app to present, and its single child is the
// tree's root.

package platform

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"
)

// AT-SPI interface names.
const (
	ifaceAccessible  = "org.a11y.atspi.Accessible"
	ifaceComponent   = "org.a11y.atspi.Component"
	ifaceAction      = "org.a11y.atspi.Action"
	ifaceApplication = "org.a11y.atspi.Application"
	ifaceCache       = "org.a11y.atspi.Cache"
	ifaceProps       = "org.freedesktop.DBus.Properties"
	ifaceIntrospect  = "org.freedesktop.DBus.Introspectable"

	registryDest = "org.a11y.atspi.Registry"
	registryPath = "/org/a11y/atspi/accessible/root"
	ifaceSocket  = "org.a11y.atspi.Socket"
)

// atspiBridge publishes a tree on the accessibility bus.
type atspiBridge struct {
	conn    *dbusConn
	busName string // our unique name, e.g. ":1.42" — half of every objRef

	writeMu sync.Mutex // serialises writes; replies and signals share the socket

	mu       sync.RWMutex
	tree     *a11yTree
	activate func(id int)
	parent   objRef // the desktop, learned from Embed
	appName  string

	published bool // whether a tree has been published yet

	stop chan struct{}
	once sync.Once
}

// newATSPIBridge connects to the accessibility bus and registers with the
// registry. It returns nil when there is no accessibility bus, which is the
// ordinary case on a machine with no assistive technology configured — the
// caller then publishes nothing and ctx.Accessibility() is correctly nil.
func newATSPIBridge(appName string) *atspiBridge {
	addr, err := a11yBusAddress()
	if err != nil || addr == "" {
		return nil
	}
	raw, err := dbusDialAddr(addr)
	if err != nil {
		return nil
	}
	c := &dbusConn{rw: raw}
	c.rw.SetDeadline(time.Now().Add(5 * time.Second))
	if err := c.auth(); err != nil {
		raw.Close()
		return nil
	}
	name, err := c.hello()
	if err != nil || name == "" {
		raw.Close()
		return nil
	}
	c.rw.SetDeadline(time.Time{})

	b := &atspiBridge{
		conn: c, busName: name, appName: appName,
		tree: buildTree(nil), stop: make(chan struct{}),
	}
	go b.serve()
	b.embed()
	return b
}

// root is this application's object reference.
func (b *atspiBridge) root() objRef { return objRef{Name: b.busName, Path: atspiRootPath} }

// nullRef is AT-SPI's "no such object" — used for the parent of the desktop
// and for lookups that find nothing. It is a real reference to a null path,
// not an error.
func (b *atspiBridge) nullRef() objRef { return objRef{Name: b.busName, Path: atspiNullPath} }

// nodeRef is the reference for one gophics node.
func (b *atspiBridge) nodeRef(id int) objRef {
	return objRef{Name: b.busName, Path: atspiNodePrefix + strconv.Itoa(id)}
}

// embed plugs this application into the desktop tree. The registry answers
// with the object that will be our parent.
func (b *atspiBridge) embed() {
	body := newMsgBuf(0)
	body.ref(b.root())

	b.writeMu.Lock()
	b.conn.serial++
	serial := b.conn.serial
	raw := dbusEncodeMsg(dbusMsgCall, serial, registryDest, registryPath, ifaceSocket, "Embed", "(so)", body.data)
	_, err := b.conn.rw.Write(raw)
	b.writeMu.Unlock()
	if err != nil {
		return
	}
	// The reply is picked up by serve(), which stores the parent. Embed is
	// advisory: a tree still answers queries without it, so a registry that
	// never replies is not fatal.
}

// SetTree replaces the published tree and tells anyone listening what changed.
//
// The diff happens outside the lock: emitting means writing to a socket, and
// holding the tree lock across that would stall every query the screen reader
// makes while we talk to it.
func (b *atspiBridge) SetTree(nodes []A11yNode, activate func(id int)) {
	next := buildTree(nodes)

	b.mu.Lock()
	prev := b.tree
	b.tree = next
	b.activate = activate
	first := !b.published
	b.published = true
	b.mu.Unlock()

	if first {
		b.emitDiff(nil, next)
		return
	}
	b.emitDiff(prev, next)
}

// Announce speaks a transient message through the screen reader.
func (b *atspiBridge) Announce(message string, assertive bool) {
	if message == "" {
		return
	}
	b.announce(message, assertive)
}

// Close shuts the connection down.
func (b *atspiBridge) Close() {
	b.once.Do(func() {
		close(b.stop)
		b.conn.rw.Close()
	})
}

// serve reads the bus and answers calls until the connection closes.
func (b *atspiBridge) serve() {
	for {
		select {
		case <-b.stop:
			return
		default:
		}
		msg, err := b.conn.readMsg()
		if err != nil {
			return
		}
		switch msg.Type {
		case dbusMsgCall:
			b.handleCall(msg)
		case dbusMsgReturn:
			// The only call we make is Embed; its reply carries our parent.
			d := newMsgDecoder(msg.Body, 0)
			if err := d.alignTo(8); err == nil {
				if name, err := d.readStr(); err == nil {
					if path, err := d.readStr(); err == nil && path != "" {
						b.mu.Lock()
						b.parent = objRef{Name: name, Path: path}
						b.mu.Unlock()
					}
				}
			}
		}
	}
}

// reply writes a METHOD_RETURN for msg.
func (b *atspiBridge) reply(msg *dbusMsg, sig string, body []byte) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.conn.serial++
	raw := dbusEncodeReturn(b.conn.serial, msg.Serial, msg.Sender, sig, body)
	b.conn.rw.Write(raw)
}

// replyErr writes an ERROR for msg.
func (b *atspiBridge) replyErr(msg *dbusMsg, name, text string) {
	b.writeMu.Lock()
	defer b.writeMu.Unlock()
	b.conn.serial++
	raw := dbusEncodeError(b.conn.serial, msg.Serial, msg.Sender, name, text)
	b.conn.rw.Write(raw)
}

// parsePath resolves an object path to a node ID. isRoot marks the application
// object, which is not a gophics node.
func parsePath(path string) (id int, isRoot, ok bool) {
	if path == atspiRootPath {
		return 0, true, true
	}
	if !strings.HasPrefix(path, atspiNodePrefix) {
		return 0, false, false
	}
	n, err := strconv.Atoi(strings.TrimPrefix(path, atspiNodePrefix))
	if err != nil {
		return 0, false, false
	}
	return n, false, true
}

// handleCall routes one method call.
func (b *atspiBridge) handleCall(msg *dbusMsg) {
	// The cache object is separate from the accessible tree.
	if msg.Interface == ifaceCache {
		b.handleCacheCall(msg)
		return
	}
	if msg.Interface == ifaceIntrospect && msg.Member == "Introspect" {
		b.reply(msg, "s", strBody(introspectXML))
		return
	}

	id, isRoot, ok := parsePath(msg.Path)
	if !ok {
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownObject", "no such object: "+msg.Path)
		return
	}

	b.mu.RLock()
	tree, activate, parent := b.tree, b.activate, b.parent
	b.mu.RUnlock()

	var node A11yNode
	if !isRoot {
		n, found := tree.byID[id]
		if !found {
			b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownObject", "stale node: "+msg.Path)
			return
		}
		node = n
	}

	switch msg.Interface {
	case ifaceProps:
		b.handleProps(msg, tree, node, isRoot, parent)
	case ifaceAccessible:
		b.handleAccessible(msg, tree, node, isRoot)
	case ifaceComponent:
		b.handleComponent(msg, tree, node, isRoot)
	case ifaceAction:
		b.handleAction(msg, node, isRoot, activate)
	case ifaceApplication:
		b.handleApplication(msg)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownInterface", "unknown interface: "+msg.Interface)
	}
}

// childRefs lists a node's children, or — for the application object — the
// tree's roots.
func (b *atspiBridge) childRefs(tree *a11yTree, id int, isRoot bool) []objRef {
	var ids []int
	if isRoot {
		ids = tree.roots
	} else {
		ids = tree.children(id)
	}
	refs := make([]objRef, 0, len(ids))
	for _, c := range ids {
		refs = append(refs, b.nodeRef(c))
	}
	return refs
}

// parentRef is the parent of a node: another node, the application object, or
// — for the application itself — the desktop learned from Embed.
func (b *atspiBridge) parentRef(tree *a11yTree, node A11yNode, isRoot bool, desktop objRef) objRef {
	if isRoot {
		if desktop.Path != "" {
			return desktop
		}
		return b.nullRef()
	}
	if node.ParentID == -1 {
		return b.root()
	}
	if _, ok := tree.byID[node.ParentID]; !ok {
		return b.root()
	}
	return b.nodeRef(node.ParentID)
}

func (b *atspiBridge) handleProps(msg *dbusMsg, tree *a11yTree, node A11yNode, isRoot bool, desktop objRef) {
	d := newMsgDecoder(msg.Body, 0)
	iface, err := d.readStr()
	if err != nil {
		b.replyErr(msg, "org.freedesktop.DBus.Error.InvalidArgs", "bad interface arg")
		return
	}
	switch msg.Member {
	case "Get":
		prop, err := d.readStr()
		if err != nil {
			b.replyErr(msg, "org.freedesktop.DBus.Error.InvalidArgs", "bad property arg")
			return
		}
		body := newMsgBuf(0)
		if !b.writeProp(body, iface, prop, tree, node, isRoot, desktop) {
			b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownProperty", prop)
			return
		}
		b.reply(msg, "v", body.data)
	case "GetAll":
		body := newMsgBuf(0)
		lenPos, contentPos := body.arrayStart(8)
		for _, p := range propsFor(iface) {
			body.padTo(8)
			body.str(p)
			if !b.writeProp(body, iface, p, tree, node, isRoot, desktop) {
				// Every name in propsFor is writable; this cannot happen, but
				// a truncated dict is worse than a missing one.
				b.replyErr(msg, "org.freedesktop.DBus.Error.Failed", "property "+p)
				return
			}
		}
		body.arrayEnd(lenPos, contentPos)
		b.reply(msg, "a{sv}", body.data)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
	}
}

// propsFor lists the properties GetAll should return for an interface.
func propsFor(iface string) []string {
	switch iface {
	case ifaceAccessible:
		return []string{"Name", "Description", "Parent", "ChildCount", "Locale", "AccessibleId"}
	case ifaceApplication:
		return []string{"ToolkitName", "Version", "AtspiVersion", "Id"}
	case ifaceAction:
		return []string{"NActions"}
	}
	return nil
}

// writeProp writes one property's value as a variant.
func (b *atspiBridge) writeProp(body *msgBuf, iface, prop string, tree *a11yTree, node A11yNode, isRoot bool, desktop objRef) bool {
	switch iface {
	case ifaceAccessible:
		switch prop {
		case "Name":
			body.variant("s", func() { body.str(b.nameOf(node, isRoot)) })
		case "Description":
			body.variant("s", func() { body.str(descOf(node, isRoot)) })
		case "Parent":
			p := b.parentRef(tree, node, isRoot, desktop)
			body.variant("(so)", func() { body.ref(p) })
		case "ChildCount":
			body.variantI32(int32(len(b.childRefs(tree, node.ID, isRoot))))
		case "Locale":
			body.variant("s", func() { body.str("C") })
		case "AccessibleId":
			if isRoot {
				body.variant("s", func() { body.str("") })
			} else {
				body.variant("s", func() { body.str(strconv.Itoa(node.ID)) })
			}
		default:
			return false
		}
		return true
	case ifaceApplication:
		switch prop {
		case "ToolkitName":
			body.variant("s", func() { body.str("gophics") })
		case "Version":
			body.variant("s", func() { body.str("1.0") })
		case "AtspiVersion":
			body.variant("s", func() { body.str("2.1") })
		case "Id":
			body.variantI32(0)
		default:
			return false
		}
		return true
	case ifaceAction:
		if prop == "NActions" {
			body.variantI32(int32(actionCount(node, isRoot)))
			return true
		}
		return false
	}
	return false
}

func (b *atspiBridge) nameOf(node A11yNode, isRoot bool) string {
	if isRoot {
		return b.appName
	}
	if node.Label != "" {
		return node.Label
	}
	return node.Value
}

// descOf prefers the hint, which is what a hint is for — the longer
// explanation a screen reader reads after the name.
func descOf(node A11yNode, isRoot bool) string {
	if isRoot {
		return ""
	}
	return node.Hint
}

func actionCount(node A11yNode, isRoot bool) int {
	if !isRoot && node.Tappable {
		return 1
	}
	return 0
}

func (b *atspiBridge) handleAccessible(msg *dbusMsg, tree *a11yTree, node A11yNode, isRoot bool) {
	switch msg.Member {
	case "GetChildAtIndex":
		d := newMsgDecoder(msg.Body, 0)
		i, err := d.readI32()
		refs := b.childRefs(tree, node.ID, isRoot)
		body := newMsgBuf(0)
		if err != nil || i < 0 || int(i) >= len(refs) {
			body.ref(b.nullRef())
		} else {
			body.ref(refs[i])
		}
		b.reply(msg, "(so)", body.data)
	case "GetChildren":
		body := newMsgBuf(0)
		body.refArray(b.childRefs(tree, node.ID, isRoot))
		b.reply(msg, "a(so)", body.data)
	case "GetIndexInParent":
		body := newMsgBuf(0)
		if isRoot {
			body.i32(0)
		} else {
			body.i32(tree.indexInParent(node.ID))
		}
		b.reply(msg, "i", body.data)
	case "GetRelationSet":
		// No relations are modelled yet; an empty set is a complete answer.
		body := newMsgBuf(0)
		lenPos, contentPos := body.arrayStart(8)
		body.arrayEnd(lenPos, contentPos)
		b.reply(msg, "a(ua(so))", body.data)
	case "GetRole":
		body := newMsgBuf(0)
		body.u32(b.roleOf(node, isRoot))
		b.reply(msg, "u", body.data)
	case "GetRoleName", "GetLocalizedRoleName":
		body := newMsgBuf(0)
		body.str(atspiRoleName(b.roleOf(node, isRoot)))
		b.reply(msg, "s", body.data)
	case "GetState":
		body := newMsgBuf(0)
		if isRoot {
			var bits [2]uint32
			bits[stateVisible/32] |= 1 << (stateVisible % 32)
			bits[stateShowing/32] |= 1 << (stateShowing % 32)
			bits[stateActive/32] |= 1 << (stateActive % 32)
			bits[stateEnabled/32] |= 1 << (stateEnabled % 32)
			bits[stateSensitive/32] |= 1 << (stateSensitive % 32)
			body.u32Array(bits[:])
		} else {
			body.u32Array(atspiStates(node))
		}
		b.reply(msg, "au", body.data)
	case "GetAttributes":
		body := newMsgBuf(0)
		body.strDict(nil, nil)
		b.reply(msg, "a{ss}", body.data)
	case "GetApplication":
		body := newMsgBuf(0)
		body.ref(b.root())
		b.reply(msg, "(so)", body.data)
	case "GetInterfaces":
		body := newMsgBuf(0)
		body.strArray(b.interfacesFor(node, isRoot))
		b.reply(msg, "as", body.data)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
	}
}

func (b *atspiBridge) roleOf(node A11yNode, isRoot bool) uint32 {
	if isRoot {
		return roleApplication
	}
	// The tree's own root is the window the user sees; AT-SPI expects a frame
	// between the application and its content.
	if node.ParentID == -1 {
		return roleFrame
	}
	return atspiRole(node.Role)
}

// interfacesFor lists what an object supports. A screen reader uses this to
// decide what it can ask for, so claiming an interface we do not answer is
// worse than omitting it.
func (b *atspiBridge) interfacesFor(node A11yNode, isRoot bool) []string {
	if isRoot {
		return []string{ifaceAccessible, ifaceApplication}
	}
	out := []string{ifaceAccessible, ifaceComponent}
	if actionCount(node, isRoot) > 0 {
		out = append(out, ifaceAction)
	}
	return out
}

func (b *atspiBridge) handleComponent(msg *dbusMsg, tree *a11yTree, node A11yNode, isRoot bool) {
	switch msg.Member {
	case "GetExtents":
		body := newMsgBuf(0)
		body.structStart()
		body.i32(int32(node.X))
		body.i32(int32(node.Y))
		body.i32(int32(node.W))
		body.i32(int32(node.H))
		b.reply(msg, "(iiii)", body.data)
	case "GetPosition":
		body := newMsgBuf(0)
		body.i32(int32(node.X))
		body.i32(int32(node.Y))
		b.reply(msg, "ii", body.data)
	case "GetSize":
		body := newMsgBuf(0)
		body.i32(int32(node.W))
		body.i32(int32(node.H))
		b.reply(msg, "ii", body.data)
	case "Contains":
		d := newMsgDecoder(msg.Body, 0)
		x, err1 := d.readI32()
		y, err2 := d.readI32()
		body := newMsgBuf(0)
		body.bool32(err1 == nil && err2 == nil && containsPoint(node, int(x), int(y)))
		b.reply(msg, "b", body.data)
	case "GetAccessibleAtPoint":
		d := newMsgDecoder(msg.Body, 0)
		x, err1 := d.readI32()
		y, err2 := d.readI32()
		body := newMsgBuf(0)
		if err1 != nil || err2 != nil {
			body.ref(b.nullRef())
		} else {
			body.ref(b.hitTest(tree, node, isRoot, int(x), int(y)))
		}
		b.reply(msg, "(so)", body.data)
	case "GetLayer":
		body := newMsgBuf(0)
		body.u32(1) // ATSPI_LAYER_BACKGROUND — one plain layer, no MDI
		b.reply(msg, "u", body.data)
	case "GetMDIZOrder":
		body := newMsgBuf(0)
		body.i16(0)
		b.reply(msg, "n", body.data)
	case "GetAlpha":
		body := newMsgBuf(0)
		body.f64(1)
		b.reply(msg, "d", body.data)
	case "GrabFocus":
		// Focus is the app's to move, and nothing routes an external request
		// into the widget tree yet. Saying so beats claiming success.
		body := newMsgBuf(0)
		body.bool32(false)
		b.reply(msg, "b", body.data)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
	}
}

func containsPoint(n A11yNode, x, y int) bool {
	return x >= n.X && x < n.X+n.W && y >= n.Y && y < n.Y+n.H
}

// hitTest finds the deepest child of node containing the point. Children are
// searched last-first: later siblings paint over earlier ones, so the last
// match is the one the user sees.
func (b *atspiBridge) hitTest(tree *a11yTree, node A11yNode, isRoot bool, x, y int) objRef {
	var ids []int
	if isRoot {
		ids = tree.roots
	} else {
		ids = tree.children(node.ID)
	}
	for i := len(ids) - 1; i >= 0; i-- {
		child, ok := tree.byID[ids[i]]
		if !ok || !containsPoint(child, x, y) {
			continue
		}
		if deeper := b.hitTest(tree, child, false, x, y); deeper.Path != atspiNullPath {
			return deeper
		}
		return b.nodeRef(child.ID)
	}
	if !isRoot && containsPoint(node, x, y) {
		return b.nodeRef(node.ID)
	}
	return b.nullRef()
}

func (b *atspiBridge) handleAction(msg *dbusMsg, node A11yNode, isRoot bool, activate func(int)) {
	n := actionCount(node, isRoot)
	switch msg.Member {
	case "GetNActions":
		body := newMsgBuf(0)
		body.i32(int32(n))
		b.reply(msg, "i", body.data)
	case "GetName", "GetLocalizedName":
		body := newMsgBuf(0)
		body.str("click")
		b.reply(msg, "s", body.data)
	case "GetDescription", "GetKeyBinding":
		body := newMsgBuf(0)
		body.str("")
		b.reply(msg, "s", body.data)
	case "GetActions":
		body := newMsgBuf(0)
		lenPos, contentPos := body.arrayStart(8)
		if n > 0 {
			body.padTo(8)
			body.str("click")
			body.str("")
			body.str("")
		}
		body.arrayEnd(lenPos, contentPos)
		b.reply(msg, "a(sss)", body.data)
	case "DoAction":
		d := newMsgDecoder(msg.Body, 0)
		i, err := d.readI32()
		ok := err == nil && i == 0 && n > 0 && activate != nil
		if ok {
			// Runs on this goroutine, which is not the UI goroutine. The
			// caller's activate is responsible for marshalling; every bridge
			// gets the same contract (see a11y.go).
			activate(node.ID)
		}
		body := newMsgBuf(0)
		body.bool32(ok)
		b.reply(msg, "b", body.data)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
	}
}

func (b *atspiBridge) handleApplication(msg *dbusMsg) {
	switch msg.Member {
	case "GetLocale":
		body := newMsgBuf(0)
		body.str("C")
		b.reply(msg, "s", body.data)
	case "RegisterEventListener", "DeregisterEventListener":
		b.reply(msg, "", nil)
	default:
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
	}
}

// handleCacheCall answers the cache interface with nothing, which makes the
// caller fall back to querying objects directly. A populated cache is an
// optimisation, not a requirement, and its item signature is elaborate enough
// that an empty-but-correct answer is worth more than a wrong full one.
func (b *atspiBridge) handleCacheCall(msg *dbusMsg) {
	if msg.Member != "GetItems" {
		b.replyErr(msg, "org.freedesktop.DBus.Error.UnknownMethod", msg.Member)
		return
	}
	body := newMsgBuf(0)
	lenPos, contentPos := body.arrayStart(8)
	body.arrayEnd(lenPos, contentPos)
	b.reply(msg, "a((so)(so)(so)iiassusau)", body.data)
}

func strBody(s string) []byte {
	b := newMsgBuf(0)
	b.str(s)
	return b.data
}

// introspectXML is deliberately static: every accessible object offers the
// same interfaces, and a screen reader uses this only to discover what it may
// call.
var introspectXML = fmt.Sprintf(`<!DOCTYPE node PUBLIC "-//freedesktop//DTD D-BUS Object Introspection 1.0//EN" "http://www.freedesktop.org/standards/dbus/1.0/introspect.dtd">
<node>
  <interface name="%s">
    <method name="Introspect"><arg name="data" direction="out" type="s"/></method>
  </interface>
  <interface name="%s">
    <method name="Get"><arg direction="in" type="s"/><arg direction="in" type="s"/><arg direction="out" type="v"/></method>
    <method name="GetAll"><arg direction="in" type="s"/><arg direction="out" type="a{sv}"/></method>
  </interface>
  <interface name="%s">
    <method name="GetChildAtIndex"><arg direction="in" type="i"/><arg direction="out" type="(so)"/></method>
    <method name="GetChildren"><arg direction="out" type="a(so)"/></method>
    <method name="GetIndexInParent"><arg direction="out" type="i"/></method>
    <method name="GetRelationSet"><arg direction="out" type="a(ua(so))"/></method>
    <method name="GetRole"><arg direction="out" type="u"/></method>
    <method name="GetRoleName"><arg direction="out" type="s"/></method>
    <method name="GetLocalizedRoleName"><arg direction="out" type="s"/></method>
    <method name="GetState"><arg direction="out" type="au"/></method>
    <method name="GetAttributes"><arg direction="out" type="a{ss}"/></method>
    <method name="GetApplication"><arg direction="out" type="(so)"/></method>
    <method name="GetInterfaces"><arg direction="out" type="as"/></method>
  </interface>
  <interface name="%s">
    <method name="Contains"><arg direction="in" type="i"/><arg direction="in" type="i"/><arg direction="in" type="u"/><arg direction="out" type="b"/></method>
    <method name="GetAccessibleAtPoint"><arg direction="in" type="i"/><arg direction="in" type="i"/><arg direction="in" type="u"/><arg direction="out" type="(so)"/></method>
    <method name="GetExtents"><arg direction="in" type="u"/><arg direction="out" type="(iiii)"/></method>
    <method name="GetPosition"><arg direction="in" type="u"/><arg direction="out" type="i"/><arg direction="out" type="i"/></method>
    <method name="GetSize"><arg direction="out" type="i"/><arg direction="out" type="i"/></method>
    <method name="GetLayer"><arg direction="out" type="u"/></method>
    <method name="GetAlpha"><arg direction="out" type="d"/></method>
    <method name="GrabFocus"><arg direction="out" type="b"/></method>
  </interface>
  <interface name="%s">
    <method name="GetNActions"><arg direction="out" type="i"/></method>
    <method name="GetActions"><arg direction="out" type="a(sss)"/></method>
    <method name="DoAction"><arg direction="in" type="i"/><arg direction="out" type="b"/></method>
  </interface>
</node>`, ifaceIntrospect, ifaceProps, ifaceAccessible, ifaceComponent, ifaceAction)
