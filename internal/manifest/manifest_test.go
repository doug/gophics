package manifest

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// capabilitiesFromInterfaces re-derives the capability set the same way capgen
// does: every <X>Window interface in this package whose methods are all
// zero-argument single-result getters.
//
// Re-derived rather than read from widget/capabilities_gen.go so this test
// depends on the source of truth, not on generated output that could itself be
// stale.
func capabilitiesFromInterfaces(t *testing.T) []string {
	t.Helper()
	files, err := filepath.Glob("../../shell/*.go")
	if err != nil {
		t.Fatal(err)
	}
	var caps []string
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, f, src, 0)
		if err != nil {
			continue // a build-tagged file for another platform
		}
		ast.Inspect(file, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			name := ts.Name.Name
			if name == "Window" || !strings.HasSuffix(name, "Window") {
				return true
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, m := range it.Methods.List {
				fn, ok := m.Type.(*ast.FuncType)
				if !ok || len(m.Names) != 1 {
					continue
				}
				if fn.Params.NumFields() != 0 || fn.Results.NumFields() != 1 {
					continue
				}
				caps = append(caps, m.Names[0].Name)
			}
			return true
		})
	}
	sort.Strings(caps)
	return caps
}

// Every capability must have an entry, even an empty one.
//
// Absent and "needs nothing" are different facts that look identical from
// outside, and only one of them is a decision. A capability added without one
// declares no permissions on any platform and fails on a device, which is the
// exact drift this table exists to prevent.
func TestEveryCapabilityHasAPermissionEntry(t *testing.T) {
	caps := capabilitiesFromInterfaces(t)
	if len(caps) < 20 {
		t.Fatalf("derived only %d capabilities from the shell interfaces; the "+
			"scan is broken and this test would pass without checking anything",
			len(caps))
	}

	for _, c := range caps {
		if _, ok := For(c); !ok {
			t.Errorf("capability %q has no entry in the permission table — add "+
				"one, empty if it needs nothing, so \"needs nothing\" is a "+
				"decision rather than an omission", c)
		}
	}
}

// And nothing may linger in the table for a capability that no longer exists,
// or the build declares permissions for something the app cannot reach.
func TestPermissionTableHasNoStaleEntries(t *testing.T) {
	live := map[string]bool{}
	for _, c := range capabilitiesFromInterfaces(t) {
		live[c] = true
	}
	for _, c := range KnownCapabilities() {
		if !live[c] {
			t.Errorf("the permission table names %q, which is not a capability "+
				"any shell interface publishes", c)
		}
	}
}

// A capability that asks the user at runtime must say so. Android's dangerous
// permissions and every iOS usage key need a prompt, and an app that only
// declares them gets a silent denial rather than an error.
func TestRuntimeRequestMarkedForPromptingPermissions(t *testing.T) {
	for _, c := range []string{"Camera", "CameraPreview", "Microphone", "Geolocation", "Notifier"} {
		p, ok := For(c)
		if !ok {
			t.Fatalf("%s missing from the table", c)
		}
		if !p.RuntimeRequest {
			t.Errorf("%s prompts the user on at least one platform but is not "+
				"marked RuntimeRequest, so nothing tells an app it must ask", c)
		}
	}
	// The inverse: something that never prompts must not claim it does, or the
	// build nags about a request that cannot be made.
	for _, c := range []string{"Haptic", "Connectivity", "Socket"} {
		p, _ := For(c)
		if p.RuntimeRequest {
			t.Errorf("%s is an install-time permission but is marked RuntimeRequest", c)
		}
	}
}

// Every iOS key must be a usage-description key, because that is the only kind
// the loader enforces and the only kind that needs app-supplied prose.
func TestIOSKeysAreUsageDescriptions(t *testing.T) {
	for _, c := range KnownCapabilities() {
		p, _ := For(c)
		for _, k := range p.IOSKeys {
			if !strings.HasPrefix(k, "NS") || !strings.HasSuffix(k, "UsageDescription") {
				t.Errorf("%s declares iOS key %q, which is not an "+
					"NS…UsageDescription; the build asks for prose for these", c, k)
			}
		}
	}
}

// Merging is what the build actually calls: two capabilities that need the same
// permission must produce it once, sorted, so the generated manifest is stable
// and a rebuild does not churn the file.
func TestMergeDeduplicatesAndSorts(t *testing.T) {
	got := Merge([]string{"CameraPreview", "Camera", "Microphone"})

	want := []string{"android.permission.CAMERA", "android.permission.RECORD_AUDIO"}
	if len(got.Android) != len(want) {
		t.Fatalf("android = %v, want %v", got.Android, want)
	}
	for i := range want {
		if got.Android[i] != want[i] {
			t.Errorf("android[%d] = %q, want %q", i, got.Android[i], want[i])
		}
	}
	if !got.RuntimeRequest {
		t.Error("merging capabilities that prompt lost RuntimeRequest")
	}
}

// An unknown capability is ignored rather than fatal: the caller decides how
// loudly to complain, and a merge that panicked would take the build down over
// a name it could simply skip.
func TestMergeIgnoresUnknown(t *testing.T) {
	got := Merge([]string{"NotACapability"})
	if len(got.Android) != 0 || len(got.IOSKeys) != 0 {
		t.Errorf("an unknown capability produced permissions: %+v", got)
	}
}

// Networking is a baseline, not a capability, and the test records why: an
// import-graph check would be true for every app, because the core widget
// package imports net/http for NetworkImage.
func TestBaselineManifestPermissionCoversNetwork(t *testing.T) {
	found := false
	for _, a := range Baseline.Android {
		if a == "android.permission.INTERNET" {
			found = true
		}
	}
	if !found {
		t.Error("the baseline does not include INTERNET, so an app that uses " +
			"NetworkImage installs and cannot resolve a hostname")
	}
	// Merging it alongside a capability that also wants INTERNET must not
	// duplicate it, or the manifest grows a second identical line each build.
	got := Merge([]string{"Socket"}, Baseline)
	n := 0
	for _, a := range got.Android {
		if a == "android.permission.INTERNET" {
			n++
		}
	}
	if n != 1 {
		t.Errorf("INTERNET appears %d times after merging, want 1", n)
	}
}
