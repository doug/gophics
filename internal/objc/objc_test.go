//go:build darwin && !ios

package objc

import "testing"

func TestInitLoadsRuntime(t *testing.T) {
	if err := Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
}

func TestClassAndSelector(t *testing.T) {
	if c := Class("NSString"); !c.Valid() {
		t.Error("NSString class not found")
	}
	if c := Class("NSObject"); !c.Valid() {
		t.Error("NSObject class not found")
	}
	if c := Class("NoSuchClassZZZ"); c.Valid() {
		t.Error("bogus class name resolved to non-nil")
	}
	s1, s2 := Sel("description"), Sel("description")
	if s1 == 0 {
		t.Error("selector not registered")
	}
	if s1 != s2 {
		t.Error("selector cache returned different values for the same name")
	}
}

// TestStringRoundTrip is the real proof the message send works end to end: build
// an NSString from Go bytes, ask Objective-C for its UTF8String, and read it back.
func TestStringRoundTrip(t *testing.T) {
	for _, want := range []string{
		"hello",
		"Assets:US:BofA:Checking",
		"unicode: héllo — ✓ 日本語",
		"",
	} {
		s := String(want)
		if want != "" && !s.Valid() {
			t.Fatalf("String(%q) returned nil", want)
		}
		if got := GoString(s); got != want {
			t.Errorf("round trip: got %q, want %q", got, want)
		}
	}
}

// TestStringLength cross-checks a real Objective-C computed property, so we know
// we're talking to Foundation rather than echoing our own bytes back.
func TestStringLength(t *testing.T) {
	s := String("abcde")
	if n := s.SendUInt("length"); n != 5 {
		t.Errorf("[@\"abcde\" length] = %d, want 5", n)
	}
	if !s.SendBool("isEqualToString:", Obj(String("abcde"))) {
		t.Error("isEqualToString: said equal strings differ")
	}
	if s.SendBool("isEqualToString:", Obj(String("other"))) {
		t.Error("isEqualToString: said different strings match")
	}
}

// TestArrayBridge covers NewArray/Array, which the file picker uses to read the
// panel's selected URLs.
func TestArrayBridge(t *testing.T) {
	arr := NewArray(String("one"), String("two"), String("three"))
	if !arr.Valid() {
		t.Fatal("NewArray returned nil")
	}
	if n := arr.SendUInt("count"); n != 3 {
		t.Fatalf("count = %d, want 3", n)
	}
	got := make([]string, 0, 3)
	for _, o := range Array(arr) {
		got = append(got, GoString(o))
	}
	if len(got) != 3 || got[0] != "one" || got[1] != "two" || got[2] != "three" {
		t.Errorf("Array round trip = %v", got)
	}
	if Array(0) != nil {
		t.Error("Array(nil) should be nil")
	}
}

// TestNilSafety pins the contract that messaging nil is a no-op returning zero,
// so a missing class or failed alloc can't panic a capability implementation.
func TestNilSafety(t *testing.T) {
	var nilObj ID
	if got := nilObj.Send("description"); got.Valid() {
		t.Error("Send on nil returned non-nil")
	}
	nilObj.SendVoid("description")
	if nilObj.SendUInt("length") != 0 {
		t.Error("SendUInt on nil should be 0")
	}
	if nilObj.SendBool("boolValue") {
		t.Error("SendBool on nil should be false")
	}
	if GoString(0) != "" {
		t.Error("GoString(nil) should be empty")
	}
}

// TestLoadFramework loads AppKit and resolves a class that only exists there,
// proving the framework-loading path the file picker depends on.
func TestLoadFramework(t *testing.T) {
	if err := LoadFramework("AppKit"); err != nil {
		t.Fatalf("LoadFramework(AppKit): %v", err)
	}
	if c := Class("NSOpenPanel"); !c.Valid() {
		t.Error("NSOpenPanel not resolvable after loading AppKit")
	}
	if err := LoadFramework("AppKit"); err != nil {
		t.Errorf("second LoadFramework should be a no-op, got %v", err)
	}
}
