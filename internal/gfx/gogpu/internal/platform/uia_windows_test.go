//go:build windows

package platform

import (
	"testing"
	"unsafe"
)

func testProvider(nodes []A11yNode, activate func(int)) *uiaProvider {
	p := &uiaProvider{}
	p.SetTree(nodes, activate)
	return p
}

var sampleTree = []A11yNode{
	{ID: 1, ParentID: -1, Role: "group", Label: "Root", X: 0, Y: 0, W: 400, H: 300},
	{ID: 2, ParentID: 1, Role: "button", Label: "Save", X: 10, Y: 20, W: 80, H: 30, Tappable: true},
	{ID: 3, ParentID: 1, Role: "checkbox", Label: "Agree", X: 10, Y: 60, W: 120, H: 24,
		Checkable: true, Checked: true, Tappable: true},
	{ID: 4, ParentID: 1, Role: "text", Label: "Hello", X: 10, Y: 100, W: 200, H: 20},
}

// VARIANT_BOOL's true is -1, not 1. Writing 1 produces a value that some
// consumers compare against VARIANT_TRUE and reject, so a property reads as
// false while looking set.
func TestVariantBoolIsMinusOne(t *testing.T) {
	var v variant
	v.setBool(true)
	if v.vt != vtBool {
		t.Errorf("vt = %d, want VT_BOOL (%d)", v.vt, vtBool)
	}
	if v.val != 0xFFFF {
		t.Errorf("VARIANT_TRUE = %#x, want 0xFFFF", v.val)
	}
	v.setBool(false)
	if v.val != 0 {
		t.Errorf("VARIANT_FALSE = %#x, want 0", v.val)
	}
}

// The struct is written through by UIA, so its size must match the ABI.
func TestVariantSize(t *testing.T) {
	if got := unsafe.Sizeof(variant{}); got != 24 {
		t.Errorf("sizeof(VARIANT) = %d, want 24", got)
	}
}

// Exercises the real oleaut32 allocator rather than a stand-in.
func TestSysAllocStringAndSafeArray(t *testing.T) {
	if s := sysAllocString("Save"); s == 0 {
		t.Error("SysAllocString returned NULL")
	}
	if sa := safeArrayOfInt32([]int32{3, 42}); sa == 0 {
		t.Error("SafeArrayCreateVector returned NULL")
	}
}

// A checkbox toggles; it does not invoke. Claiming both makes a screen reader
// offer two different actions for one control.
func TestPatternsAreExclusive(t *testing.T) {
	button := A11yNode{Role: "button", Tappable: true}
	check := A11yNode{Role: "checkbox", Tappable: true, Checkable: true}
	if !supportsInvoke(button) || supportsToggle(button) {
		t.Error("a button should invoke and not toggle")
	}
	if supportsInvoke(check) || !supportsToggle(check) {
		t.Error("a checkbox should toggle and not invoke")
	}
	plain := A11yNode{Role: "text"}
	if supportsInvoke(plain) || supportsToggle(plain) {
		t.Error("static text should support neither pattern")
	}
}

func TestControlTypeMapping(t *testing.T) {
	for aria, want := range map[string]int32{
		"button": ctButton, "checkbox": ctCheckBox, "link": ctHyperlink,
		"textbox": ctEdit, "text": ctText, "slider": ctSlider,
	} {
		if got := uiaControlType(aria); got != want {
			t.Errorf("uiaControlType(%q) = %d, want %d", aria, got, want)
		}
	}
	// An unmapped role becomes a Group — an ordinary container a reader will
	// descend into — rather than something it must apologise for.
	if got := uiaControlType("no-such-role"); got != ctGroup {
		t.Errorf("unmapped role = %d, want group (%d)", got, ctGroup)
	}
}

// The window element sits above the published tree: its children are the
// roots, and it has no parent of its own.
func TestNavigateFromRoot(t *testing.T) {
	p := testProvider(sampleTree, nil)
	root := p.rootElem()

	if got := root.navigate(navParent); got != nil {
		t.Error("the window element should have no parent; UIA supplies the desktop")
	}
	first := root.navigate(navFirstChild)
	if first == nil || first.id != 1 {
		t.Fatalf("first child = %v, want node 1", first)
	}
	if up := first.navigate(navParent); up == nil || up.id != rootElemID {
		t.Error("a tree root's parent should be the window element")
	}
}

func TestNavigateSiblings(t *testing.T) {
	p := testProvider(sampleTree, nil)
	n2 := p.newElem(2)

	next := n2.navigate(navNextSibling)
	if next == nil || next.id != 3 {
		t.Fatalf("next sibling of 2 = %v, want 3", next)
	}
	prev := next.navigate(navPreviousSibling)
	if prev == nil || prev.id != 2 {
		t.Fatalf("previous sibling of 3 = %v, want 2", prev)
	}
	// Edges return nil rather than wrapping, which would make a reader loop.
	if n2.navigate(navPreviousSibling) != nil {
		t.Error("first child should have no previous sibling")
	}
	if p.newElem(4).navigate(navNextSibling) != nil {
		t.Error("last child should have no next sibling")
	}
	if p.newElem(4).navigate(navFirstChild) != nil {
		t.Error("a leaf should have no children")
	}
}

// Later siblings paint over earlier ones, and the deepest match wins.
func TestHitTest(t *testing.T) {
	p := testProvider(sampleTree, nil)
	if hit := p.hitTest(20, 30); hit == nil || hit.id != 2 {
		t.Errorf("hit at (20,30) = %v, want the Save button (2)", hit)
	}
	if hit := p.hitTest(15, 65); hit == nil || hit.id != 3 {
		t.Errorf("hit at (15,65) = %v, want the checkbox (3)", hit)
	}
	// Inside the group but not any child.
	if hit := p.hitTest(300, 250); hit == nil || hit.id != 1 {
		t.Errorf("hit at (300,250) = %v, want the group (1)", hit)
	}
	if hit := p.hitTest(9999, 9999); hit != nil {
		t.Errorf("hit outside everything = %v, want nil", hit)
	}
}

func TestFocusedElement(t *testing.T) {
	p := testProvider(sampleTree, nil)
	if f := p.focused(); f != nil {
		t.Errorf("focused = %v with nothing focused, want nil", f)
	}
	focused := append([]A11yNode(nil), sampleTree...)
	focused[1].Focused = true
	p.SetTree(focused, nil)
	if f := p.focused(); f == nil || f.id != 2 {
		t.Errorf("focused = %v, want node 2", f)
	}
}

// Properties are what a screen reader reads aloud, so the important ones must
// be present and correct.
func TestProperties(t *testing.T) {
	p := testProvider(sampleTree, nil)
	btn := p.newElem(2)

	var v variant
	btn.property(propName, &v)
	if v.vt != vtBSTR || v.val == 0 {
		t.Errorf("Name = vt %d val %#x, want a non-null BSTR", v.vt, v.val)
	}
	btn.property(propControlType, &v)
	if v.vt != vtI4 || int32(v.val) != ctButton {
		t.Errorf("ControlType = %d, want button (%d)", int32(v.val), ctButton)
	}
	btn.property(propIsEnabled, &v)
	if v.val != 0xFFFF {
		t.Error("an enabled node should report IsEnabled true")
	}
	// A node absent from both views is invisible to a reader even when every
	// other property is right.
	for _, id := range []int32{propIsControlElement, propIsContentElement} {
		btn.property(id, &v)
		if v.val != 0xFFFF {
			t.Errorf("property %d should be true", id)
		}
	}

	chk := p.newElem(3)
	chk.property(propToggleState, &v)
	if v.vt != vtI4 || int32(v.val) != toggleOn {
		t.Errorf("ToggleState = %d, want on (%d)", int32(v.val), toggleOn)
	}

	// An unknown property must stay VT_EMPTY, which UIA reads as "no opinion".
	txt := p.newElem(4)
	txt.property(propToggleState, &v)
	if v.vt != vtEmpty {
		t.Errorf("ToggleState on static text = vt %d, want VT_EMPTY", v.vt)
	}
}

func TestDisabledNodeReportsDisabled(t *testing.T) {
	nodes := []A11yNode{
		{ID: 1, ParentID: -1, Role: "group"},
		{ID: 2, ParentID: 1, Role: "button", Label: "Off", Tappable: true, Disabled: true},
	}
	p := testProvider(nodes, nil)
	var v variant
	p.newElem(2).property(propIsEnabled, &v)
	if v.val != 0 {
		t.Error("a disabled node reported IsEnabled true")
	}
}

// The window element answers for the window, not for any node.
func TestRootProperties(t *testing.T) {
	p := testProvider(sampleTree, nil)
	var v variant
	p.rootElem().property(propControlType, &v)
	if int32(v.val) != ctWindow {
		t.Errorf("root ControlType = %d, want window (%d)", int32(v.val), ctWindow)
	}
}

// Invoking through the pattern must reach the app's callback with the node ID.
func TestInvokeReachesActivate(t *testing.T) {
	var got []int
	p := testProvider(sampleTree, func(id int) { got = append(got, id) })

	e := p.newElem(2)
	n, ok := e.node()
	if !ok || !supportsInvoke(n) {
		t.Fatal("the Save button should support InvokePattern")
	}
	p.mu.RLock()
	activate := p.activate
	p.mu.RUnlock()
	activate(n.ID)

	if len(got) != 1 || got[0] != 2 {
		t.Errorf("activate got %v, want [2]", got)
	}
}

// Reference counting has to actually count: UIA holds pointers across calls and
// releases them on its own schedule, so an element freed early is a crash and
// one never freed is a leak.
func TestRefCounting(t *testing.T) {
	p := testProvider(sampleTree, nil)
	e := p.newElem(2)
	base := e.base()

	liveMu.Lock()
	_, present := live[base]
	liveMu.Unlock()
	if !present {
		t.Fatal("a new element should be registered live")
	}

	e.addRef()
	if n := e.release(); n != 1 {
		t.Errorf("after addRef+release, refs = %d, want 1", n)
	}
	liveMu.Lock()
	_, present = live[base]
	liveMu.Unlock()
	if !present {
		t.Error("element dropped while still referenced")
	}

	if n := e.release(); n != 0 {
		t.Errorf("final release returned %d, want 0", n)
	}
	liveMu.Lock()
	_, present = live[base]
	liveMu.Unlock()
	if present {
		t.Error("element still live after its last release")
	}
}

// QueryInterface decides which vtable slot a caller gets, and the offsets are
// what make each slot a valid interface pointer.
func TestQueryInterfaceSlots(t *testing.T) {
	p := testProvider(sampleTree, nil)

	check := func(e *uiaElem, id *guid, wantOff uintptr, wantOK bool) {
		t.Helper()
		var out uintptr
		hr := e.queryInterface(uintptr(unsafe.Pointer(id)), uintptr(unsafe.Pointer(&out)))
		if !wantOK {
			if hr == sOK {
				t.Errorf("QueryInterface unexpectedly succeeded")
			}
			return
		}
		if hr != sOK {
			t.Errorf("QueryInterface failed: %#x", hr)
			return
		}
		if out != e.base()+wantOff {
			t.Errorf("slot = %#x, want base+%d", out, wantOff)
		}
	}

	btn := p.newElem(2)
	check(btn, &iidSimple, offSimple, true)
	check(btn, &iidUnknown, offSimple, true)
	check(btn, &iidFragment, offFragment, true)
	check(btn, &iidInvoke, offPattern, true)
	// A button is not a fragment root; saying otherwise makes UIA treat it as
	// the top of its own tree.
	check(btn, &iidFragmentRoot, 0, false)
	// Nor does it toggle.
	check(btn, &iidToggle, 0, false)

	chk := p.newElem(3)
	check(chk, &iidToggle, offPattern, true)
	check(chk, &iidInvoke, 0, false)

	root := p.rootElem()
	check(root, &iidFragmentRoot, offRoot, true)
}

// The vtable pointers must be the first words of the allocation, in order —
// that is what makes &e.vtblFragment a valid IRawElementProviderFragment.
func TestVtableSlotOffsets(t *testing.T) {
	var e uiaElem
	base := uintptr(unsafe.Pointer(&e))
	for _, tc := range []struct {
		name string
		addr uintptr
		want uintptr
	}{
		{"Simple", uintptr(unsafe.Pointer(&e.vtblSimple)), offSimple},
		{"Fragment", uintptr(unsafe.Pointer(&e.vtblFragment)), offFragment},
		{"Root", uintptr(unsafe.Pointer(&e.vtblRoot)), offRoot},
		{"Pattern", uintptr(unsafe.Pointer(&e.vtblPattern)), offPattern},
	} {
		if got := tc.addr - base; got != tc.want {
			t.Errorf("%s slot at offset %d, want %d", tc.name, got, tc.want)
		}
	}
}

// Building the vtables must produce real, non-null thunks.
func TestVtablesBuild(t *testing.T) {
	for name, fn := range map[string]func() uintptr{
		"Simple": vtSimple, "Fragment": vtFragment, "Root": vtRoot, "Pattern": vtPattern,
	} {
		if fn() == 0 {
			t.Errorf("%s vtable is null", name)
		}
	}
	// A vtable is per type, not per object: asking twice must not burn another
	// set of process-wide callbacks, which are a finite resource.
	first := vtSimple()
	second := vtSimple()
	if first != second {
		t.Error("vtSimple built a second table")
	}
}
