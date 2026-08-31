//go:build windows

// Provider behaviour: what an element answers, and how the window hands one out.

package platform

import (
	"strconv"
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

// property fills v with one property's value.
//
// Leaving v empty is a legitimate answer — UIA treats VT_EMPTY as "this element
// has no opinion" and moves on, which is what should happen for a property that
// does not apply. Guessing instead produces something a screen reader reads
// aloud.
func (e *uiaElem) property(id int32, v *variant) {
	// Clear first, so a property that does not apply leaves VT_EMPTY rather
	// than whatever the variant held before. The vtable also clears, but
	// depending on that made this function wrong to call on its own — a
	// checkbox's ToggleState would leak into the next element asked.
	v.setEmpty()

	if e.id == rootElemID {
		uiaLogf("ROOT prop id=%d", id)
		e.rootProperty(id, v)
		return
	}
	n, ok := e.node()
	if !ok {
		return
	}
	uiaLogf("prop id=%d node=%d", id, e.id)
	switch id {
	case propName:
		name := n.Label
		if name == "" {
			name = n.Value
		}
		v.setString(name)
	case propControlType:
		v.setI4(uiaControlType(n.Role))
	case propHelpText:
		if n.Hint != "" {
			v.setString(n.Hint)
		}
	case propAutomationID:
		v.setString(strconv.Itoa(n.ID))
	case propIsEnabled:
		v.setBool(!n.Disabled)
	case propIsKeyboardFocusable:
		v.setBool(n.Tappable)
	case propHasKeyboardFocus:
		v.setBool(n.Focused)
	case propIsControlElement, propIsContentElement:
		// Both true, so the node appears in the control view and the content
		// view. A node absent from both is invisible to a screen reader even
		// though every other property is correct.
		v.setBool(true)
	case propIsOffscreen:
		v.setBool(false)
	case propProcessID:
		v.setI4(int32(windows.GetCurrentProcessId()))
	case propBoundingRectangle:
		// Also served by IRawElementProviderFragment.get_BoundingRectangle, but
		// that is not the route clients take: UIA asks for the property, and a
		// provider that only implements the fragment method reports every
		// element as zero-sized — which is what a screen reader uses to place
		// its highlight, so the tree reads correctly and highlights nothing.
		x, y := e.prov.clientToScreen(n.X, n.Y)
		uiaLogf("prop bounds id=%d node=(%d,%d %dx%d) screen=(%d,%d)", e.id, n.X, n.Y, n.W, n.H, x, y)
		v.setRect(float64(x), float64(y), float64(n.W), float64(n.H))
	case propToggleState:
		if supportsToggle(n) {
			v.setI4(toggleState(n))
		}
	case propIsInvokePatternAvail:
		v.setBool(supportsInvoke(n))
	case propIsTogglePatternAvail:
		v.setBool(supportsToggle(n))
	case propExpandCollapseState:
		if supportsExpandCollapse(n) {
			v.setI4(expandCollapseState(n))
		}
	case propIsExpandCollapseAvail:
		v.setBool(supportsExpandCollapse(n))
	}
}

// rootProperty answers for the window element, which has no gophics node.
func (e *uiaElem) rootProperty(id int32, v *variant) {
	switch id {
	case propControlType:
		v.setI4(ctWindow)
	case propIsControlElement, propIsContentElement, propIsEnabled:
		v.setBool(true)
	case propIsOffscreen, propHasKeyboardFocus, propIsKeyboardFocusable:
		v.setBool(false)
	case propAutomationID:
		v.setString("gophics.root")
	case propProcessID:
		v.setI4(int32(windows.GetCurrentProcessId()))
	case propBoundingRectangle:
		l, t, w, h := e.prov.windowRect()
		uiaLogf("ROOT bounds -> %v,%v %vx%v", l, t, w, h)
		v.setRect(l, t, w, h)
	case propName:
		// The window's own title is what the shell already shows; leaving this
		// empty lets UIA fall back to the host provider rather than duplicating
		// or contradicting it.
	}
}

// navigate walks the tree in one direction, returning nil at the edges.
//
// The window element sits above the published tree: its children are the
// tree's roots, and it has no parent of its own (UIA supplies the desktop).
func (e *uiaElem) navigate(dir int32) *uiaElem {
	t := e.tree()
	if t == nil {
		return nil
	}
	p := e.prov

	if e.id == rootElemID {
		switch dir {
		case navFirstChild:
			if len(t.roots) > 0 {
				return p.newElem(t.roots[0])
			}
		case navLastChild:
			if n := len(t.roots); n > 0 {
				return p.newElem(t.roots[n-1])
			}
		}
		return nil
	}

	n, ok := t.byID[e.id]
	if !ok {
		return nil
	}
	switch dir {
	case navParent:
		if n.ParentID == -1 {
			return p.rootElem()
		}
		if _, ok := t.byID[n.ParentID]; !ok {
			return p.rootElem()
		}
		return p.newElem(n.ParentID)
	case navFirstChild:
		if kids := t.kids[e.id]; len(kids) > 0 {
			return p.newElem(kids[0])
		}
	case navLastChild:
		if kids := t.kids[e.id]; len(kids) > 0 {
			return p.newElem(kids[len(kids)-1])
		}
	case navNextSibling:
		if sibs, i := t.siblings(e.id); i >= 0 && i+1 < len(sibs) {
			return p.newElem(sibs[i+1])
		}
	case navPreviousSibling:
		if sibs, i := t.siblings(e.id); i > 0 {
			return p.newElem(sibs[i-1])
		}
	}
	return nil
}

// rootElem returns a fresh element for the window itself.
func (p *uiaProvider) rootElem() *uiaElem { return p.newElem(rootElemID) }

// focused returns the element the app considers focused, or nil.
func (p *uiaProvider) focused() *uiaElem {
	p.mu.RLock()
	t := p.tree
	p.mu.RUnlock()
	if t == nil {
		return nil
	}
	for _, n := range t.nodes {
		if n.Focused {
			return p.newElem(n.ID)
		}
	}
	return nil
}

// hitTest finds the deepest node containing a client-space point. Later
// siblings paint over earlier ones, so the search runs back to front.
func (p *uiaProvider) hitTest(x, y int) *uiaElem {
	p.mu.RLock()
	t := p.tree
	p.mu.RUnlock()
	if t == nil {
		return nil
	}
	var walk func(ids []int) *uiaElem
	walk = func(ids []int) *uiaElem {
		for i := len(ids) - 1; i >= 0; i-- {
			n, ok := t.byID[ids[i]]
			if !ok || x < n.X || x >= n.X+n.W || y < n.Y || y >= n.Y+n.H {
				continue
			}
			if deeper := walk(t.kids[n.ID]); deeper != nil {
				return deeper
			}
			return p.newElem(n.ID)
		}
		return nil
	}
	return walk(t.roots)
}

// clientToScreen converts gophics' client-space pixels to the screen
// coordinates UIA reports bounds in.
func (p *uiaProvider) clientToScreen(x, y int) (int, int) {
	pt := struct{ X, Y int32 }{int32(x), int32(y)}
	procClientToScreen.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

func (p *uiaProvider) screenToClient(x, y int) (int, int) {
	pt := struct{ X, Y int32 }{int32(x), int32(y)}
	procScreenToClient.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&pt)))
	return int(pt.X), int(pt.Y)
}

// windowRect returns the window's bounds in screen coordinates, which is what
// the fragment root reports.
func (p *uiaProvider) windowRect() (left, top, width, height float64) {
	var r struct{ Left, Top, Right, Bottom int32 }
	procGetWindowRect.Call(uintptr(p.hwnd), uintptr(unsafe.Pointer(&r)))
	return float64(r.Left), float64(r.Top), float64(r.Right - r.Left), float64(r.Bottom - r.Top)
}

// SetTree publishes a new tree.
func (p *uiaProvider) SetTree(nodes []A11yNode, activate func(id int)) {
	p.mu.Lock()
	p.tree = buildTree(nodes)
	p.activate = activate
	p.mu.Unlock()
}

// --- window integration -----------------------------------------------------

// uiaHosts maps an HWND to its provider. A provider is created the first time
// the window publishes a tree, not at window creation: most apps never publish
// one, and building it costs a COM object and a map entry per window.
var (
	uiaMu    sync.Mutex
	uiaHosts = map[windows.HWND]*uiaProvider{}
)

func uiaProviderFor(hwnd windows.HWND) *uiaProvider {
	uiaMu.Lock()
	defer uiaMu.Unlock()
	p := uiaHosts[hwnd]
	if p == nil {
		p = &uiaProvider{hwnd: hwnd}
		uiaHosts[hwnd] = p
	}
	return p
}

func uiaLookup(hwnd windows.HWND) *uiaProvider {
	uiaMu.Lock()
	defer uiaMu.Unlock()
	return uiaHosts[hwnd]
}

// handleGetObject answers WM_GETOBJECT.
//
// Windows asks every window this, for several different object models —
// MSAA, the client area, and UIA — distinguished by lParam. Only
// UiaRootObjectId is ours; answering anything else, or answering before a tree
// exists, tells the system this window has an accessibility implementation when
// it has nothing to say.
//
// Reports handled=false to let DefWindowProc take the message.
func handleGetObject(hwnd windows.HWND, wParam, lParam uintptr) (uintptr, bool) {
	uiaLogf("WM_GETOBJECT lParam=%d wantRoot=%d", int32(lParam), uiaRootObjectID)
	if int32(lParam) != uiaRootObjectID {
		return 0, false
	}
	p := uiaLookup(hwnd)
	if p == nil {
		uiaLogf("no provider for this window yet")
		return 0, false
	}
	p.mu.RLock()
	empty := p.tree == nil || len(p.tree.nodes) == 0
	p.mu.RUnlock()
	if empty {
		uiaLogf("provider exists but has no tree")
		return 0, false
	}
	root := p.rootElem()
	uiaLogf("returning provider")
	ret, _, _ := procUiaReturnRawElementProvider.Call(
		uintptr(hwnd), wParam, lParam, root.simplePtr())
	return ret, true
}
