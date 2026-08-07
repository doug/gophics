package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// benchApp builds a representative tree: 30 decorated, labeled rows.
func benchApp(b *testing.B) (*Headless, *rowsState) {
	b.Helper()
	var st *rowsState
	h, err := NewHeadless(benchRoot{hook: func(s *rowsState) { st = s }}, Config{
		Size: geom.Size{W: 400, H: 800}, Background: paint.RGB(0.1, 0.1, 0.1),
		Font: goregular.TTF,
	}, 2)
	if err != nil {
		b.Fatal(err)
	}
	h.Render()
	return h, st
}

type benchRoot struct{ hook func(*rowsState) }

func (r benchRoot) CreateState() widget.State { return &benchState{hook: r.hook} }

type benchState struct {
	widget.StateBase[benchRoot]
	rows rowsState
	hook func(*rowsState)
}

func (s *benchState) Init(ctx widget.Ctx) {
	s.rows.hover = -1
	s.hook(&s.rows)
}

func (s *benchState) Build(ctx widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, 30)
	for i := range rows {
		col := paint.RGB(0.2, 0.2, 0.25)
		if i == s.rows.hover {
			col = paint.RGB(0.4, 0.4, 0.5)
		}
		rows[i] = widget.Decorated{Color: col, Radius: 6,
			Child: widget.Padding{All: 4,
				Child: widget.Text{S: "list row with some text", Color: paint.RGB(0.9, 0.9, 0.9)}}}
	}
	c := widget.Column(rows...)
	return widget.Padding{All: 10, Child: c}
}

// hoverAndRender marks the row widget dirty (via its owner state) and
// renders one frame.
func hoverAndRender(h *Headless, st *rowsState, row int) {
	st.hover = row
	// benchState owns the rows; mark the root dirty through a state change.
	h.core.Owner.SetRoot(benchRoot{hook: func(*rowsState) {}})
	h.Render()
}

func BenchmarkFrameUnchanged(b *testing.B) {
	h, _ := benchApp(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Render()
		if !h.core.Skipped {
			b.Fatal("frame should skip")
		}
	}
}

func BenchmarkFrameLocalizedChange(b *testing.B) {
	h, st := benchApp(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		hoverAndRender(h, st, i%2) // alternate two rows: small damage
	}
}

func BenchmarkFrameFullRepaint(b *testing.B) {
	h, st := benchApp(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.core.prev.Reset() // empty previous scene: everything is damage
		hoverAndRender(h, st, i%2)
	}
}

func BenchmarkRecordAndDiff(b *testing.B) {
	h, _ := benchApp(b)
	size := geom.Size{W: 400, H: 800}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.core.RecordScene(size, 2)
	}
}
