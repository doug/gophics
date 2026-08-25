package chart

// Stacking groups marks so they accumulate rather than sit side by side.
//
// The field has converged on roughly three ways of saying this, and the choice
// here follows from how gophics states a chart. ggplot2 and Observable Plot
// treat it as a position adjustment applied to a layer, orthogonal to the mark
// (position="stack" against "dodge"). Vega-Lite makes it a property of the
// quantitative encoding channel ("stack": "zero" | "normalize" | null). Apple's
// Swift Charts and ECharts attach it to the series — Swift Charts stacks by
// default and takes a MarkStackingMethod to opt out, ECharts takes a stack name
// and series sharing it accumulate.
//
// The first two infer the grouping from an aesthetic mapping — the fill or
// color channel says which series belong together. gophics has no such
// mapping: marks are values in a slice, and nothing about two BarMarks says
// they are one stack. So the grouping has to be named, which is ECharts' answer
// and the one adopted here: marks sharing a non-empty Stack accumulate, and
// each distinct Stack occupies one slot in the band.
//
// One thing worth taking from the others: every mature system treats stacking
// as a mode with more than two values — standard, normalized to 100%, centered
// for a streamgraph. A bool would be regretted. Stack is a string rather than a
// bool for grouping, and a stacking *method* can be added later without
// disturbing it.

// stackable is a mark that can participate in a stack.
type stackable interface {
	stackID() string
	stackData() []Datum
}

// stackBases returns, for each mark index that stacks, the running total of the
// marks below it at each X — the baseline that mark should draw from. Marks are
// accumulated in slice order, so the first mark in a stack sits on zero and
// each subsequent one rests on the sum of those before it.
//
// A mark with no Stack is absent from the result and draws from zero as before.
func stackBases(marks []Mark) map[int]map[float64]float64 {
	var bases map[int]map[float64]float64
	running := map[string]map[float64]float64{}
	for i, mk := range marks {
		sm, ok := mk.(stackable)
		if !ok || sm.stackID() == "" {
			continue
		}
		id := sm.stackID()
		acc := running[id]
		if acc == nil {
			acc = map[float64]float64{}
			running[id] = acc
		}
		base := make(map[float64]float64, len(sm.stackData()))
		for _, d := range sm.stackData() {
			base[d.X] = acc[d.X]
			acc[d.X] += d.Y
		}
		if bases == nil {
			bases = map[int]map[float64]float64{}
		}
		bases[i] = base
	}
	return bases
}

// stackExtent returns the lowest and highest stacked total across every stack,
// so the Y domain covers the stacks rather than their tallest single mark.
// Without this a stack of 3 and 7 would be plotted against a domain ending at
// 7 and draw straight out of the plot area.
func stackExtent(marks []Mark) (lo, hi float64, any bool) {
	totals := map[string]map[float64]float64{}
	for _, mk := range marks {
		sm, ok := mk.(stackable)
		if !ok || sm.stackID() == "" {
			continue
		}
		acc := totals[sm.stackID()]
		if acc == nil {
			acc = map[float64]float64{}
			totals[sm.stackID()] = acc
		}
		for _, d := range sm.stackData() {
			acc[d.X] += d.Y
		}
	}
	for _, acc := range totals {
		for _, v := range acc {
			if !any {
				lo, hi, any = v, v, true
				continue
			}
			lo, hi = min(lo, v), max(hi, v)
		}
	}
	return lo, hi, any
}

// stackTops reports which mark index is the last one in each stack, so only
// that segment's outer edge is rounded. Segments have to butt: a radius on the
// facing edges of two adjacent segments leaves a lens of background between
// them, and a stack reads as a column of separate floating blocks.
func stackTops(marks []Mark) map[int]bool {
	last := map[string]int{}
	for i, mk := range marks {
		if sm, ok := mk.(stackable); ok && sm.stackID() != "" && len(sm.stackData()) > 0 {
			last[sm.stackID()] = i
		}
	}
	tops := map[int]bool{}
	for _, i := range last {
		tops[i] = true
	}
	return tops
}

// stackSlots counts how many slots the bars need in a band: each distinct
// stack takes one, and every unstacked bar mark takes one of its own. Without
// this a two-mark stack would dodge into two half-width slots and stop looking
// like a stack at all.
func stackSlots(marks []Mark) (slots int, slotOf map[int]int) {
	slotOf = map[int]int{}
	seen := map[string]int{}
	for i, mk := range marks {
		if _, ok := mk.(BarMark); !ok {
			continue
		}
		id := ""
		if sm, ok := mk.(stackable); ok {
			id = sm.stackID()
		}
		if id == "" {
			slotOf[i] = slots
			slots++
			continue
		}
		if s, ok := seen[id]; ok {
			slotOf[i] = s
			continue
		}
		seen[id] = slots
		slotOf[i] = slots
		slots++
	}
	return slots, slotOf
}
