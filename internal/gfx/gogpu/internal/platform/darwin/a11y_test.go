//go:build darwin

package darwin

import (
	"testing"
	"unsafe"
)

// These tests drive the real Objective-C runtime: they register the class,
// instantiate NSAccessibilityElements, set properties through objc_msgSend and
// read them back with the AppKit getters. That is as far as this can be
// verified without a live window and a screen reader driving it — but it is
// the part that silently does nothing when it is wrong, so it is the part
// worth pinning down.

func TestAxRoleMapping(t *testing.T) {
	cases := []struct {
		role     string
		tappable bool
		want     string
	}{
		{"button", true, "AXButton"},
		{"checkbox", true, "AXCheckBox"},
		{"switch", true, "AXCheckBox"},
		{"radio", true, "AXRadioButton"},
		{"tab", true, "AXRadioButton"},
		{"slider", false, "AXSlider"},
		{"progressbar", false, "AXProgressIndicator"},
		{"textfield", false, "AXTextField"},
		{"heading", false, "AXHeading"},
		{"link", true, "AXLink"},
		{"img", false, "AXImage"},
		{"list", false, "AXList"},
		{"listitem", false, "AXRow"},
		{"text", false, "AXStaticText"},
		// An unknown role that can still be pressed is a button; one that
		// cannot is a group. Either beats handing VoiceOver a role string it
		// does not know, which it reports as "unknown".
		{"whatsit", true, "AXButton"},
		{"whatsit", false, "AXGroup"},
		{"", false, "AXGroup"},
	}
	for _, c := range cases {
		if got := axRole(c.role, c.tappable); got != c.want {
			t.Errorf("axRole(%q, tappable=%v) = %q, want %q", c.role, c.tappable, got, c.want)
		}
	}
}

func TestPublishable(t *testing.T) {
	if publishable(A11yNode{Role: "group"}) {
		t.Error("an unlabeled, inert node would add a silent VoiceOver stop")
	}
	if !publishable(A11yNode{Label: "Send"}) {
		t.Error("a labeled node must be published")
	}
	if !publishable(A11yNode{Tappable: true}) {
		t.Error("an actionable node must be published even without a label")
	}
	if !publishable(A11yNode{Value: "25%"}) {
		t.Error("a node with a value must be published")
	}
}

// The class has to actually register with the ObjC runtime, and it has to
// subclass NSAccessibilityElement — the whole design rests on inheriting that
// class's stored frame/role/label, because the Go callback trampoline cannot
// return the NSRect the accessibility protocol asks for.
func TestA11yElementClassRegisters(t *testing.T) {
	cls, err := a11yElementClassRef()
	if err != nil {
		t.Fatalf("registering GoGPUA11yElement: %v", err)
	}
	if cls == 0 {
		t.Fatal("class is nil")
	}
	if got := GetClass("GoGPUA11yElement"); got != cls {
		t.Errorf("GetClass returned %v, want the registered class %v", got, cls)
	}
	if GetClass("NSAccessibilityElement") == 0 {
		t.Fatal("NSAccessibilityElement missing: AppKit did not load")
	}
}

// Properties set through objc_msgSend must be readable back through AppKit's
// own getters. If the selectors are misspelled the setters are silent no-ops,
// and a screen reader reads an empty element — exactly the failure this test
// exists to catch.
func TestA11yElementCarriesItsDescription(t *testing.T) {
	n := A11yNode{
		ID: 7, Role: "button", Label: "Send message", Hint: "sends the draft",
		X: 40, Y: 20, W: 200, H: 44, Tappable: true,
	}
	el := newA11yElement(n, 100 /* view height in points */, 2 /* retina */)
	if el.IsNil() {
		t.Fatal("newA11yElement returned nil")
	}

	label := el.Send(RegisterSelector("accessibilityLabel"))
	if got := nsStringToGo(label); got != "Send message" {
		t.Errorf("accessibilityLabel = %q, want %q", got, "Send message")
	}
	role := el.Send(RegisterSelector("accessibilityRole"))
	if got := nsStringToGo(role); got != "AXButton" {
		t.Errorf("accessibilityRole = %q, want AXButton", got)
	}
	help := el.Send(RegisterSelector("accessibilityHelp"))
	if got := nsStringToGo(help); got != "sends the draft" {
		t.Errorf("accessibilityHelp = %q, want %q", got, "sends the draft")
	}
}

// A checkbox's state reaches VoiceOver as its value, not its label. An
// element that reports no value is announced identically whether it is
// checked or not.
func TestA11yCheckboxCarriesState(t *testing.T) {
	on := newA11yElement(A11yNode{
		Role: "checkbox", Label: "Remember me", Tappable: true,
		Checkable: true, Checked: true,
	}, 100, 1)
	if on.IsNil() {
		t.Fatal("nil element")
	}
	v := on.Send(RegisterSelector("accessibilityValue"))
	if v.IsNil() {
		t.Fatal("checked checkbox published no value")
	}
	if got := v.GetInt64(RegisterSelector("intValue")); got != 1 {
		t.Errorf("checked value = %d, want 1", got)
	}

	off := newA11yElement(A11yNode{
		Role: "checkbox", Label: "Remember me", Tappable: true, Checkable: true,
	}, 100, 1)
	v = off.Send(RegisterSelector("accessibilityValue"))
	if v.IsNil() {
		t.Fatal("unchecked checkbox published no value")
	}
	if got := v.GetInt64(RegisterSelector("intValue")); got != 0 {
		t.Errorf("unchecked value = %d, want 0", got)
	}
}

// nsStringToGo reads an NSString back into Go, for assertions.
//
// It goes through UTF8String rather than NSString.String, which is a stub
// that always returns "" — reading through it made every assertion here pass
// vacuously on empty and fail on everything else.
func nsStringToGo(s ID) string {
	if s.IsNil() {
		return ""
	}
	p := NSStringUTF8Ptr(s)
	if p == 0 {
		return ""
	}
	n := NSStringLength(s)
	if n == 0 {
		return ""
	}
	// Length is in UTF-16 code units; for the ASCII strings these tests use
	// it bounds the UTF-8 byte count safely. Scan to the NUL to get the real
	// end rather than trusting that bound.
	buf := unsafe.Slice((*byte)(unsafe.Pointer(p)), n*4)
	for i, b := range buf {
		if b == 0 {
			return string(buf[:i])
		}
	}
	return string(buf)
}

// The frame conversion is the other place a mistake is silent: a wrong y-flip
// puts VoiceOver's cursor somewhere the control is not, and nothing errors.
// gophics hands over top-left-origin physical pixels; an unflipped NSView
// wants bottom-left-origin points.
func TestA11yFrameFlipsIntoViewSpace(t *testing.T) {
	// A 44pt-tall control 20pt below the top of a 100pt-tall view, on a
	// retina display: its bottom edge is 100-20-44 = 36pt above the view's
	// bottom.
	el := newA11yElement(A11yNode{
		Label: "Send", Tappable: true,
		X: 40, Y: 40, W: 200, H: 88, // physical px at scale 2
	}, 100, 2)
	if el.IsNil() {
		t.Fatal("nil element")
	}
	got := el.GetRect(RegisterSelector("accessibilityFrameInParentSpace"))
	want := NSRect{Origin: NSPoint{X: 20, Y: 36}, Size: NSSize{Width: 100, Height: 44}}
	if got != want {
		t.Errorf("frame = %+v, want %+v", got, want)
	}
}

// The activation path is the riskiest single thing here: a Go function
// registered as an Objective-C method through an FFI trampoline. If the type
// encoding or the selector is wrong, AppKit calls nothing and a VoiceOver user
// simply cannot press anything — with no error anywhere.
func TestA11yPerformPressReachesGo(t *testing.T) {
	el := newA11yElement(A11yNode{ID: 42, Label: "Send", Tappable: true}, 100, 1)
	if el.IsNil() {
		t.Fatal("nil element")
	}

	got := -1
	a11yRegistry.Lock()
	a11yRegistry.nodes = map[uintptr]int{el.Ptr(): 42}
	a11yRegistry.activate = func(id int) { got = id }
	a11yRegistry.Unlock()
	t.Cleanup(func() {
		a11yRegistry.Lock()
		a11yRegistry.nodes = map[uintptr]int{}
		a11yRegistry.activate = nil
		a11yRegistry.Unlock()
	})

	ok := el.GetBool(RegisterSelector("accessibilityPerformPress"))
	if !ok {
		t.Error("accessibilityPerformPress returned NO")
	}
	if got != 42 {
		t.Errorf("activate got node %d, want 42", got)
	}
}

// An element the app never registered must not activate something else.
func TestA11yPerformPressOnUnknownElementIsInert(t *testing.T) {
	el := newA11yElement(A11yNode{Label: "Decorative"}, 100, 1)
	if el.IsNil() {
		t.Fatal("nil element")
	}
	called := false
	a11yRegistry.Lock()
	a11yRegistry.nodes = map[uintptr]int{}
	a11yRegistry.activate = func(int) { called = true }
	a11yRegistry.Unlock()
	t.Cleanup(func() {
		a11yRegistry.Lock()
		a11yRegistry.activate = nil
		a11yRegistry.Unlock()
	})

	if el.GetBool(RegisterSelector("accessibilityPerformPress")) {
		t.Error("unregistered element reported that it handled a press")
	}
	if called {
		t.Error("unregistered element activated a node")
	}
}
