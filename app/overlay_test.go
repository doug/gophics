package app

import (
	"image/png"
	"os"
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type dlgApp struct{ hook func(*dlgState) }

func (a dlgApp) CreateState() widget.State { s := &dlgState{}; s.hook = a.hook; return s }

type dlgState struct {
	widget.StateBase[dlgApp]
	hook      func(*dlgState)
	dismiss   func()
	confirmed bool
}

func (s *dlgState) Init(widget.Ctx) { s.hook(s) }

func (s *dlgState) open(ctx widget.Ctx) {
	s.dismiss = theme.ShowDialog(ctx, widget.Column(
		theme.Title("Delete?"),
		widget.Sized{H: 12},
		theme.Body("This cannot be undone."),
		widget.Sized{H: 16},
		theme.Button{Label: "Confirm", Primary: true, OnTap: func() {
			s.SetState(func() { s.confirmed = true })
			if s.dismiss != nil {
				s.dismiss()
			}
		}},
	))
}

func (s *dlgState) Build(widget.Ctx) widget.Widget {
	// Theme provided at the app root; the opener is a child so its ctx sees
	// the provider (realistic call site).
	return widget.Provide[theme.Theme]{Value: theme.Dark(), Child: widget.Fill{Color: theme.Dark().Bg,
		Child: widget.Center(opener{onOpen: s.open})}}
}

type opener struct{ onOpen func(widget.Ctx) }

func (o opener) Build(ctx widget.Ctx) widget.Widget {
	return theme.Button{Label: "Open", OnTap: func() { o.onOpen(ctx) }}
}

func dlgHarness(t *testing.T) (*Headless, *dlgState) {
	t.Helper()
	var st *dlgState
	h, err := NewHeadless(dlgApp{hook: func(s *dlgState) { st = s }}, Config{
		Size: geom.Size{W: 400, H: 400}, Font: goregular.TTF,
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h, st
}

func hasSemLabel(h *Headless, sub string) bool {
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if strings.Contains(n.Label, sub) {
			return true
		}
	}
	return false
}

func TestDialogShowsAndScrimDismisses(t *testing.T) {
	h, _ := dlgHarness(t)
	h.Tap(geom.Pt{X: 200, Y: 200}) // "Open" button (centered)
	h.Render()
	if !hasSemLabel(h, "Delete?") {
		t.Fatal("dialog did not appear")
	}
	// Tap top-left corner: on the scrim, outside the centered card → dismiss.
	h.Tap(geom.Pt{X: 10, Y: 10})
	h.Render()
	if hasSemLabel(h, "Delete?") {
		t.Fatal("scrim tap did not dismiss the dialog")
	}
}

func TestDialogEscapeDismisses(t *testing.T) {
	h, st := dlgHarness(t)
	_ = st
	h.Tap(geom.Pt{X: 200, Y: 200})
	h.Render()
	if !hasSemLabel(h, "Delete?") {
		t.Fatal("dialog missing")
	}
	h.Key(shell.KeyEscape)
	h.Render()
	if hasSemLabel(h, "Delete?") {
		t.Fatal("Escape did not dismiss")
	}
}

func TestDialogConfirmButton(t *testing.T) {
	h, st := dlgHarness(t)
	h.Tap(geom.Pt{X: 200, Y: 200})
	h.Render()
	// Tap the Confirm button at its semantic bounds. A positional scan is
	// fragile: a tap on the card's non-button area falls through to the modal
	// scrim and dismisses the dialog before the button is reached, and the card
	// height tracks the theme's type scale.
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if n.Role == layout.RoleButton && strings.Contains(n.Label, "Confirm") {
			h.Tap(geom.Pt{X: n.Rect.Min.X + n.Rect.Dx()/2, Y: n.Rect.Min.Y + n.Rect.Dy()/2})
			break
		}
	}
	if !st.confirmed {
		t.Fatal("Confirm button not triggered")
	}
	h.Render()
	if hasSemLabel(h, "Delete?") {
		t.Fatal("dialog should close after confirm")
	}
	if out := os.Getenv("GOPHICS_RENDER_OUT"); out != "" {
		h.Tap(geom.Pt{X: 200, Y: 200})
		img := h.Render()
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
		_ = paint.RGB
	}
}
