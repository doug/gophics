// Package overlay is the embedded UI: a widget tree and nothing else.
//
// It does not import Ebiten, which is deliberate and is the pattern the example
// is recommending. An embedded UI reads and writes host state through a narrow
// interface, so it can be built and tested without starting the host — Ebiten's
// package init opens a window, and a UI that imports it cannot be tested
// headlessly at all.
package overlay

import (
	"fmt"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// Model is the host state the overlay reads and writes.
//
// This is the whole coupling: an embedded UI is a control surface for the
// program it sits in, and the interface is what that program has to expose. It
// is also what lets the UI be tested against a fake instead of a running game.
type Model interface {
	Elapsed() float64
	Paused() bool
	TogglePause()
	Speed() float32
	SetSpeed(float32)
}

// UI is the overlay's widget tree.
type UI struct{ M Model }

func (u UI) CreateState() widget.State { return &uiState{} }

type uiState struct {
	widget.StateBase[UI]
	ctx  widget.Ctx
	note string
}

// Init registers a ticker, because the overlay shows host state that changes
// without the UI touching it.
//
// This is the one thing an embedded UI cannot get for free. gophics rebuilds
// when *its* state changes; a readout of the host's clock changes when the host
// says so, and nothing in the widget tree knows that happened. Without this the
// panel renders once and the elapsed time never moves — which looks like a
// broken UI and is really a missing subscription.
//
// It costs a rebuild per frame, which is the honest price of a live readout.
// A panel showing only settings would not need it.
func (s *uiState) Init(ctx widget.Ctx) {
	s.ctx = ctx
	ctx.AddTicker(s)
}

func (s *uiState) Dispose() { s.ctx.RemoveTicker(s) }

// Tick asks for a rebuild each frame so the readout tracks the host.
func (s *uiState) Tick(float64) bool {
	s.SetState(nil)
	return true
}

func (s *uiState) Build(ctx widget.Ctx) widget.Widget {
	m := s.W().M

	fg := paint.RGB(0.92, 0.94, 0.98)
	dim := paint.RGB(0.62, 0.66, 0.76)

	rows := []widget.Widget{
		widget.Text{Value: "Overlay", Size: 20, Color: fg},
		widget.Sized{H: 4},
		widget.Text{Value: "a gophics UI over a live game", Size: 12, Color: dim, Wrap: true},
		widget.Sized{H: 16},

		// A live readout: proof the overlay is reading host state each frame.
		widget.Text{Value: fmt.Sprintf("t = %.1fs", m.Elapsed()), Size: 13, Color: dim},
		widget.Sized{H: 12},

		button(pauseLabel(m), m.TogglePause),
		widget.Sized{H: 8},
		button("faster", func() { m.SetSpeed(min(m.Speed()+0.5, 4)) }),
		widget.Sized{H: 8},
		button("slower", func() { m.SetSpeed(max(m.Speed()-0.5, 0)) }),
		widget.Sized{H: 16},

		// A text field, because it is the affordance most likely to be broken
		// by an incomplete host: it needs keys, committed text, and focus.
		widget.Text{Value: "note", Size: 12, Color: dim},
		widget.Sized{H: 4},
		widget.Decorated{
			Color: paint.Color{R: 1, G: 1, B: 1, A: 0.08}, Radius: 6,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(8, 6),
				Child: widget.Sized{H: 22, Child: widget.TextField{
					Value:       s.note,
					Placeholder: "type here",
					TextColor:   fg,
					OnChange:    func(v string) { s.SetState(func() { s.note = v }) },
				}},
			},
		},
	}

	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch
	return widget.Padding{Insets: geom.InsetsAll(18), Child: col}
}

func button(label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Gestures: widget.Gestures{OnTap: onTap},
		Child: widget.Decorated{
			Color: paint.Color{R: 1, G: 1, B: 1, A: 0.12}, Radius: 6,
			Child: widget.Padding{
				Insets: geom.InsetsSymmetric(12, 9),
				Child:  widget.Center(widget.Text{Value: label, Size: 13, Color: paint.RGB(0.92, 0.94, 0.98)}),
			},
		},
	}
}

func pauseLabel(m Model) string {
	if m.Paused() {
		return "resume"
	}
	return "pause"
}
