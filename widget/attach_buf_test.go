package widget

import (
	"fmt"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// Nobody keeps the slice attach is handed.
//
// attachKids fills one buffer per element and reuses it every frame, so an
// attach implementation that stores the slice rather than copying out of it
// ends up reading the *next* frame's boxes. That fails silently: the tree lays
// out against children it no longer has, nothing errors, and the symptom is a
// frame of lag somewhere in a layout nobody is looking at. Every
// implementation copies today and they all live in this package, but nothing
// makes that true.
//
// So: null the buffer the instant attach returns. A no-op if the contract
// holds; a pile of nils in somebody's Children if it does not.
func TestNoAttachRetainsItsBuffer(t *testing.T) {
	// A tree covering the attach shapes — single-child wrappers, a
	// multi-child flex, a stack, and nesting of both.
	build := func() Widget {
		rows := make([]Widget, 0, 6)
		for i := range 6 {
			rows = append(rows, Padding{All: 2, Child: Sized{W: 40, H: 10,
				Child: Align{X: 0.5, Y: 0.5, Child: Decorated{Radius: 2,
					Child: Semantics{Label: fmt.Sprint("cell", i), Child: Sized{W: 10, H: 4}}}}}})
		}
		return Padding{All: 4, Child: Stack{Children: []Widget{
			Fill{Child: Sized{W: 100, H: 100}},
			Column(rows...),
		}}}
	}

	layoutWith := func(scrub bool) []layout.InspectNode {
		prev := scrubAttachBuf
		scrubAttachBuf = scrub
		defer func() { scrubAttachBuf = prev }()

		o := &Owner{}
		o.SetRoot(build())
		o.FlushBuilds()
		box := o.root.renderBox()
		box.Layout(layout.Loose(geom.Size{W: 200, H: 200}))
		return layout.Inspect(box)
	}

	want := layoutWith(false)
	got := layoutWith(true)
	if len(want) == 0 {
		t.Fatal("the probe tree laid out to nothing; it is not exercising attach")
	}
	if len(got) != len(want) {
		t.Fatalf("scrubbing the attach buffer changed the box count: %d, want %d — "+
			"an attach kept the slice and lost its children", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("scrubbing the attach buffer changed the layout at box %d:\n got %+v\nwant %+v\n"+
				"some attach kept the slice instead of copying out of it", i, got[i], want[i])
		}
	}
}
