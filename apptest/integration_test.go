package apptest_test

import (
	"os"
	"strconv"
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// counter is the smallest widget that has state worth asserting on: a label
// showing a number and a button that increments it.
type counter struct{}

func (counter) CreateState() widget.State { return &counterState{} }

type counterState struct {
	widget.StateBase[counter]
	n int
}

func (s *counterState) Build(widget.Ctx) widget.Widget {
	return widget.Column(
		theme.Label("count "+strconv.Itoa(s.n)),
		theme.Button{
			Label: "Increment",
			OnTap: func() { s.SetState(func() { s.n++ }) },
		},
	)
}

// The whole point of the package, exercised the way a user would: build an app,
// drive it by the labels a screen reader would see, and assert on the result
// without a display anywhere in sight.
func TestDriveAppByLabel(t *testing.T) {
	a := apptest.New(t, counter{}, apptest.Size(200, 120))

	a.AssertLabel("Increment")
	if !a.HasLabel("count 0") {
		t.Fatalf("expected the initial count. Labels: %v", a.Labels())
	}

	a.TapLabel("Increment")
	if !a.HasLabel("count 1") {
		t.Errorf("tapping by label did not increment. Labels: %v", a.Labels())
	}

	a.TapLabel("Increment")
	a.TapLabel("Increment")
	if !a.HasLabel("count 3") {
		t.Errorf("after three taps. Labels: %v", a.Labels())
	}
}

// MustNode's failure lists the labels that do exist. That matters because
// "not found" alone never says whether the label is wrong or the widget never
// rendered at all, which are different bugs.
func TestLabelsAreDiscoverable(t *testing.T) {
	a := apptest.New(t, counter{}, apptest.Size(200, 120))

	labels := a.Labels()
	if len(labels) == 0 {
		t.Fatal("no labels — a widget with a button and text produced no semantics")
	}

	n := a.MustNode("Increment")
	if n.Rect.Max.X <= n.Rect.Min.X || n.Rect.Max.Y <= n.Rect.Min.Y {
		t.Errorf("Increment has an empty rect %+v — TapLabel would tap nothing", n.Rect)
	}
}

func TestAssertNoLabel(t *testing.T) {
	a := apptest.New(t, counter{}, apptest.Size(200, 120))
	a.AssertNoLabel("Decrement")
}

// Golden's full cycle from a caller's side: create it, then match against it.
// The temp dir keeps this from depending on a committed fixture, so the test
// proves the mechanism rather than the contents of some PNG.
func TestGoldenCreateThenMatch(t *testing.T) {
	dir := t.TempDir()

	t.Setenv(apptest.UpdateEnv, "1")
	create := apptest.New(t, counter{}, apptest.Size(120, 80), apptest.Dir(dir))
	create.Golden("counter")

	if _, err := os.Stat(dir + "/counter.png"); err != nil {
		t.Fatalf("update mode wrote no golden: %v", err)
	}

	os.Unsetenv(apptest.UpdateEnv)
	match := apptest.New(t, counter{}, apptest.Size(120, 80), apptest.Dir(dir))
	match.Golden("counter") // fails the test if it does not match

	// A different state must not match the golden of the first. If it did,
	// Golden would be incapable of catching a visual regression.
	changed := apptest.New(t, counter{}, apptest.Size(120, 80), apptest.Dir(dir))
	changed.TapLabel("Increment")

	got := toRGBAish(changed)
	want := toRGBAish(match)
	if imagesEqual(got, want) {
		t.Error("the frame after a tap is pixel-identical to the frame before — " +
			"Golden could not detect a change here")
	}
}

// Scale renders at a device pixel ratio, so the same logical size produces a
// larger image. A test asserting retina output depends on this.
func TestScaleChangesPixelSize(t *testing.T) {
	one := apptest.New(t, counter{}, apptest.Size(100, 50), apptest.Scale(1))
	two := apptest.New(t, counter{}, apptest.Size(100, 50), apptest.Scale(2))

	b1 := one.Render().Bounds()
	b2 := two.Render().Bounds()

	if b2.Dx() != b1.Dx()*2 || b2.Dy() != b1.Dy()*2 {
		t.Errorf("scale 2 produced %dx%d, want double %dx%d",
			b2.Dx(), b2.Dy(), b1.Dx(), b1.Dy())
	}
}

func toRGBAish(a *apptest.App) []byte {
	img := a.Render()
	b := img.Bounds()
	out := make([]byte, 0, b.Dx()*b.Dy()*4)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, al := img.At(x, y).RGBA()
			out = append(out, byte(r>>8), byte(g>>8), byte(bl>>8), byte(al>>8))
		}
	}
	return out
}

func imagesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// nested buries the button several containers deep, which is what a real tree
// looks like. SemNode carries Children, so helpers that walk only the roots
// find nothing here — the first version of this package had exactly that bug,
// and the flat counter above was too shallow to expose it.
type nested struct{}

func (nested) CreateState() widget.State { return &nestedState{} }

type nestedState struct {
	widget.StateBase[nested]
	tapped bool
}

func (s *nestedState) Build(widget.Ctx) widget.Widget {
	return widget.Column(
		widget.Padding{All: 4, Child: widget.Column(
			widget.Padding{All: 4, Child: widget.Column(
				theme.Button{
					Label: "Deep",
					OnTap: func() { s.SetState(func() { s.tapped = true }) },
				},
			)},
		)},
	)
}

func TestFindsDeeplyNestedWidgets(t *testing.T) {
	a := apptest.New(t, nested{}, apptest.Size(200, 120))

	if !a.HasLabel("Deep") {
		t.Fatalf("a button nested in containers was not found — semantics is a "+
			"tree and the helpers must flatten it. Labels seen: %v", a.Labels())
	}

	a.TapLabel("Deep")
	if !a.HasLabel("Deep") {
		t.Error("button vanished after tapping it")
	}
}

// A semantic node that contains another semantic node is the case flattening
// exists for: CollectSemantics keeps structural children ("buttons inside a
// group") nested rather than hoisting them. Walking only the roots misses them.
func TestNodesFlattensNestedSemantics(t *testing.T) {
	// A Tappable wrapping a Button: both carry semantics, so the Button
	// becomes a child of the Tappable rather than a sibling.
	a := apptest.New(t, theme.Tappable{
		OnTap: func() {},
		Child: widget.Column(theme.Button{Label: "Inner", OnTap: func() {}}),
	}, apptest.Size(200, 120))

	flat := a.Nodes()      // renders, then flattens
	roots := a.Semantics() // same frame, roots only

	nestedChildren := 0
	for _, r := range roots {
		nestedChildren += len(r.Children)
	}
	if nestedChildren == 0 {
		t.Skip("this tree did not nest; nothing to flatten here")
	}

	if len(flat) <= len(roots) {
		t.Errorf("Nodes() returned %d and there are %d roots holding %d children — "+
			"the tree is not being flattened", len(flat), len(roots), nestedChildren)
	}
}

// Exact and substring matching are different tools: a label assembled from
// pieces is found by substring, and a label that must not drift is asserted
// exactly. Both must work, and neither should quietly do the other's job.
func TestExactAndSubstringMatching(t *testing.T) {
	a := apptest.New(t, counter{}, apptest.Size(200, 120))

	if !a.HasLabel("count 0") {
		t.Fatalf("exact match failed. Labels: %v", a.Labels())
	}
	if a.HasLabel("count") {
		t.Error("HasLabel matched a prefix — it must be exact")
	}
	if !a.HasText("count") {
		t.Error("HasText did not match a substring")
	}

	a.TapText("Increm") // partial label
	if !a.HasLabel("count 1") {
		t.Errorf("TapText did not tap the button. Labels: %v", a.Labels())
	}
}
