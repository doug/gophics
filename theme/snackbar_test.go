package theme_test

import (
	"testing"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// snackApp opens a snackbar from a centered button.
type snackApp struct{ hook func(*snackState) }

func (a snackApp) CreateState() widget.State { s := &snackState{}; s.hook = a.hook; return s }

type snackState struct {
	widget.StateBase[snackApp]
	hook    func(*snackState)
	open    func(ctx widget.Ctx)
	dismiss func()
	acted   bool
}

func (s *snackState) Init(widget.Ctx) { s.hook(s) }

func (s *snackState) Build(widget.Ctx) widget.Widget {
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Fill{Color: theme.Light().Bg,
		Child: widget.Center(snackOpener{onOpen: func(ctx widget.Ctx) {
			if s.open != nil {
				s.open(ctx)
			}
		}})}}
}

type snackOpener struct{ onOpen func(widget.Ctx) }

func (o snackOpener) Build(ctx widget.Ctx) widget.Widget {
	return theme.Button{Label: "Open", OnTap: func() { o.onOpen(ctx) }}
}

func snackHarness(t *testing.T, open func(s *snackState, ctx widget.Ctx)) (*app.Headless, *snackState) {
	t.Helper()
	var st *snackState
	h, err := app.NewHeadless(snackApp{hook: func(s *snackState) { st = s }}, app.Config{
		Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	st.open = func(ctx widget.Ctx) { open(st, ctx) }
	h.Render()
	return h, st
}

// stepFor advances roughly d worth of frames (plus a margin) at 60fps.
func stepFor(h *app.Headless, d time.Duration) {
	n := int(d.Seconds()*60) + 30
	for i := 0; i < n; i++ {
		h.Step(1.0 / 60)
		h.Render()
	}
}

func TestSnackbarShowsAndAutoDismisses(t *testing.T) {
	h, _ := snackHarness(t, func(s *snackState, ctx widget.Ctx) {
		s.dismiss = theme.ShowSnackbar(ctx, "Saved changes", theme.WithDuration(120*time.Millisecond))
	})
	h.Tap(geom.Pt{X: 200, Y: 300})
	// Let it enter and hold, but not yet time out.
	for i := 0; i < 4; i++ {
		h.Step(1.0 / 60)
		h.Render()
	}
	if !hasLabel(h, "Saved changes") {
		t.Fatal("snackbar did not appear")
	}
	// Advance past the hold timeout plus the exit animation.
	stepFor(h, 500*time.Millisecond)
	if hasLabel(h, "Saved changes") {
		t.Fatal("snackbar did not auto-dismiss after its timeout")
	}
}

func TestSnackbarActionDismisses(t *testing.T) {
	h, st := snackHarness(t, func(s *snackState, ctx widget.Ctx) {
		s.dismiss = theme.ShowSnackbar(ctx, "Item deleted",
			theme.WithAction("Undo", func() { s.acted = true }),
			theme.WithDuration(10*time.Second)) // long, so the timeout can't interfere
	})
	h.Tap(geom.Pt{X: 200, Y: 300})
	settle(h)
	if !hasLabel(h, "Item deleted") {
		t.Fatal("snackbar did not appear")
	}
	if !hasLabel(h, "Undo") {
		t.Fatal("action label missing")
	}
	// Tap the Undo action by its semantic bounds.
	if !tapLabel(h, "Undo") {
		t.Fatal("could not tap the Undo action")
	}
	settle(h)
	if !st.acted {
		t.Fatal("action tap did not fire onTap")
	}
	if hasLabel(h, "Item deleted") {
		t.Fatal("snackbar should close after the action")
	}
}

func TestSnackbarProgrammaticDismiss(t *testing.T) {
	h, st := snackHarness(t, func(s *snackState, ctx widget.Ctx) {
		s.dismiss = theme.ShowSnackbar(ctx, "Uploading", theme.WithDuration(10*time.Second))
	})
	h.Tap(geom.Pt{X: 200, Y: 300})
	settle(h)
	if !hasLabel(h, "Uploading") {
		t.Fatal("snackbar missing")
	}
	if st.dismiss == nil {
		t.Fatal("ShowSnackbar returned no dismiss func")
	}
	st.dismiss()
	settle(h)
	if hasLabel(h, "Uploading") {
		t.Fatal("programmatic dismiss did not close the snackbar")
	}
}
