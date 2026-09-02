package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// sheetApp opens a bottom sheet from a centered button; the hook captures the
// live state so tests can drive it and read back the dismiss closer.
type sheetApp struct{ hook func(*sheetState) }

func (a sheetApp) CreateState() widget.State { s := &sheetState{}; s.hook = a.hook; return s }

type sheetState struct {
	widget.StateBase[sheetApp]
	hook    func(*sheetState)
	dismiss func()
}

func (s *sheetState) Init(widget.Ctx) { s.hook(s) }

func (s *sheetState) open(ctx widget.Ctx) {
	s.dismiss = theme.ShowBottomSheet(ctx, widget.Column(
		theme.Title("Sheet Title"),
		widget.Sized{H: 8},
		widget.Text{Value: "Sheet body row", Size: 14},
	))
}

func (s *sheetState) Build(widget.Ctx) widget.Widget {
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Fill{Color: theme.Light().Bg,
		Child: widget.Center(sheetOpener{onOpen: s.open})}}
}

type sheetOpener struct{ onOpen func(widget.Ctx) }

func (o sheetOpener) Build(ctx widget.Ctx) widget.Widget {
	return theme.Button{Label: "Open", OnTap: func() { o.onOpen(ctx) }}
}

func sheetHarness(t *testing.T) (*apptest.App, *sheetState) {
	t.Helper()
	var st *sheetState
	a := apptest.New(t, sheetApp{hook: func(s *sheetState) { st = s }},
		apptest.WithConfig(app.Config{
			Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF,
			FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
		}),
		apptest.Scale(1),
	)
	a.Render()
	return a, st
}

// settle advances a batch of frames so entrance/exit animations complete.
func settle(h *apptest.App) {
	for range 60 {
		h.Step(1.0 / 60)
		h.Render()
	}
}

func TestBottomSheetShowsAndScrimDismisses(t *testing.T) {
	h, _ := sheetHarness(t)
	h.Tap(geom.Pt{X: 200, Y: 300}) // centered "Open" button
	settle(h)
	if !h.HasText("Sheet Title") {
		t.Fatal("bottom sheet did not appear")
	}
	// Top-left is on the scrim, above the bottom-anchored sheet → dismiss.
	h.Tap(geom.Pt{X: 10, Y: 10})
	settle(h)
	if h.HasText("Sheet Title") {
		t.Fatal("scrim tap did not dismiss the bottom sheet")
	}
}

func TestBottomSheetEscapeDismisses(t *testing.T) {
	h, _ := sheetHarness(t)
	h.Tap(geom.Pt{X: 200, Y: 300})
	settle(h)
	if !h.HasText("Sheet Title") {
		t.Fatal("bottom sheet missing")
	}
	h.Key(shell.KeyEscape)
	settle(h)
	if h.HasText("Sheet Title") {
		t.Fatal("Escape did not dismiss the bottom sheet")
	}
}

func TestBottomSheetProgrammaticDismiss(t *testing.T) {
	h, st := sheetHarness(t)
	h.Tap(geom.Pt{X: 200, Y: 300})
	settle(h)
	if !h.HasText("Sheet Title") {
		t.Fatal("bottom sheet missing")
	}
	if st.dismiss == nil {
		t.Fatal("ShowBottomSheet returned no dismiss func")
	}
	st.dismiss()
	settle(h)
	if h.HasText("Sheet Title") {
		t.Fatal("programmatic dismiss did not close the bottom sheet")
	}
}
