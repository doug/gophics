package widget_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Helpers for the headless tests in this package.

// headless builds a w×h headless app around root, with a real font so text
// fields measure and paint.
func headless(t *testing.T, root widget.Widget, w, h float32) *app.Headless {
	t.Helper()
	hl, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: w, H: h}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	return hl
}

// mutable is a stateful root whose Build the test rewrites between frames:
// the way an app's own state drives a rebuild, without the test reaching
// into the tree.
type mutable struct{ m *mutableModel }

type mutableModel struct {
	build func(widget.Ctx) widget.Widget
	st    *mutableState
}

// newMutable returns the root to mount and the handle the test rebuilds it
// through.
func newMutable(build func(widget.Ctx) widget.Widget) (widget.Widget, *mutableModel) {
	m := &mutableModel{build: build}
	return mutable{m}, m
}

// Rebuild asks the root to build again with whatever the closure now reads.
func (m *mutableModel) Rebuild() { m.st.SetState(nil) }

func (r mutable) CreateState() widget.State { return &mutableState{} }

type mutableState struct{ widget.StateBase[mutable] }

func (s *mutableState) Init(widget.Ctx)                    { s.W().m.st = s }
func (s *mutableState) Build(ctx widget.Ctx) widget.Widget { return s.W().m.build(ctx) }

// builder is a stateless widget built from a closure, for a test that needs
// a Ctx (to reach a Nav or an Overlay) and nothing else.
type builder struct {
	b func(widget.Ctx) widget.Widget
}

func (b builder) Build(ctx widget.Ctx) widget.Widget { return b.b(ctx) }

// semRect returns the rect of the first semantics node carrying label.
func semRect(t *testing.T, h *app.Headless, label string) geom.Rect {
	t.Helper()
	var find func(nodes []layout.SemNode) (geom.Rect, bool)
	find = func(nodes []layout.SemNode) (geom.Rect, bool) {
		for _, n := range nodes {
			if n.Label == label {
				return n.Rect, true
			}
			if r, ok := find(n.Children); ok {
				return r, true
			}
		}
		return geom.Rect{}, false
	}
	r, ok := find(h.Semantics())
	if !ok {
		t.Fatalf("no semantics node labelled %q", label)
	}
	return r
}

// findRole returns the first semantics node with role r, or nil.
func findRole(nodes []layout.SemNode, r layout.Role) *layout.SemNode {
	for i := range nodes {
		if nodes[i].Role == r {
			return &nodes[i]
		}
		if n := findRole(nodes[i].Children, r); n != nil {
			return n
		}
	}
	return nil
}
