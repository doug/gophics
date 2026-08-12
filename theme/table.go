package theme

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// ColAlign positions a column's cell content horizontally.
type ColAlign uint8

const (
	AlignStart  ColAlign = iota // text (default), left-aligned
	AlignEnd                    // numbers — right-aligned so decimals line up
	AlignCenter                 // centered
)

// Col describes one Table column: a header title, a width rule, and alignment.
type Col struct {
	Title string
	// Width is a fixed column width in logical pixels. Zero makes the column
	// flexible, sharing the leftover width by Flex weight.
	Width float32
	// Flex is the weight of a flexible column (used only when Width == 0; 0 → 1).
	Flex int
	// Align positions cell content. Numeric columns should use AlignEnd.
	Align ColAlign
}

// Table is a dense, scannable data grid designed from Edward Tufte's rules for
// analytical tables: no vertical rules, a single hairline under the header,
// right-aligned numeric columns, a muted header, generous whitespace, and no
// zebra striping by default. Rows are virtualized on widget.LazyList — a
// register of tens of thousands of rows mounts only what's visible — and the
// header stays pinned above the scrolling body.
//
// Fixed (Width) and flexible (Flex) columns resolve identically for the header
// and every body row, so columns line up without a shared measurement pass.
//
// See design/data-table.md.
type Table struct {
	Columns []Col
	Count   int
	// Cell builds the widget shown at (row, col). Called lazily, for visible
	// rows only. Numeric cells should render in a monospaced/tabular face
	// (widget.Text{Font: "mono"}) so digits and commas stack.
	Cell func(row, col int) widget.Widget

	// RowHeight is each body row's height in logical px (0 → a comfortable
	// default derived from the body text size).
	RowHeight float32
	// ColumnGap is the horizontal breathing room inside each cell (0 → 14).
	ColumnGap float32

	// Selected highlights a row with the faintest tint (−1 → none).
	Selected int
	// OnTapRow fires when a body row is tapped.
	OnTapRow func(row int)

	// Zebra enables faint alternating-row shading. Off by default: Tufte treats
	// it as chartjunk, useful only for unusually dense tables.
	Zebra bool
	// RowRule draws a faint hairline between rows. Off by default; the single
	// header hairline is always drawn.
	RowRule bool

	// Sortable makes header cells tappable. SortCol/SortDesc draw a quiet
	// indicator; OnSort reports the requested order (the caller sorts the data).
	Sortable bool
	SortCol  int
	SortDesc bool
	OnSort   func(col int, desc bool)
}

func (t Table) CreateState() widget.State { return &tableState{} }

type tableState struct {
	widget.StateBase[Table]
}

func (s *tableState) Build(ctx widget.Ctx) widget.Widget {
	t := s.W()
	th := Of(ctx)

	gap := t.ColumnGap
	if gap == 0 {
		gap = 14
	}
	line := th.Type.Body
	vpad := float32(9)
	if t.RowHeight > 0 {
		vpad = (t.RowHeight - line) / 2
		if vpad < 2 {
			vpad = 2
		}
	}
	rowH := line + 2*vpad

	header := widget.Padding{
		Insets: geom.Insets{Top: 10, Bottom: 10},
		Child:  cellsRow(t.Columns, gap, func(c int) widget.Widget { return t.headerCell(th, c) }),
	}
	rule := widget.Fill{Color: th.Border, Child: widget.Sized{H: 1}}

	body := widget.LazyList{
		Count:           t.Count,
		EstimatedExtent: rowH,
		Build: func(i int) widget.Widget {
			return tableRow{
				cols: t.Columns, row: i, cell: t.Cell,
				gap: gap, vpad: vpad,
				selected: i == t.Selected,
				zebra:    t.Zebra, rowRule: t.RowRule,
				onTap: t.rowTap(i),
			}
		},
	}

	col := widget.Column(header, rule, widget.Expand(body))
	col.CrossAlign = layout.CrossStretch
	return col
}

func (t Table) rowTap(i int) func() {
	if t.OnTapRow == nil {
		return nil
	}
	return func() { t.OnTapRow(i) }
}

// headerCell renders a muted column label, with a quiet sort arrow when the
// table is sorted on this column.
func (t Table) headerCell(th Theme, c int) widget.Widget {
	title := widget.Text{S: t.Columns[c].Title, Size: th.Type.Label, Color: th.Muted}
	var content widget.Widget = title
	if t.Sortable && t.SortCol == c {
		arrow := "▲"
		if t.SortDesc {
			arrow = "▼"
		}
		content = widget.Row(title, widget.Sized{W: 4}, widget.Text{S: arrow, Size: th.Type.Caption, Color: th.Muted})
	}
	if !t.Sortable {
		return content
	}
	return widget.Interactive{
		Handler: widget.Handler{OnTap: func() { t.requestSort(c) }},
		Child:   content,
	}
}

func (t Table) requestSort(c int) {
	if t.OnSort == nil {
		return
	}
	desc := false
	if t.SortCol == c {
		desc = !t.SortDesc
	}
	t.OnSort(c, desc)
}

// tableRow is one body row, stateful only so its hover highlight is isolated —
// hovering a row must not rebuild the whole table.
type tableRow struct {
	cols     []Col
	row      int
	cell     func(row, col int) widget.Widget
	gap      float32
	vpad     float32
	selected bool
	zebra    bool
	rowRule  bool
	onTap    func()
}

func (tableRow) CreateState() widget.State { return &tableRowState{} }

type tableRowState struct {
	widget.StateBase[tableRow]
	hover bool
}

func (s *tableRowState) Build(ctx widget.Ctx) widget.Widget {
	r := s.W()
	th := Of(ctx)

	var bg paint.Color // transparent by default — no ink
	switch {
	case r.selected:
		bg = paint.Lerp(th.Bg, th.Primary, 0.10)
	case s.hover:
		bg = th.SurfaceHover
	case r.zebra && r.row%2 == 1:
		bg = paint.Lerp(th.Bg, th.Surface, 0.6)
	}

	cells := cellsRow(r.cols, r.gap, func(c int) widget.Widget { return r.cell(r.row, c) })
	content := widget.Padding{Insets: geom.Insets{Top: r.vpad, Bottom: r.vpad}, Child: cells}
	var row widget.Widget = widget.Decorated{Color: bg, Child: content}
	if r.rowRule {
		stack := widget.Column(row, widget.Fill{Color: th.Border.WithAlpha(0.5), Child: widget.Sized{H: 1}})
		stack.CrossAlign = layout.CrossStretch
		row = stack
	}

	return widget.Interactive{
		Handler: widget.Handler{
			OnTap:   r.onTap,
			OnEnter: func() { s.SetState(func() { s.hover = true }) },
			OnExit:  func() { s.SetState(func() { s.hover = false }) },
		},
		Child: row,
	}
}

// cellsRow lays out one row of cells (header or body) across the columns. Fixed
// columns take their Width; flexible columns share the remainder by weight; the
// content within each column is aligned per Col.Align. Header and body call this
// with the same columns, so their cells line up exactly.
func cellsRow(cols []Col, gap float32, build func(c int) widget.Widget) widget.Widget {
	cells := make([]widget.Widget, len(cols))
	for c, col := range cols {
		cells[c] = colCell(col, gap, build(c))
	}
	row := widget.Row(cells...)
	row.CrossAlign = layout.CrossStretch
	return row
}

// colCell sizes one cell to its column and aligns its content horizontally. It
// aligns with spacers rather than a fill-and-position box so a cell never has to
// resolve its height against the row — the outer row centers it vertically.
func colCell(col Col, gap float32, content widget.Widget) widget.Widget {
	padded := widget.Padding{Insets: geom.Insets{Left: gap / 2, Right: gap / 2}, Child: content}

	var inner widget.Widget
	switch col.Align {
	case AlignEnd:
		inner = widget.Row(widget.Spacer(), padded)
	case AlignCenter:
		inner = widget.Row(widget.Spacer(), padded, widget.Spacer())
	default:
		inner = widget.Row(padded, widget.Spacer())
	}

	if col.Width > 0 {
		return widget.Sized{W: col.Width, Child: inner}
	}
	flex := col.Flex
	if flex == 0 {
		flex = 1
	}
	return widget.Flexible{Flex: flex, Child: inner}
}
