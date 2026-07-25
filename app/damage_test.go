package app

import (
	"bytes"
	"image/png"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// rowsApp is a hoverable list used to produce localized scene changes.
type rowsApp struct{ N int }

func (r rowsApp) CreateState() widget.State { return &rowsState{} }

type rowsState struct {
	widget.StateBase[rowsApp]
	hover int
}

func (s *rowsState) Init(widget.Ctx) { s.hover = -1 }

func (s *rowsState) Build(widget.Ctx) widget.Widget {
	rows := make([]widget.Widget, s.W().N)
	for i := range rows {
		col := paint.RGB(0.3, 0.3, 0.35)
		if i == s.hover {
			col = paint.RGB(0.5, 0.5, 0.6)
		}
		i := i
		rows[i] = widget.Interactive{
			Handler: widget.Handler{
				OnEnter: func() { s.SetState(func() { s.hover = i }) },
				OnExit:  func() { s.SetState(func() { s.hover = -1 }) },
			},
			Child: widget.Decorated{Color: col, Radius: 4,
				Child: widget.Sized{W: 180, H: 24}},
		}
	}
	c := widget.Column(rows...)
	return widget.Padding{All: 10, Child: c}
}

func damageApp(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(rowsApp{N: 8}, Config{
		Size: geom.Size{W: 200, H: 300}, Background: paint.RGB(0.1, 0.1, 0.1),
		Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func TestUnchangedFrameSkipsRasterization(t *testing.T) {
	h := damageApp(t)
	h.Render() // no state change since last render
	if !h.Core.Skipped {
		t.Fatal("identical frame must skip rasterization")
	}
}

func TestHoverDamageIsLocalized(t *testing.T) {
	h := damageApp(t)
	h.Move(geom.Pt{X: 100, Y: 50}) // hover a row
	h.Render()
	if h.Core.Skipped {
		t.Fatal("hover change must repaint")
	}
	d := h.Core.LastDamage
	full := geom.RectFromSize(geom.Size{W: 200, H: 300})
	if d == full || d.Dy() > 40 {
		t.Fatalf("hover damage %v should be one row, not the surface", d)
	}
}

// TestIncrementalMatchesFullRepaint is the damage-correctness oracle: after
// a series of state changes rendered incrementally, the surface must be
// pixel-identical to a fresh app rendered directly into the final state.
func TestIncrementalMatchesFullRepaint(t *testing.T) {
	h := damageApp(t)
	// Hover around, ending hovering row ~5.
	for _, y := range []float32{30, 80, 130, 180, 160} {
		h.Move(geom.Pt{X: 100, Y: y})
		h.Render()
	}
	incremental := encodePNG(t, h)

	fresh := damageApp(t)
	fresh.Move(geom.Pt{X: 100, Y: 160})
	fresh.Render()
	direct := encodePNG(t, fresh)

	if !bytes.Equal(incremental, direct) {
		t.Fatal("incremental damaged rendering diverged from full repaint")
	}
}

func encodePNG(t *testing.T, h *Headless) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, h.Render()); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
