package widget

import (
	"reflect"
	"testing"
)

// The half-extent threshold is what makes the swap happen as the dragged row's
// centre passes its neighbour's. A full extent feels stuck; zero flickers
// between two positions.
func TestReorderTargetSteps(t *testing.T) {
	const extent, count = 40, 5
	for _, tc := range []struct {
		name  string
		delta float32
		want  int
	}{
		{"still", 0, 2},
		{"a nudge is not a move", 15, 2},
		{"just past half", 21, 3},
		{"one full row", 40, 3},
		{"two rows", 80, 4},
		{"back one", -21, 1},
		{"back two", -80, 0},
	} {
		if got := reorderTarget(2, tc.delta, extent, count); got != tc.want {
			t.Errorf("%s: delta %v gave %d, want %d", tc.name, tc.delta, got, tc.want)
		}
	}
}

// Dragging past either end stops there rather than wrapping or going negative.
func TestReorderTargetClamps(t *testing.T) {
	if got := reorderTarget(2, 1000, 40, 5); got != 4 {
		t.Errorf("dragged past the end = %d, want 4", got)
	}
	if got := reorderTarget(2, -1000, 40, 5); got != 0 {
		t.Errorf("dragged past the start = %d, want 0", got)
	}
}

// Unmeasured rows must refuse to guess: dividing by a zero extent would send
// every drag to index 0, silently shuffling the user's list.
func TestReorderTargetInertWithoutExtent(t *testing.T) {
	if got := reorderTarget(3, 100, 0, 5); got != 3 {
		t.Errorf("zero extent gave %d, want the row to stay at 3", got)
	}
	if got := reorderTarget(-1, 100, 40, 5); got != -1 {
		t.Error("a target was computed with nothing being dragged")
	}
}

// The display order is what makes a gap open ahead of the drop.
func TestDisplayOrder(t *testing.T) {
	for _, tc := range []struct {
		name     string
		from, to int
		want     []int
	}{
		{"untouched", -1, -1, []int{0, 1, 2, 3, 4}},
		{"no move", 2, 2, []int{0, 1, 2, 3, 4}},
		{"down one", 1, 2, []int{0, 2, 1, 3, 4}},
		{"down to the end", 0, 4, []int{1, 2, 3, 4, 0}},
		{"up to the start", 3, 0, []int{3, 0, 1, 2, 4}},
	} {
		if got := displayOrder(5, tc.from, tc.to); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: displayOrder(5, %d, %d) = %v, want %v", tc.name, tc.from, tc.to, got, tc.want)
		}
	}
}

// Every item must appear exactly once however the drag is expressed, or a row
// disappears or duplicates mid-drag.
func TestDisplayOrderIsAPermutation(t *testing.T) {
	const n = 6
	for from := range n {
		for to := range n {
			got := displayOrder(n, from, to)
			if len(got) != n {
				t.Fatalf("displayOrder(%d,%d,%d) has %d items, want %d", n, from, to, len(got), n)
			}
			seen := map[int]bool{}
			for _, v := range got {
				if v < 0 || v >= n {
					t.Fatalf("index %d out of range in %v", v, got)
				}
				if seen[v] {
					t.Fatalf("index %d appears twice in %v", v, got)
				}
				seen[v] = true
			}
		}
	}
}

// Out-of-range inputs must leave the order untouched rather than panicking.
func TestDisplayOrderGuards(t *testing.T) {
	want := []int{0, 1, 2}
	for _, tc := range [][2]int{{-1, 1}, {1, -1}, {5, 1}, {1, 5}} {
		if got := displayOrder(3, tc[0], tc[1]); !reflect.DeepEqual(got, want) {
			t.Errorf("displayOrder(3, %d, %d) = %v, want it unchanged", tc[0], tc[1], got)
		}
	}
}
