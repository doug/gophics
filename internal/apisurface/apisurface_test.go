package apisurface_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/apisurface"
)

// manifest is the committed public surface: testdata, because that is what it
// is — a fixture this package's test reads and rewrites, not a document. It
// used to live under design/, which put a gate's input in a directory of
// planning prose.
const manifest = "testdata/api-surface.txt"

// The public surface is a promise, so it is a committed artifact and a change
// to it is a change to the diff.
//
// gophics commits to "a small surface and the Go 1 compatibility ethos
// after 1.0". Before this test the number in that promise could only be
// asserted — every audit re-derived it by hand, and the last one shipped with a
// wrong conclusion in it. Now a refactor that claims to shrink the surface
// shows exactly what it removed, and one that grows it by accident cannot be
// merged quietly.
//
// The diff is taken by key rather than by line, so the failure can say which
// changes are breaks: a signature that moved under a name that stayed is the
// case the manifest used to be blind to, and it is reported first.
//
// GOPHICS_UPDATE_API=1 rewrites the file, the same convention apptest uses for
// golden images.
func TestPublicSurfaceMatchesManifest(t *testing.T) {
	got, err := apisurface.Collect("../..")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) < 1000 {
		t.Fatalf("collected only %d exported identifiers; the scan is broken and "+
			"this test would pass while measuring nothing", len(got))
	}
	want := apisurface.Render(got)

	if os.Getenv("GOPHICS_UPDATE_API") != "" {
		if err := os.MkdirAll(filepath.Dir(manifest), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(manifest, []byte(want), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("wrote %s (%d identifiers)", manifest, len(got))
		return
	}

	have, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatalf("%v\n\nRun: GOPHICS_UPDATE_API=1 go test ./internal/apisurface", err)
	}
	if string(have) == want {
		t.Logf("public surface: %d identifiers", len(got))
		return
	}

	d := apisurface.Diff(apisurface.ParseManifest(string(have)), got)
	if d.Empty() {
		// Same elements, different file: the header or the tier table moved.
		t.Errorf("the manifest header changed (the surface itself did not).\n\n" +
			"    GOPHICS_UPDATE_API=1 go test ./internal/apisurface")
		return
	}
	t.Errorf("the public API surface changed.\n\n"+
		"%d added, %d removed, %d signatures changed. If that is the intent,\n"+
		"regenerate and let the diff be reviewed:\n\n"+
		"    GOPHICS_UPDATE_API=1 go test ./internal/apisurface\n\n%s",
		len(d.Added), len(d.Removed), len(d.Changed), report(d))
}

// Every element carries a signature.
//
// The manifest recorded names and kinds and nothing else for most of its life,
// which made it a gate against renames only. A regression to that would not
// fail any other test here: the file would still match the scan, because the
// scan would have stopped recording the same thing the file stopped holding.
// This asserts against the committed file, so losing signatures fails loudly.
func TestManifestRecordsSignatures(t *testing.T) {
	text, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entries := apisurface.ParseManifest(string(text))
	if len(entries) < 1000 {
		t.Fatalf("manifest holds %d entries; it is not the real one", len(entries))
	}
	var bare int
	for _, e := range entries {
		if e.Signature() == "" {
			t.Errorf("%s has no signature", e.Key())
			if bare++; bare > 10 {
				t.Fatal("… and more; the manifest has lost its signatures")
			}
		}
	}

	// A sample of shapes that must survive, one per rendering rule. These are
	// spot checks on the format, not on the API: each one is a rule in
	// Render's header that would otherwise only be enforced by review.
	for _, want := range []string{
		"paint.Color.WithAlpha method func(paint.Color, float32) paint.Color", // receiver as first param
		"widget.Text.Wrap field bool",                                         // field types
		"geom.Pt type struct",                                                 // struct underlying
		"paint.Canvas type interface",                                         // interface underlying
	} {
		if !strings.Contains(string(text), want+"\n") {
			t.Errorf("manifest is missing the line %q\n"+
				"if the API moved, fix the expectation; if the *format* moved, that is\n"+
				"the thing this test is here to make you justify", want)
		}
	}
}

// Every public package is classified, and a v1 package does not need an
// unstable one to be called.
//
// The tier table is a proposal about v1, and a proposal nobody checks rots.
// The first half fails when a new public package arrives without a decision
// about what it promises — which is the only moment the decision is cheap. The
// second half is the thing recording signatures made possible: a committed
// signature that names an uncommitted type is a promise with a hole in it, and
// before signatures were in the manifest there was no way to look for one.
func TestStabilityTiersCoverTheSurface(t *testing.T) {
	text, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	entries := apisurface.ParseManifest(string(text))

	if un := apisurface.Unclassified(entries); len(un) > 0 {
		t.Errorf("public packages with no stability tier: %v\n"+
			"classify them in internal/apisurface/tiers.go and say why in README.md",
			un)
	}
	if leaks := apisurface.Leaks(entries); len(leaks) > 0 {
		t.Errorf("v1 signatures a v1 caller cannot commit to:\n    %s\n"+
			"either move the type, change the tier, or record it in tierExceptions "+
			"with a reason", strings.Join(leaks, "\n    "))
	}
	if stale := apisurface.StaleExceptions(entries); len(stale) > 0 {
		t.Errorf("tierExceptions entries that no longer describe anything:\n    %s\n"+
			"delete them", strings.Join(stale, "\n    "))
	}
}

// The blind spot that produced a wrong conclusion, closed.
//
// The 2026-08-24 audit reported "nothing imports intl" and internalizing it was
// on the plan. examples/tally imports it and calls intl.Auto() — which reads
// LC_ALL and LANG, environment variables that do not exist on Android or iOS,
// where tally ships. The audit missed it because tally is a separate module and
// invisible to `go list ./...` from the root.
//
// Any future "no caller anywhere" claim has to survive this test.
func TestSeparateModulesAreVisibleToTheAudit(t *testing.T) {
	consumers, err := apisurface.Consumers("../..", "../../examples/tally")
	if err != nil {
		t.Fatal(err)
	}
	tallyUsers, ok := consumers["intl"]
	if !ok {
		t.Fatal("no consumer found for intl; the separate-module scan is not working, " +
			"which is exactly the hole that let 'nothing imports intl' be published")
	}
	var fromTally bool
	for _, c := range tallyUsers {
		if strings.Contains(c, "tally") {
			fromTally = true
		}
	}
	if !fromTally {
		t.Errorf("intl consumers = %v; expected one in the tally module", tallyUsers)
	}
}

// report renders a delta for a failure message: breaks first, because that is
// the decision the reader has to make, and a capped sample of the rest.
func report(d apisurface.Delta) string {
	var b strings.Builder
	show := func(label string, xs []string) {
		if len(xs) == 0 {
			return
		}
		b.WriteString(label + "\n")
		for i, x := range xs {
			if i == 15 {
				b.WriteString("    … and " + itoa(len(xs)-15) + " more\n")
				break
			}
			b.WriteString("    " + x + "\n")
		}
	}
	show("BREAKING:", d.Breaking())
	var added []string
	for _, e := range d.Added {
		if e.Kind() != "ifacemethod" { // already listed as breaking
			added = append(added, string(e))
		}
	}
	show("added (additive):", added)
	return b.String()
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var d []byte
	for n > 0 {
		d = append([]byte{byte('0' + n%10)}, d...)
		n /= 10
	}
	return string(d)
}
