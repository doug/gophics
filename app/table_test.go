package app

import (
	"fmt"
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/gomono"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// tableApp drives theme.Table through the headless harness so we can assert on
// virtualization, row taps, and header-sort routing.
type tableApp struct{ hook func(*tableTestState) }

func (a tableApp) CreateState() widget.State { s := &tableTestState{}; s.hook = a.hook; return s }

type tableTestState struct {
	widget.StateBase[tableApp]
	hook      func(*tableTestState)
	rows      int
	cellCalls int // Cell invocations in the last frame (virtualization probe)
	tapped    int
	selected  int
	sortCol   int
	sortDesc  bool
}

func (s *tableTestState) Init(widget.Ctx) {
	s.hook(s)
	s.rows = 5000
	s.tapped, s.selected, s.sortCol = -1, -1, -1
}

func (s *tableTestState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Light()
	s.cellCalls = 0 // reset per frame; Cell increments it for each built cell
	tbl := theme.Table{
		Columns: []theme.Col{
			{Title: "Date", Width: 92},
			{Title: "Payee", Flex: 2},
			{Title: "Account", Flex: 3},
			{Title: "Amount", Width: 110, Align: theme.AlignEnd},
		},
		Count: s.rows,
		Cell: func(r, c int) widget.Widget {
			s.cellCalls++
			switch c {
			case 0:
				return widget.Text{S: fmt.Sprintf("2026-01-%02d", r%28+1), Font: "mono", Size: th.Type.Body, Color: th.Text}
			case 1:
				return widget.Text{S: fmt.Sprintf("Payee number %d", r), Size: th.Type.Body, Color: th.Text, Ellipsis: true, MaxLines: 1}
			case 2:
				return widget.Text{S: "Expenses:Food:Groceries", Size: th.Type.Body, Color: th.Muted, Ellipsis: true, MaxLines: 1}
			default:
				return widget.Text{S: fmt.Sprintf("%.2f", float64(r%1000)+0.5), Font: "mono", Size: th.Type.Body, Color: th.Text}
			}
		},
		Selected: s.selected,
		OnTapRow: func(r int) { s.SetState(func() { s.tapped, s.selected = r, r }) },
		Sortable: true, SortCol: s.sortCol, SortDesc: s.sortDesc,
		OnSort: func(col int, desc bool) { s.SetState(func() { s.sortCol, s.sortDesc = col, desc }) },
	}
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg, Child: tbl}}
}

func tableHarness(t *testing.T) (*Headless, *tableTestState) {
	t.Helper()
	var st *tableTestState
	h, err := NewHeadless(tableApp{hook: func(s *tableTestState) { st = s }}, Config{
		Size: geom.Size{W: 560, H: 420}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF, "mono": gomono.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

// TestTableVirtualizes proves the table mounts only the visible rows: with 5000
// rows, a single frame builds a small constant number of cells, not 5000×cols.
func TestTableVirtualizes(t *testing.T) {
	h, st := tableHarness(t)
	h.Render()
	if st.cellCalls == 0 {
		t.Fatal("no cells built")
	}
	// 4 columns × ~a dozen visible rows (+overscan). Nowhere near 5000×4=20000.
	if st.cellCalls > 400 {
		t.Fatalf("expected only visible rows built, got %d cell calls for %d rows", st.cellCalls, st.rows)
	}
}

// TestTableRowTap checks a body-row tap routes to OnTapRow and drives selection.
func TestTableRowTap(t *testing.T) {
	h, st := tableHarness(t)
	for y := float32(44); y < 410 && st.tapped < 0; y += 3 {
		h.Tap(geom.Pt{X: 60, Y: y})
	}
	if st.tapped < 0 {
		t.Fatal("tapping the body did not fire OnTapRow")
	}
	if st.selected != st.tapped {
		t.Fatalf("selection %d did not follow tapped row %d", st.selected, st.tapped)
	}
}

// TestTableHeaderSort checks a header tap reports a sort, and re-tapping the same
// column toggles the direction.
func TestTableHeaderSort(t *testing.T) {
	h, st := tableHarness(t)
	var hitX float32
	for x := float32(540); x > 12 && st.sortCol < 0; x -= 4 {
		h.Tap(geom.Pt{X: x, Y: 20})
		if st.sortCol >= 0 {
			hitX = x
		}
	}
	if st.sortCol < 0 {
		t.Fatal("tapping a header did not fire OnSort")
	}
	if st.sortDesc {
		t.Fatal("first sort should be ascending")
	}
	col := st.sortCol
	h.Tap(geom.Pt{X: hitX, Y: 20}) // same column again → toggle
	if st.sortCol != col || !st.sortDesc {
		t.Fatalf("re-tapping column %d should toggle to descending; got col=%d desc=%v", col, st.sortCol, st.sortDesc)
	}
}

// TestTableGolden renders the table for visual inspection (dump with
// GOPHICS_RENDER_OUT=/tmp/table.png go test -run TestTableGolden ./app/).
func TestTableGolden(t *testing.T) {
	h, st := tableHarness(t)
	st.selected = 2
	h.core.Owner.RebuildAll()
	img := h.Render()
	if out := os.Getenv("GOPHICS_RENDER_OUT"); out != "" {
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}
}
