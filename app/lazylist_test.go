package app

import (
	"fmt"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

func lazyHarness(t *testing.T, count int) (*Headless, *map[int]int) {
	t.Helper()
	built := map[int]int{}
	list := widget.LazyList{
		Count:           count,
		EstimatedExtent: 30,
		Build: func(i int) widget.Widget {
			built[i]++
			return widget.Text{S: fmt.Sprintf("item %d", i)}
		},
	}
	h, err := NewHeadless(list, Config{Size: geom.Size{W: 200, H: 300}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, &built
}

func TestLazyListMountsOnlyVisible(t *testing.T) {
	h, built := lazyHarness(t, 10000)
	h.Render()

	if len(*built) > 100 {
		t.Fatalf("built %d of 10000 items; laziness failed", len(*built))
	}
	if _, ok := (*built)[0]; !ok {
		t.Fatal("first item must be built")
	}
	if _, ok := (*built)[9999]; ok {
		t.Fatal("last item must not be built at offset 0")
	}
}

func TestLazyListScrollRevealsAndPreserves(t *testing.T) {
	h, built := lazyHarness(t, 10000)
	h.Render()

	// Scroll deep into the list.
	h.Move(geom.Pt{X: 100, Y: 150})
	for range 20 {
		h.Scroll(geom.Pt{Y: -400})
		h.Render()
	}
	// Items near the new window are built; the very first is not rebuilt
	// endlessly (it should be unmounted, far above the window).
	far := false
	for i := range *built {
		if i > 200 {
			far = true
		}
	}
	if !far {
		t.Fatalf("scrolling did not reveal deeper items: %d built", len(*built))
	}
	if len(*built) > 600 {
		t.Fatalf("too many items built while scrolling: %d", len(*built))
	}
}

// Regression: a hit test arriving between a list-mutating rebuild and its
// layout must not index stale flex offsets (Android fling crash).
func TestHitTestAfterRebuildBeforeLayout(t *testing.T) {
	h, _ := lazyHarness(t, 5000)
	h.Render()
	h.Move(geom.Pt{X: 100, Y: 150})
	for range 30 {
		// Scroll mutates the visible window (rebuild pending), then a raw
		// pointer event hits immediately — no Render in between.
		h.Core.Pointer(shell.Pointer{Kind: shell.PointerScroll, Scroll: geom.Pt{Y: -300}})
		h.Core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 100, Y: 150}})
		h.Core.Pointer(shell.Pointer{Kind: shell.PointerUp, Pos: geom.Pt{X: 100, Y: 150}})
	}
	h.Render()
}
