package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// A row is RowHeight tall whatever its cells contain.
//
// RowHeight used to be padding rather than height: the row measured its cells
// and added the padding around them, so a cell holding a label over a caption
// produced a row taller than RowHeight. A caller sizing a table at
// count*RowHeight then came up short and lost its last rows — worse the more
// rows there were, and only for tables with a multi-line cell, which made it
// look random. LazyList's extent estimate was wrong by the same amount.
func tableRowSpan(t *testing.T, rowHeight float32, cell func(row, col int) widget.Widget) float32 {
	t.Helper()
	tbl := theme.Table{
		Columns:   []theme.Col{{Title: "A", Flex: 1}},
		Count:     4,
		RowHeight: rowHeight,
		Cell:      cell,
	}
	root := widget.Provide[theme.Theme]{Value: theme.Light(),
		Child: widget.Fill{Color: theme.Light().Bg, Child: tbl}}
	h, err := app.NewHeadless(root, app.Config{
		Size: geom.Size{W: 300, H: 600}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Distance between consecutive rows, measured from their labels.
	var first, second float32 = -1, -1
	for _, n := range h.Semantics() {
		switch n.Label {
		case "r0":
			first = n.Rect.Min.Y
		case "r1":
			second = n.Rect.Min.Y
		}
	}
	if first < 0 || second < 0 {
		t.Fatal("could not find two rows in the semantic tree")
	}
	return second - first
}

func TestRowHeightIsExactWhateverTheCellHolds(t *testing.T) {
	const rowHeight = 56
	oneLine := func(row, col int) widget.Widget {
		return widget.Semantics{Label: label(row), Child: widget.Text{S: "single"}}
	}
	twoLine := func(row, col int) widget.Widget {
		c := widget.Column(widget.Text{S: "label"}, widget.Text{S: "caption"})
		return widget.Semantics{Label: label(row), Child: c}
	}

	single := tableRowSpan(t, rowHeight, oneLine)
	double := tableRowSpan(t, rowHeight, twoLine)
	if single != double {
		t.Errorf("a two-line cell made rows %v apart, a one-line cell %v: RowHeight is not the row's height", double, single)
	}
	if single != rowHeight {
		t.Errorf("rows are %v apart, want RowHeight %v", single, rowHeight)
	}
}

// And the resolved height is askable, so callers sizing a scrolling parent do
// not reimplement the arithmetic.
func TestRowExtentReportsTheResolvedHeight(t *testing.T) {
	th := theme.Light()
	if got := (theme.Table{RowHeight: 56}).RowExtent(th); got != 56 {
		t.Errorf("RowExtent with RowHeight 56 = %v, want 56", got)
	}
	if got := (theme.Table{}).RowExtent(th); got <= 0 {
		t.Errorf("RowExtent with no RowHeight = %v, want the derived default", got)
	}
}

func label(row int) string { return "r" + string(rune('0'+row)) }
