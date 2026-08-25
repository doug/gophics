package chart

import "testing"

// Marks sharing a Stack accumulate; marks without one dodge.
//
// The trap this replaces is worth stating: the natural workaround was to feed
// each series its cumulative total and draw back to front, which is how you
// stack areas. Bars still dodged, so what came out was a grouped chart whose
// bar heights were running totals — plausible-looking and wrong in every
// number. Tiller shipped that for an iteration before a screenshot caught it.
func TestStackedMarksRestOnTheOnesBelow(t *testing.T) {
	marks := []Mark{
		BarMark{Data: Values("mon", 3, "tue", 5), Stack: "spend"},
		BarMark{Data: Values("mon", 7, "tue", 1), Stack: "spend"},
	}
	bases := stackBases(marks)

	// The first mark of a stack sits on zero.
	if got := bases[0][0]; got != 0 {
		t.Errorf("first stacked mark has base %v at x=0, want 0", got)
	}
	// The second rests on the first's value, not on zero and not on a total.
	if got, want := bases[1][0], 3.0; got != want {
		t.Errorf("second stacked mark has base %v at x=0, want %v", got, want)
	}
	if got, want := bases[1][1], 5.0; got != want {
		t.Errorf("second stacked mark has base %v at x=1, want %v", got, want)
	}
}

func TestUnstackedMarksHaveNoBase(t *testing.T) {
	marks := []Mark{
		BarMark{Data: Values("mon", 3)},
		BarMark{Data: Values("mon", 7)},
	}
	if bases := stackBases(marks); len(bases) != 0 {
		t.Errorf("unstacked marks got bases %v, want none", bases)
	}
}

// Separate stacks do not pool: two stacks side by side each start from zero.
func TestStacksAreIndependent(t *testing.T) {
	marks := []Mark{
		BarMark{Data: Values("mon", 3), Stack: "a"},
		BarMark{Data: Values("mon", 7), Stack: "b"},
		BarMark{Data: Values("mon", 2), Stack: "a"},
	}
	bases := stackBases(marks)
	if got := bases[1][0]; got != 0 {
		t.Errorf("stack b starts at %v, want 0 — it must not pool with stack a", got)
	}
	if got, want := bases[2][0], 3.0; got != want {
		t.Errorf("second mark of stack a rests at %v, want %v", got, want)
	}
}

// The Y domain has to cover the stack's total. Without this a stack of 3 and 7
// is plotted against a domain ending at 7 and draws out of the plot area.
func TestTheDomainCoversTheStackNotItsTallestMark(t *testing.T) {
	marks := []Mark{
		BarMark{Data: Values("mon", 3), Stack: "s"},
		BarMark{Data: Values("mon", 7), Stack: "s"},
	}
	_, hi, any := stackExtent(marks)
	if !any {
		t.Fatal("stackExtent found no stacks")
	}
	if want := 10.0; hi != want {
		t.Errorf("stack tops out at %v, want %v", hi, want)
	}

	_, ys := resolveScales(Chart{Marks: marks})
	if got := ys.Map(10); got > 1.0001 {
		t.Errorf("the stack's total maps to %v, past the top of the plot area", got)
	}
}

// A stack occupies one slot in the band, not one per mark — otherwise a
// two-mark stack dodges into two half-width slots and stops reading as a stack.
func TestAStackTakesOneSlot(t *testing.T) {
	stacked := []Mark{
		BarMark{Data: Values("mon", 3), Stack: "s"},
		BarMark{Data: Values("mon", 7), Stack: "s"},
	}
	if slots, slotOf := stackSlots(stacked); slots != 1 || slotOf[0] != slotOf[1] {
		t.Errorf("a two-mark stack took %d slots (%v), want 1 shared", slots, slotOf)
	}

	// Grouped bars still dodge, one slot each.
	grouped := []Mark{
		BarMark{Data: Values("mon", 3)},
		BarMark{Data: Values("mon", 7)},
	}
	if slots, slotOf := stackSlots(grouped); slots != 2 || slotOf[0] == slotOf[1] {
		t.Errorf("two ungrouped bars took %d slots (%v), want 2 separate", slots, slotOf)
	}

	// And the two mix: a stack beside a lone bar is two slots.
	mixed := []Mark{
		BarMark{Data: Values("mon", 3), Stack: "s"},
		BarMark{Data: Values("mon", 7), Stack: "s"},
		BarMark{Data: Values("mon", 2)},
	}
	if slots, _ := stackSlots(mixed); slots != 2 {
		t.Errorf("a stack beside a lone bar took %d slots, want 2", slots)
	}
}
