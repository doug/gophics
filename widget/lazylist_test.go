package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// LazyList's contract: only rows in or near the viewport are built.
func TestLazyListWindowsBuilds(t *testing.T) {
	built := map[int]int{}
	list := LazyList{
		Count:           1000,
		EstimatedExtent: 40,
		Build: func(i int) Widget {
			built[i]++
			return Sized{W: 100, H: 40}
		},
	}
	o := newOwner()
	o.SetRoot(list)
	o.FlushBuilds()

	// First frame (viewport not yet measured, generous 800px window +
	// overscan): a bounded prefix, nowhere near all 1000 rows.
	if len(built) == 0 {
		t.Fatal("no rows built")
	}
	if len(built) > 60 {
		t.Fatalf("first build touched %d rows — windowing is not engaging", len(built))
	}
	if _, ok := built[0]; !ok {
		t.Fatal("row 0 not built on first frame")
	}
	if _, ok := built[999]; ok {
		t.Fatal("row 999 built while far outside the window")
	}

	// Scroll deep into the list: the window must move with the offset.
	ls := digState[LazyList](o.root).(*lazyState)
	clear(built)
	ls.SetState(func() {
		ls.offset = 400 * 40 // top of row 400
		ls.viewH = 600
	})
	o.FlushBuilds()

	if _, ok := built[400]; !ok {
		t.Fatal("row at the scrolled offset not built")
	}
	if len(built) > 60 {
		t.Fatalf("scrolled build touched %d rows, want a viewport-sized window", len(built))
	}
	for _, i := range []int{0, 100, 800, 999} {
		if _, ok := built[i]; ok {
			t.Fatalf("row %d built while far outside the scrolled window", i)
		}
	}
	// The window is contiguous around the offset: rows just inside the
	// overscan margin are present.
	// offset 16000, viewH 600, overscan 300 → content window [15700, 16900]
	// → rows 393..422 (40px each).
	if _, ok := built[395]; !ok {
		t.Fatal("row just above the viewport (in overscan) not built")
	}
	if _, ok := built[420]; !ok {
		t.Fatal("row just below the viewport (in overscan) not built")
	}
}

// Rows report their laid-out heights into the list's cache, replacing the
// estimate; rows never built stay unmeasured (0).
func TestLazyListMeasuredHeightCaching(t *testing.T) {
	rowH := func(i int) float32 { return float32(20 + (i%5)*10) } // 20..60
	list := LazyList{
		Count:           200,
		EstimatedExtent: 30,
		Build: func(i int) Widget {
			return Sized{W: 100, H: rowH(i)}
		},
	}
	o := newOwner()
	o.SetRoot(list)
	o.FlushBuilds()

	box := o.RootBox()
	box.Layout(layout.Tight(geom.Size{W: 300, H: 400}))
	o.FlushBuilds() // deliver any OnOffset SetState from layout

	ls := digState[LazyList](o.root).(*lazyState)
	if len(ls.heights) != 200 {
		t.Fatalf("heights len = %d, want 200", len(ls.heights))
	}
	// The first rows were built and laid out: measured, not estimated.
	for i := range 5 {
		if got := ls.heights[i]; got != rowH(i) {
			t.Fatalf("heights[%d] = %v, want measured %v", i, got, rowH(i))
		}
	}
	// A row far outside the window was never laid out: still unmeasured,
	// so height() falls back to the estimate.
	if got := ls.heights[190]; got != 0 {
		t.Fatalf("heights[190] = %v, want 0 (unmeasured)", got)
	}
	if got := ls.height(190); got != 30 {
		t.Fatalf("height(190) = %v, want the 30px estimate", got)
	}
}

// Growing Count (infinite feed) must preserve already-measured heights.
func TestLazyListCountGrowthKeepsMeasurements(t *testing.T) {
	mk := func(count int) LazyList {
		return LazyList{
			Count:           count,
			EstimatedExtent: 30,
			Build:           func(i int) Widget { return Sized{W: 100, H: 44} },
		}
	}
	o := newOwner()
	o.SetRoot(mk(50))
	o.FlushBuilds()
	o.RootBox().Layout(layout.Tight(geom.Size{W: 300, H: 400}))
	o.FlushBuilds()

	ls := digState[LazyList](o.root).(*lazyState)
	if ls.heights[0] != 44 {
		t.Fatalf("setup: heights[0] = %v, want 44", ls.heights[0])
	}

	o.SetRoot(mk(100))
	o.FlushBuilds()
	if len(ls.heights) != 100 {
		t.Fatalf("heights len after growth = %d, want 100", len(ls.heights))
	}
	if ls.heights[0] != 44 {
		t.Fatalf("measured height lost across Count growth: heights[0] = %v", ls.heights[0])
	}
}
