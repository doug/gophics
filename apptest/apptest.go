// Package apptest drives a gophics UI in a test and asserts on what it
// produced — pixels, or the semantics tree.
//
// A UI you can test with `go test` and no display is the point of an
// own-rendering toolkit, and app.Headless already provides the hard half: it
// lays out, paints, and takes input without a window. What it returns is an
// image.Image, which leaves every caller to write the same golden-file
// plumbing — capture, compare with some tolerance, decide how to update, and
// produce something legible when it fails. This package is that half.
//
// A test reads:
//
//	func TestCounter(t *testing.T) {
//	    a := apptest.New(t, Counter{})
//	    a.Golden("counter-initial")
//
//	    a.TapLabel("Increment")
//	    a.Golden("counter-after-tap")
//	}
//
// The App embeds *app.Headless, so every input and inspection method on it —
// Tap, Type, Key, Drag, Scroll, Resize, SetDarkMode, Semantics — is available
// directly. This package adds the assertions.
package apptest

import (
	"fmt"
	"strings"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/widget"
)

// App is a headless application under test.
type App struct {
	*app.Headless

	tb   testing.TB
	opts Options
}

// Options configures the harness. Use the Option helpers rather than building
// this directly; the zero value is filled in with defaults by New.
type Options struct {
	// Config is passed through to app.NewHeadless. Size is taken from here.
	Config app.Config
	// Scale is the device pixel ratio. Default 1.
	Scale float32
	// Dir holds golden images. Default "testdata".
	Dir string
	// Tol is how much difference Golden accepts. Default Exact.
	Tol Tolerance
}

// Option customises the harness.
type Option func(*Options)

// Size sets the logical window size. Default 320x240.
func Size(w, h float32) Option {
	return func(o *Options) { o.Config.Size = geom.Size{W: w, H: h} }
}

// Scale sets the device pixel ratio, so a test can assert on the 2x rendering
// that a retina display would produce. Default 1.
func Scale(s float32) Option {
	return func(o *Options) { o.Scale = s }
}

// Dir sets the directory holding golden images. Default "testdata".
func Dir(d string) Option {
	return func(o *Options) { o.Dir = d }
}

// Tol sets how much difference Golden tolerates. Default Exact.
func Tol(t Tolerance) Option {
	return func(o *Options) { o.Tol = t }
}

// WithConfig sets the app.Config, for tests that need a title, an AppID, or a
// particular background. Size set here is used unless Size() overrides it.
func WithConfig(c app.Config) Option {
	return func(o *Options) { o.Config = c }
}

// New builds a headless app around root and fails the test if it cannot start.
func New(tb testing.TB, root widget.Widget, opts ...Option) *App {
	tb.Helper()

	o := Options{
		Config: app.Config{Size: geom.Size{W: 320, H: 240}},
		Scale:  1,
		Dir:    "testdata",
	}
	for _, fn := range opts {
		fn(&o)
	}
	if o.Config.Size.W <= 0 || o.Config.Size.H <= 0 {
		o.Config.Size = geom.Size{W: 320, H: 240}
	}
	if o.Scale <= 0 {
		o.Scale = 1
	}

	h, err := app.NewHeadless(root, o.Config, o.Scale)
	if err != nil {
		tb.Fatalf("apptest: starting the app: %v", err)
	}
	return &App{Headless: h, tb: tb, opts: o}
}

// Nodes returns every semantics node in the tree, flattened, in tree order.
//
// Semantics is what assistive technology sees, so asserting on it checks the
// thing a screen-reader user gets rather than an implementation detail — and
// unlike pixel coordinates it survives restyling.
//
// The flattening is the whole point: SemNode carries Children, so a widget
// nested inside any container is invisible to code that walks only the roots.
// That is most widgets in a real tree.
func (a *App) Nodes() []layout.SemNode {
	a.Render() // semantics are produced during layout
	return layout.FlattenSemantics(a.Semantics())
}

// Node returns the first semantics node whose Label equals label, or nil.
func (a *App) Node(label string) *layout.SemNode {
	a.tb.Helper()
	nodes := a.Nodes()
	for i := range nodes {
		if nodes[i].Label == label {
			return &nodes[i]
		}
	}
	return nil
}

// MustNode returns the node labelled label, failing the test if there is none.
// The failure lists the labels that do exist, because "not found" on its own
// rarely says whether the label is wrong or the widget never rendered.
func (a *App) MustNode(label string) layout.SemNode {
	a.tb.Helper()
	if n := a.Node(label); n != nil {
		return *n
	}
	a.tb.Fatalf("apptest: no node labelled %q. Present labels: %s",
		label, formatLabels(a.Labels()))
	return layout.SemNode{}
}

// Labels lists every non-empty semantics label in the tree, in tree order.
func (a *App) Labels() []string {
	var out []string
	for _, n := range a.Nodes() {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

// Role returns the first node with this role, or nil. Useful where a widget
// has no stable label — the only button on a screen, say.
func (a *App) Role(r layout.Role) *layout.SemNode {
	a.tb.Helper()
	nodes := a.Nodes()
	for i := range nodes {
		if nodes[i].Role == r {
			return &nodes[i]
		}
	}
	return nil
}

// TapLabel taps the centre of the widget labelled label.
//
// Tapping by label rather than by coordinate keeps a test readable and stops it
// breaking every time the layout moves. It fails if no such node exists.
func (a *App) TapLabel(label string) {
	a.tb.Helper()
	n := a.MustNode(label)
	a.Tap(centerOf(n.Rect))
}

// HasLabel reports whether a node with this exact label is present.
func (a *App) HasLabel(label string) bool {
	a.tb.Helper()
	return a.Node(label) != nil
}

// NodeContaining returns the first node whose label contains sub, or nil.
//
// Exact matching is the better default — it fails when a label changes, which
// is usually what you want — but labels are often assembled from several
// pieces, and a node labelled "Sort by: Newest" is most naturally found by
// "Sort by". Both forms exist so a test can say which it means.
func (a *App) NodeContaining(sub string) *layout.SemNode {
	a.tb.Helper()
	nodes := a.Nodes()
	for i := range nodes {
		if strings.Contains(nodes[i].Label, sub) {
			return &nodes[i]
		}
	}
	return nil
}

// HasText reports whether any node's label contains sub.
func (a *App) HasText(sub string) bool {
	a.tb.Helper()
	return a.NodeContaining(sub) != nil
}

// TapText taps the centre of the first node whose label contains sub.
func (a *App) TapText(sub string) {
	a.tb.Helper()
	n := a.NodeContaining(sub)
	if n == nil {
		a.tb.Fatalf("apptest: no node whose label contains %q. Present labels: %s",
			sub, formatLabels(a.Labels()))
	}
	a.Tap(centerOf(n.Rect))
}

// AssertText fails the test unless some node's label contains sub.
func (a *App) AssertText(sub string) {
	a.tb.Helper()
	if !a.HasText(sub) {
		a.tb.Errorf("apptest: expected a node whose label contains %q. Present labels: %s",
			sub, formatLabels(a.Labels()))
	}
}

// AssertLabel fails the test unless a node with this label is present.
func (a *App) AssertLabel(label string) {
	a.tb.Helper()
	if !a.HasLabel(label) {
		a.tb.Errorf("apptest: expected a node labelled %q. Present labels: %s",
			label, formatLabels(a.Labels()))
	}
}

// AssertNoLabel fails the test if a node with this label is present. Useful for
// asserting something was dismissed or hidden.
func (a *App) AssertNoLabel(label string) {
	a.tb.Helper()
	if a.HasLabel(label) {
		a.tb.Errorf("apptest: did not expect a node labelled %q", label)
	}
}

func centerOf(r geom.Rect) geom.Pt {
	return geom.Pt{
		X: (r.Min.X + r.Max.X) / 2,
		Y: (r.Min.Y + r.Max.Y) / 2,
	}
}

func formatLabels(labels []string) string {
	if len(labels) == 0 {
		return "(none — did the widget render?)"
	}
	quoted := make([]string, len(labels))
	for i, l := range labels {
		quoted[i] = fmt.Sprintf("%q", l)
	}
	return strings.Join(quoted, ", ")
}
