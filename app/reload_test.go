package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/widget"
)

// TestReloadBoundary proves the hot-reload boundary: swapping the cell's
// builder and rebuilding re-runs the new code against the existing element
// tree (the mechanism `gossamer dev --hot` drives when a reloaded plugin
// supplies a fresh Root).
func TestReloadBoundary(t *testing.T) {
	label := "alpha"
	cell := &reloadCell{build: func() widget.Widget {
		return widget.Semantics{Label: label, Child: widget.Text{S: label, Size: 14}}
	}}
	h, err := NewHeadless(reloadHost{cell}, Config{Size: geom.Size{W: 200, H: 80}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if !hasA11yLabel(h, "alpha") {
		t.Fatal("initial builder did not mount")
	}

	// Simulate a reload: new code at the same tree position.
	label = "beta"
	h.Core.Owner.RebuildAll()
	h.Render()

	if !hasA11yLabel(h, "beta") {
		t.Fatal("reload did not re-run the builder with new code")
	}
	if hasA11yLabel(h, "alpha") {
		t.Fatal("stale view survived the reload")
	}
}

func hasA11yLabel(h *Headless, s string) bool {
	for _, n := range h.Core.A11yTree(1) {
		if n.Label == s {
			return true
		}
	}
	return false
}
