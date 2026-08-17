//go:build windows

// A UI Automation provider, in pure Go.
//
// UIA is COM, and COM is a calling convention rather than a library: an object
// is a pointer whose first machine word points at a table of function
// pointers. syscall.NewCallback turns a Go closure into such a pointer, so a
// provider can be built without CGo. This repo already relies on that —
// internal/gfx/naga/internal/dxcvalidator hands dxil.dll a Go IDxcBlob the same
// way — but this is a bigger version of the idea: real reference counting, real
// QueryInterface, and four interfaces on one object.
//
// # Layout
//
// A COM object exposing several interfaces is laid out the way a C++ compiler
// lays out multiple inheritance: one vtable pointer per interface, at fixed
// offsets, all inside the same allocation. Handing out the address of the
// second slot yields a valid pointer to the second interface, and a method can
// recover the object by subtracting its slot's offset. That is what the
// vtbl* fields below are, and why their order must not change.
//
// # Lifetime
//
// UIA holds provider pointers across calls and releases them on its own
// schedule, so an element cannot be an ordinary Go value that goes out of
// scope. Live elements are kept in a map keyed by their own address, which both
// keeps them reachable for the garbage collector and lets a callback find the
// object from a bare `this` pointer. Release removes them.

package platform

import (
	"sync"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	uiaCore = windows.NewLazySystemDLL("uiautomationcore.dll")
	oleaut  = windows.NewLazySystemDLL("oleaut32.dll")

	procUiaReturnRawElementProvider = uiaCore.NewProc("UiaReturnRawElementProvider")
	procUiaHostProviderFromHwnd     = uiaCore.NewProc("UiaHostProviderFromHwnd")
	procUiaRaiseAutomationEvent     = uiaCore.NewProc("UiaRaiseAutomationEvent")
	procUiaDisconnectProvider       = uiaCore.NewProc("UiaDisconnectProvider")

	procSysAllocString        = oleaut.NewProc("SysAllocString")
	procSafeArrayCreateVector = oleaut.NewProc("SafeArrayCreateVector")
	procSafeArrayPutElement   = oleaut.NewProc("SafeArrayPutElement")
)

// COM HRESULTs.
const (
	sOK                  = 0
	eNoInterface         = 0x80004002
	eInvalidArg          = 0x80070057
	eFail                = 0x80004005
	eNotImpl             = 0x80004001
	eElementNotAvailable = 0x80040201 // UIA_E_ELEMENTNOTAVAILABLE
)

// UIA_RootObjectId, the lParam WM_GETOBJECT uses to ask for a UIA provider.
const uiaRootObjectID = -25

// ProviderOptions_ServerSideProvider.
const providerOptionsServerSide = 1

// VARIANT types.
const (
	vtEmpty = 0
	vtI4    = 3
	vtBSTR  = 8
	vtBool  = 11
	vtR8    = 5
)

// variant mirrors the Win32 VARIANT: a type tag, three reserved words, then the
// value union. 24 bytes on 64-bit, and the layout matters because UIA writes
// through this pointer.
type variant struct {
	vt         uint16
	r1, r2, r3 uint16
	val        uintptr
	_          uintptr
}

func (v *variant) setEmpty()     { *v = variant{vt: vtEmpty} }
func (v *variant) setI4(x int32) { *v = variant{vt: vtI4, val: uintptr(uint32(x))} }
func (v *variant) setR8(x float64) {
	*v = variant{vt: vtR8, val: uintptr(*(*uint64)(unsafe.Pointer(&x)))}
}

// setBool writes a VARIANT_BOOL, whose true is -1 rather than 1 — a detail that
// silently inverts nothing and confuses everything if got wrong, since 1 is not
// VARIANT_TRUE and some consumers compare against it exactly.
func (v *variant) setBool(b bool) {
	var x uintptr
	if b {
		x = 0xFFFF
	}
	*v = variant{vt: vtBool, val: x}
}

func (v *variant) setString(s string) {
	bstr := sysAllocString(s)
	if bstr == 0 {
		v.setEmpty()
		return
	}
	*v = variant{vt: vtBSTR, val: bstr}
}

func sysAllocString(s string) uintptr {
	p, err := windows.UTF16PtrFromString(s)
	if err != nil {
		return 0
	}
	r, _, _ := procSysAllocString.Call(uintptr(unsafe.Pointer(p)))
	return r
}

// uiaRect is UiaRect: four doubles, in screen coordinates.
type uiaRect struct{ left, top, width, height float64 }

// --- GUIDs ------------------------------------------------------------------

type guid struct {
	data1 uint32
	data2 uint16
	data3 uint16
	data4 [8]byte
}

func (g *guid) equals(o *guid) bool {
	return g.data1 == o.data1 && g.data2 == o.data2 && g.data3 == o.data3 && g.data4 == o.data4
}

var (
	iidUnknown = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}
	// IRawElementProviderSimple
	iidSimple = guid{0xD6DD68D1, 0x86FD, 0x4332, [8]byte{0x86, 0x66, 0x9A, 0xBE, 0xDE, 0xA2, 0xD2, 0x4C}}
	// IRawElementProviderFragment
	iidFragment = guid{0xF7063DA8, 0x8359, 0x439C, [8]byte{0x92, 0x97, 0xBB, 0xC5, 0x29, 0x9A, 0x7D, 0x87}}
	// IRawElementProviderFragmentRoot
	iidFragmentRoot = guid{0x620CE2A5, 0xAB8F, 0x40A9, [8]byte{0x86, 0xCB, 0xDE, 0x3C, 0x75, 0x59, 0x9B, 0x58}}
	// IInvokeProvider
	iidInvoke = guid{0x54FCB24B, 0xE18E, 0x47A2, [8]byte{0xB4, 0xD3, 0xEC, 0xCB, 0xE7, 0x75, 0x99, 0xA2}}
	// IToggleProvider
	iidToggle = guid{0x56D00BD0, 0xC4F4, 0x433C, [8]byte{0xA8, 0x36, 0x1A, 0x52, 0xA5, 0x7E, 0x08, 0x92}}
)

// --- element ----------------------------------------------------------------

// rootElemID marks the element standing for the window itself, which is not a
// gophics node.
const rootElemID = -1

// uiaElem is one automation element. The four vtable pointers must stay first
// and in this order: their offsets are what make a pointer to any of them a
// valid COM interface pointer for that interface.
type uiaElem struct {
	vtblSimple   uintptr // offset 0
	vtblFragment uintptr // offset 8
	vtblRoot     uintptr // offset 16
	vtblPattern  uintptr // offset 24 — Invoke and Toggle share a slot; see below

	prov *uiaProvider
	id   int
	refs int32
}

// Offsets of each vtable slot, used to recover the element from `this`.
const (
	offSimple   = 0
	offFragment = 8
	offRoot     = 16
	offPattern  = 24
)

// live keeps elements reachable while UIA holds them, and maps an interface
// pointer back to its element.
var (
	liveMu sync.Mutex
	live   = map[uintptr]*uiaElem{}
)

// elemFrom recovers the element behind a `this` pointer for a given slot.
func elemFrom(this uintptr, off uintptr) *uiaElem {
	liveMu.Lock()
	defer liveMu.Unlock()
	return live[this-off]
}

func (e *uiaElem) base() uintptr { return uintptr(unsafe.Pointer(e)) }

// --- provider ---------------------------------------------------------------

// uiaProvider owns the tree for one window.
type uiaProvider struct {
	hwnd windows.HWND

	mu       sync.RWMutex
	tree     *a11yTreeWin
	activate func(id int)
}

// a11yTreeWin indexes a published node list. It mirrors the Linux a11yTree but
// is declared separately because that one is behind a linux build tag.
type a11yTreeWin struct {
	nodes []A11yNode
	byID  map[int]A11yNode
	kids  map[int][]int
	roots []int
}

func buildTreeWin(nodes []A11yNode) *a11yTreeWin {
	t := &a11yTreeWin{byID: make(map[int]A11yNode, len(nodes)), kids: map[int][]int{}, nodes: nodes}
	for _, n := range nodes {
		t.byID[n.ID] = n
	}
	for _, n := range nodes {
		if n.ParentID == -1 {
			t.roots = append(t.roots, n.ID)
			continue
		}
		if _, ok := t.byID[n.ParentID]; !ok {
			t.roots = append(t.roots, n.ID)
			continue
		}
		t.kids[n.ParentID] = append(t.kids[n.ParentID], n.ID)
	}
	return t
}

// siblings returns the ordered sibling list containing id, and id's position.
func (t *a11yTreeWin) siblings(id int) ([]int, int) {
	n, ok := t.byID[id]
	if !ok {
		return nil, -1
	}
	list := t.roots
	if n.ParentID != -1 {
		if _, ok := t.byID[n.ParentID]; ok {
			list = t.kids[n.ParentID]
		}
	}
	for i, s := range list {
		if s == id {
			return list, i
		}
	}
	return list, -1
}

// newElem allocates an element and registers it as live. Every element starts
// with one reference: the one being handed to the caller.
func (p *uiaProvider) newElem(id int) *uiaElem {
	e := &uiaElem{prov: p, id: id, refs: 1}
	e.vtblSimple = vtSimple()
	e.vtblFragment = vtFragment()
	e.vtblRoot = vtRoot()
	e.vtblPattern = vtPattern()

	liveMu.Lock()
	live[e.base()] = e
	liveMu.Unlock()
	return e
}

// simplePtr is the IRawElementProviderSimple pointer for this element, which is
// what every UIA entry point wants.
func (e *uiaElem) simplePtr() uintptr { return e.base() + offSimple }

func (e *uiaElem) node() (A11yNode, bool) {
	e.prov.mu.RLock()
	defer e.prov.mu.RUnlock()
	if e.prov.tree == nil {
		return A11yNode{}, false
	}
	n, ok := e.prov.tree.byID[e.id]
	return n, ok
}

func (e *uiaElem) tree() *a11yTreeWin {
	e.prov.mu.RLock()
	defer e.prov.mu.RUnlock()
	return e.prov.tree
}

// --- IUnknown ---------------------------------------------------------------

// queryInterface is shared by every vtable: the interface a caller asks for
// decides which slot's address is returned, and each hand-out adds a reference.
func (e *uiaElem) queryInterface(riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return eInvalidArg
	}
	id := (*guid)(unsafe.Pointer(riid)) //nolint:govet // riid is a REFIID from COM
	out := (*uintptr)(unsafe.Pointer(ppv))

	var slot uintptr
	switch {
	case id.equals(&iidUnknown), id.equals(&iidSimple):
		slot = e.base() + offSimple
	case id.equals(&iidFragment):
		slot = e.base() + offFragment
	case id.equals(&iidFragmentRoot):
		// Only the window element is a fragment root; a button is not, and
		// saying otherwise makes UIA treat it as the top of its own tree.
		if e.id != rootElemID {
			*out = 0
			return eNoInterface
		}
		slot = e.base() + offRoot
	case id.equals(&iidInvoke):
		n, ok := e.node()
		if !ok || !supportsInvoke(n) {
			*out = 0
			return eNoInterface
		}
		slot = e.base() + offPattern
	case id.equals(&iidToggle):
		n, ok := e.node()
		if !ok || !supportsToggle(n) {
			*out = 0
			return eNoInterface
		}
		slot = e.base() + offPattern
	default:
		*out = 0
		return eNoInterface
	}
	e.addRef()
	*out = slot
	return sOK
}

func (e *uiaElem) addRef() uintptr {
	liveMu.Lock()
	defer liveMu.Unlock()
	e.refs++
	return uintptr(e.refs)
}

func (e *uiaElem) release() uintptr {
	liveMu.Lock()
	defer liveMu.Unlock()
	e.refs--
	n := e.refs
	if n <= 0 {
		// Dropping it from the map is what finally lets the collector take it.
		delete(live, e.base())
	}
	return uintptr(n)
}

// unknownSlots builds the three IUnknown thunks for a vtable whose `this`
// points at the slot at offset off.
func unknownSlots(off uintptr) [3]uintptr {
	return [3]uintptr{
		syscall.NewCallback(func(this, riid, ppv uintptr) uintptr {
			e := elemFrom(this, off)
			if e == nil {
				return eFail
			}
			return e.queryInterface(riid, ppv)
		}),
		syscall.NewCallback(func(this uintptr) uintptr {
			e := elemFrom(this, off)
			if e == nil {
				return 1
			}
			return e.addRef()
		}),
		syscall.NewCallback(func(this uintptr) uintptr {
			e := elemFrom(this, off)
			if e == nil {
				return 0
			}
			return e.release()
		}),
	}
}
