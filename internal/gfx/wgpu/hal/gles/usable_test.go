//go:build (windows || linux) && !(js && wasm)

package gles

import "testing"

// TestAdapterUsable pins the version gate that decides whether a GL context is
// offered at all.
//
// This is a regression test for a crash, not a style preference. Windows hands
// back a "GDI Generic" OpenGL 1.1 context whenever there is no graphics driver
// — inside a VM, over RDP, on a fresh install. Every GL 3.0 entry point is then
// a null pointer, and the backend called one (glGenVertexArrays) in
// Adapter.Open, so a gophics app died at PC=0 with an empty stack before it
// could draw anything. Found by running the counter example in a UTM VM.
func TestAdapterUsable(t *testing.T) {
	for _, tc := range []struct {
		name   string
		caps   AdapterCapabilities
		usable bool
	}{
		{"GDI Generic 1.1 — the crash", AdapterCapabilities{GLMajor: 1, GLMinor: 1, Renderer: "GDI Generic"}, false},
		{"GL 2.1, still no VAOs", AdapterCapabilities{GLMajor: 2, GLMinor: 1}, false},
		{"GL 3.0, below our GLSL floor", AdapterCapabilities{GLMajor: 3, GLMinor: 0}, false},
		{"GL 3.3, the minimum", AdapterCapabilities{GLMajor: 3, GLMinor: 3}, true},
		{"GL 4.6", AdapterCapabilities{GLMajor: 4, GLMinor: 6}, true},
		{"ES 2.0", AdapterCapabilities{GLMajor: 2, GLMinor: 0, IsES: true}, false},
		{"ES 3.0, the ES minimum", AdapterCapabilities{GLMajor: 3, GLMinor: 0, IsES: true}, true},
		{"unparsed version reads as 0.0", AdapterCapabilities{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.caps.Usable(); got != tc.usable {
				t.Errorf("Usable() = %v, want %v", got, tc.usable)
			}
		})
	}
}

// The reason is a log line a user may have to act on, so it should name what
// was found and what is needed.
func TestUnusableReasonIsInformative(t *testing.T) {
	caps := AdapterCapabilities{GLMajor: 1, GLMinor: 1, Renderer: "GDI Generic", Version: "1.1.0"}
	got := caps.UnusableReason()
	for _, want := range []string{"GDI Generic", "1.1", "OpenGL 3.3+"} {
		if !contains(got, want) {
			t.Errorf("reason %q does not mention %q", got, want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
