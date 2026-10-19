package apisurface_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/apisurface"
)

// The extractor is tested against source it can actually break.
//
// A test that only asserts things about the committed manifest cannot show
// that a *break* is detected: the manifest and the scan are regenerated
// together, so a hole in the extractor appears in both and they agree. The
// only way to prove the gate works is to change a signature and watch it get
// reported — so this builds a throwaway module, scans it, edits one
// declaration, scans again, and insists on what the diff says.
//
// Every case here is a break that passed the old name-and-kind manifest
// untouched, together with one additive change as a control: if the control
// ever reports as breaking, the gate has become the kind that cries wolf and
// gets switched off.
const surfaceBase = `package api

// Opt is a struct whose fields are API.
type Opt struct {
	Name  string
	Count int
}

// Writer is an interface somebody outside this module implements.
type Writer interface {
	Write([]byte) (int, error)
}

// Mode's value is part of its contract.
const Mode = 3

// Draw is a plain function.
func Draw(x int) error { return nil }

// Canvas has a method with a value receiver.
type Canvas struct{ Scale float32 }

func (c Canvas) Clear(alpha float32) {}

// Depth is a defined type over a builtin.
type Depth int
`

func TestSignatureChangesAreDetected(t *testing.T) {
	dir := t.TempDir()
	write := func(src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "api.go"), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	// The module path pretends to be part of gophics so that the rendering
	// under test is the one the real manifest uses: a module-relative package
	// qualifier. The directory is a temp dir and never exists in the repo.
	gomod := "module github.com/doug/gophics/surfacetest\n\ngo 1.27\n"
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte(gomod), 0o644); err != nil {
		t.Fatal(err)
	}

	scan := func() []apisurface.Entry {
		t.Helper()
		got, err := apisurface.CollectTarget(dir, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 0 {
			t.Fatal("scanned the throwaway module and found nothing; the test is " +
				"measuring the empty set and would pass whatever the extractor did")
		}
		return got
	}

	write(surfaceBase)
	base := scan()

	// Sanity: the baseline itself has to contain signatures, or every case
	// below would "pass" by comparing nothing to nothing.
	mustHave(t, base,
		"surfacetest.Draw func func(int) error",
		"surfacetest.Opt.Count field int",
		"surfacetest.Writer.Write ifacemethod func([]byte) (int, error)",
		"surfacetest.Canvas.Clear method func(surfacetest.Canvas, float32)",
		"surfacetest.Mode const untyped int = 3",
	)

	cases := []struct {
		name string
		// edit is applied to surfaceBase as a single replacement.
		old, new string
		// what the diff must say
		wantChanged  []string // keys whose signature moved
		wantAdded    []string // whole lines
		wantRemoved  int
		wantBreaking bool
	}{{
		name:         "parameter type changed",
		old:          "func Draw(x int) error",
		new:          "func Draw(x int64) error",
		wantChanged:  []string{"surfacetest.Draw"},
		wantBreaking: true,
	}, {
		name:         "result added",
		old:          "func Draw(x int) error",
		new:          "func Draw(x int) (error, bool)",
		wantChanged:  []string{"surfacetest.Draw"},
		wantBreaking: true,
	}, {
		name:         "struct field type changed",
		old:          "Count int",
		new:          "Count int64",
		wantChanged:  []string{"surfacetest.Opt.Count"},
		wantBreaking: true,
	}, {
		name:         "interface method added",
		old:          "Write([]byte) (int, error)",
		new:          "Write([]byte) (int, error)\n\tClose() error",
		wantAdded:    []string{"surfacetest.Writer.Close ifacemethod func() error"},
		wantBreaking: true,
	}, {
		name:         "interface method signature changed",
		old:          "Write([]byte) (int, error)",
		new:          "Write([]byte) (int64, error)",
		wantChanged:  []string{"surfacetest.Writer.Write"},
		wantBreaking: true,
	}, {
		name:         "method moved to a pointer receiver",
		old:          "func (c Canvas) Clear",
		new:          "func (c *Canvas) Clear",
		wantChanged:  []string{"surfacetest.Canvas.Clear"},
		wantBreaking: true,
	}, {
		name:         "constant value changed",
		old:          "const Mode = 3",
		new:          "const Mode = 4",
		wantChanged:  []string{"surfacetest.Mode"},
		wantBreaking: true,
	}, {
		name:         "a defined type's underlying type changed",
		old:          "type Depth int",
		new:          "type Depth int64",
		wantChanged:  []string{"surfacetest.Depth"},
		wantBreaking: true,
	}, {
		// The control. A new struct field is additive: no existing caller
		// wrote it, so none can break on it.
		name:         "struct field added",
		old:          "Count int",
		new:          "Count int\n\tLabel string",
		wantAdded:    []string{"surfacetest.Opt.Label field string"},
		wantBreaking: false,
	}, {
		// The other control: a new function is additive too.
		name:         "function added",
		old:          "func Draw(x int) error",
		new:          "func Draw(x int) error\n\nfunc Erase() {}",
		wantAdded:    []string{"surfacetest.Erase func func()"},
		wantBreaking: false,
	}}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if !strings.Contains(surfaceBase, tc.old) {
				t.Fatalf("the base source does not contain %q, so this case edits nothing", tc.old)
			}
			write(strings.Replace(surfaceBase, tc.old, tc.new, 1))
			d := apisurface.Diff(base, scan())

			var changed []string
			for _, c := range d.Changed {
				changed = append(changed, c.Key)
			}
			if !sameSet(changed, tc.wantChanged) {
				t.Errorf("signature changes = %v, want %v\n%s", changed, tc.wantChanged, detail(d))
			}
			var added []string
			for _, e := range d.Added {
				added = append(added, string(e))
			}
			if !sameSet(added, tc.wantAdded) {
				t.Errorf("added = %v, want %v\n%s", added, tc.wantAdded, detail(d))
			}
			if len(d.Removed) != tc.wantRemoved {
				t.Errorf("removed = %v, want %d\n%s", d.Removed, tc.wantRemoved, detail(d))
			}
			if got := len(d.Breaking()) > 0; got != tc.wantBreaking {
				t.Errorf("Breaking() non-empty = %v, want %v; Breaking() = %v",
					got, tc.wantBreaking, d.Breaking())
			}
		})
	}
}

// Removing a name is still caught, and still reads as a removal rather than as
// a signature change — Diff matches on keys, so a key that is gone must not be
// paired with some other key's line.
func TestRemovalIsReportedAsRemoval(t *testing.T) {
	have := []apisurface.Entry{
		"pkg.A func func(int)",
		"pkg.B func func(string)",
	}
	want := []apisurface.Entry{
		"pkg.B func func(string)",
	}
	d := apisurface.Diff(have, want)
	if len(d.Removed) != 1 || d.Removed[0].Key() != "pkg.A" {
		t.Errorf("Removed = %v, want [pkg.A]", d.Removed)
	}
	if len(d.Changed) != 0 || len(d.Added) != 0 {
		t.Errorf("Changed = %v, Added = %v, want neither", d.Changed, d.Added)
	}
	if len(d.Breaking()) != 1 {
		t.Errorf("Breaking() = %v, want one entry", d.Breaking())
	}
}

// Entry's three fields have to split the same way for every kind, because Diff
// keys on two of them and the tier leak check reads the third. A signature
// with spaces in it (results, constant values) is the case that would break a
// naive split.
func TestEntryFieldsSplit(t *testing.T) {
	for _, tc := range []struct{ line, key, kind, sig string }{
		{"geom.Pt type struct", "geom.Pt", "type", "struct"},
		{"geom.Pt.X field float32", "geom.Pt.X", "field", "float32"},
		{"theme.FontBold const untyped string = \"bold\"", "theme.FontBold", "const", `untyped string = "bold"`},
		{"app.Run func func(app.Config, widget.Widget) error", "app.Run", "func", "func(app.Config, widget.Widget) error"},
		{"paint.Canvas.TextIn ifacemethod func(string, string, geom.Pt, float32, paint.Color)",
			"paint.Canvas.TextIn", "ifacemethod", "func(string, string, geom.Pt, float32, paint.Color)"},
		{"shell.Window type interface", "shell.Window", "type", "interface"},
	} {
		e := apisurface.Entry(tc.line)
		if e.Key() != tc.key || e.Kind() != tc.kind || e.Signature() != tc.sig {
			t.Errorf("%q split as (%q, %q, %q), want (%q, %q, %q)",
				tc.line, e.Key(), e.Kind(), e.Signature(), tc.key, tc.kind, tc.sig)
		}
	}
}

func TestEntryPackage(t *testing.T) {
	for line, want := range map[string]string{
		"geom.Pt.X field float32":                     "geom",
		"shell/desktop.Run func func()":               "shell/desktop",
		"sound/mp3.Decoder type struct":               "sound/mp3",
		"tools/uitrace/trace.V const untyped int = 1": "tools/uitrace/trace",
	} {
		if got := apisurface.Entry(line).Package(); got != want {
			t.Errorf("Entry(%q).Package() = %q, want %q", line, got, want)
		}
	}
}

// The tier checks have to be able to report something, or the test over the
// real tree is a test that passes because it looked at nothing.
//
// Both halves are asserted against hand-written entries rather than the
// manifest: the manifest is clean today, which is the outcome we want and the
// worst possible evidence that the check works.
func TestTierChecksReportWhatTheyShould(t *testing.T) {
	// widget is v1, chart is unstable: a committed function that takes a
	// chart type cannot be called without an uncommitted one.
	leaky := apisurface.Entry("widget.Plot func func(chart.Mark, geom.Size) error")
	if got := apisurface.Leaks([]apisurface.Entry{leaky}); len(got) != 1 {
		t.Errorf("Leaks(%q) = %v, want one finding naming chart.Mark", leaky, got)
	}
	// The control: geom is v1 too, so this one is fine.
	clean := apisurface.Entry("widget.Plot func func(geom.Size) error")
	if got := apisurface.Leaks([]apisurface.Entry{clean}); len(got) != 0 {
		t.Errorf("Leaks(%q) = %v, want nothing", clean, got)
	}
	// A type from outside the module is not the tiers' business.
	foreign := apisurface.Entry("widget.Plot func func(time.Duration) error")
	if got := apisurface.Leaks([]apisurface.Entry{foreign}); len(got) != 0 {
		t.Errorf("Leaks(%q) = %v, want nothing", foreign, got)
	}

	// An internal type in a committed signature is the other half of the
	// check: it is callable, and nobody outside the module can name it.
	opaque := apisurface.Entry("app.Thing method func(*app.Thing) *internal/scene.List")
	if got := apisurface.Leaks([]apisurface.Entry{opaque}); len(got) != 1 {
		t.Errorf("Leaks(%q) = %v, want one finding naming internal/scene.List", opaque, got)
	}

	// The allowlist prunes itself: given a surface that holds none of the
	// excused keys, every one of them is reported as stale.
	if got := apisurface.StaleExceptions(nil); len(got) == 0 {
		t.Error("StaleExceptions(nil) found nothing; either the exception list is " +
			"empty — then delete this — or the pruning check does not work")
	}

	newPkg := apisurface.Entry("telemetry.Start func func() error")
	if got := apisurface.Unclassified([]apisurface.Entry{newPkg, clean}); len(got) != 1 || got[0] != "telemetry" {
		t.Errorf("Unclassified = %v, want [telemetry]", got)
	}
}

func mustHave(t *testing.T, entries []apisurface.Entry, want ...string) {
	t.Helper()
	have := map[string]bool{}
	for _, e := range entries {
		have[string(e)] = true
	}
	for _, w := range want {
		if !have[w] {
			t.Errorf("baseline scan is missing %q\nscanned:\n    %s", w, join(entries))
		}
	}
}

func join(entries []apisurface.Entry) string {
	var ss []string
	for _, e := range entries {
		ss = append(ss, string(e))
	}
	return strings.Join(ss, "\n    ")
}

func sameSet(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	have := map[string]int{}
	for _, g := range got {
		have[g]++
	}
	for _, w := range want {
		have[w]--
		if have[w] < 0 {
			return false
		}
	}
	return true
}

func detail(d apisurface.Delta) string {
	var b strings.Builder
	for _, c := range d.Changed {
		b.WriteString("  changed " + c.String() + "\n")
	}
	for _, e := range d.Added {
		b.WriteString("  added   " + string(e) + "\n")
	}
	for _, e := range d.Removed {
		b.WriteString("  removed " + string(e) + "\n")
	}
	return b.String()
}
