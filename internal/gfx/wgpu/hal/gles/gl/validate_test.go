//go:build (windows || linux) && !(js && wasm)

package gl

import "testing"

// An unloaded context must be reported as unusable rather than accepted.
//
// This is the regression test for a crash that took a VM and a crash dump to
// diagnose: Load filled 126 function pointers and checked none of them, so a
// driver missing a name silently stored 0 and Load returned success. The first
// call was then an access violation with PC=0 and an empty stack.
func TestValidateRejectsUnloadedContext(t *testing.T) {
	var c Context
	err := c.Validate()
	if err == nil {
		t.Fatal("a context with no entry points loaded reported itself valid")
	}
	// The message has to name what is missing: this error is the only clue a
	// user gets about why their machine fell back to CPU rendering.
	for _, want := range []string{"glGenVertexArrays", "glCreateShader", "required entry points missing"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// The required set is the "cannot draw at all" list, so it must stay small and
// stay core. Extensions belong at their call sites, guarded individually.
func TestRequiredEntryPointsAreCore(t *testing.T) {
	var c Context
	req := c.requiredEntryPoints()
	if len(req) < 20 {
		t.Errorf("only %d required entry points; the list looks truncated", len(req))
	}
	for _, e := range req {
		if e.name == "" {
			t.Error("an entry point has no name, so a failure could not be reported")
		}
		// Anything version- or vendor-suffixed is optional by nature and must
		// not be required.
		for _, suffix := range []string{"ARB", "EXT", "OES", "KHR", "NV", "AMD"} {
			if len(e.name) > len(suffix) && e.name[len(e.name)-len(suffix):] == suffix {
				t.Errorf("%s is an extension entry point and must not be required", e.name)
			}
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
