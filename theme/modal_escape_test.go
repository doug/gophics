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

// escapeApp opens a modal from a button, or from a field's OnSubmit (which
// leaves the field focused), so the Escape route can be exercised with the
// keyboard target inside or beneath the modal.
type escapeApp struct {
	open func(ctx widget.Ctx) // ShowDialog / ShowBottomSheet with some content
}

func (a escapeApp) CreateState() widget.State { return &escapeState{} }

type escapeState struct{ widget.StateBase[escapeApp] }

func (s *escapeState) Build(ctx widget.Ctx) widget.Widget {
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Fill{Color: theme.Light().Bg,
		Child: widget.Center(widget.Column(
			theme.Button{Label: "Open", OnTap: func() { s.W().open(ctx) }},
			widget.Sized{H: 20},
			widget.Sized{W: 200, Child: theme.Field{Placeholder: "Search", OnSubmit: func(string) { s.W().open(ctx) }}},
		))}}
}

func escapeHarness(t *testing.T, open func(widget.Ctx)) *apptest.App {
	t.Helper()
	a := apptest.New(t, escapeApp{open: open}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}))
	a.Render()
	return a
}

func fieldDialog(ctx widget.Ctx) {
	theme.ShowDialog(ctx, widget.Column(
		theme.Body("Dialog body"),
		widget.Sized{W: 200, Child: theme.Field{Autofocus: true, Placeholder: "Name"}},
	))
}

// A dialog whose content autofocuses a field: the scrim never holds focus, so
// its own OnKey never sees Escape. The doc promises Escape dismisses anyway.
func TestDialogEscapeWithFocusedField(t *testing.T) {
	a := escapeHarness(t, fieldDialog)
	a.TapLabel("Open")
	settle(a)
	if !a.HasText("Dialog body") {
		t.Fatal("dialog did not open")
	}
	a.Type("x") // the field, not the scrim, is the keyboard target
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Dialog body") {
		t.Fatal("Escape did not dismiss a dialog with a focused field")
	}
}

// A dialog opened from a field's OnSubmit: the field beneath the dialog keeps
// focus, and Escape used to collapse its selection instead of closing.
func TestDialogEscapeOpenedFromFocusedField(t *testing.T) {
	a := escapeHarness(t, func(ctx widget.Ctx) { theme.ShowDialog(ctx, theme.Body("Dialog body")) })
	a.TapText("Search")
	a.Type("go")
	a.Key(shell.KeyEnter)
	settle(a)
	if !a.HasText("Dialog body") {
		t.Fatal("OnSubmit did not open the dialog")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Dialog body") {
		t.Fatal("Escape did not dismiss a dialog opened over a focused field")
	}
	// With the dialog gone Escape is the field's again: nothing modal may
	// stay registered to swallow it.
	if a.Owner().TopModal() != nil {
		t.Fatal("a modal layer stayed registered after the dialog closed")
	}
}

func TestBottomSheetEscapeWithFocusedField(t *testing.T) {
	a := escapeHarness(t, func(ctx widget.Ctx) {
		theme.ShowBottomSheet(ctx, widget.Column(
			theme.Title("Sheet Title"),
			widget.Sized{W: 200, Child: theme.Field{Autofocus: true, Placeholder: "Name"}},
		))
	})
	a.TapLabel("Open")
	settle(a)
	if !a.HasText("Sheet Title") {
		t.Fatal("sheet did not open")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Sheet Title") {
		t.Fatal("Escape did not dismiss a bottom sheet with a focused field")
	}
}

// A sheet on its way out no longer claims Escape. The exit slide keeps the
// entry mounted for the length of the animation, and its Modal used to stay
// registered with a close that had become a no-op, so an Escape in that
// window was consumed and dropped instead of reaching the dialog beneath.
func TestBottomSheetDeclinesEscapeWhileExiting(t *testing.T) {
	a := escapeHarness(t, func(ctx widget.Ctx) {
		theme.ShowDialog(ctx, widget.Column(
			theme.Body("Outer dialog"),
			theme.Button{Label: "Sheet", OnTap: func() {
				theme.ShowBottomSheet(ctx, theme.Title("Sheet Title"))
			}},
		))
	})
	a.TapLabel("Open")
	settle(a)
	a.TapLabel("Sheet")
	settle(a)
	if !a.HasText("Sheet Title") {
		t.Fatal("sheet did not open")
	}
	a.Key(shell.KeyEscape) // starts the exit slide
	a.Step(1.0 / 60)
	a.Render()
	if !a.HasText("Sheet Title") {
		t.Fatal("the sheet left in one frame; the test needs the exit window")
	}
	a.Key(shell.KeyEscape) // reaches the dialog, not the departing sheet
	settle(a)
	if a.HasText("Outer dialog") {
		t.Fatal("Escape during the sheet's exit was swallowed instead of closing the dialog beneath")
	}
	if a.HasText("Sheet Title") {
		t.Fatal("the sheet never finished leaving")
	}
}

// Two stacked dialogs: Escape closes the top one only.
func TestDialogEscapeClosesTopmostOnly(t *testing.T) {
	a := escapeHarness(t, func(ctx widget.Ctx) {
		theme.ShowDialog(ctx, widget.Column(
			theme.Body("Outer dialog"),
			theme.Button{Label: "Nest", OnTap: func() { fieldDialog(ctx) }},
		))
	})
	a.TapLabel("Open")
	settle(a)
	a.TapLabel("Nest")
	settle(a)
	if !a.HasText("Dialog body") || !a.HasText("Outer dialog") {
		t.Fatal("nested dialogs did not open")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Dialog body") {
		t.Fatal("Escape did not close the inner dialog")
	}
	if !a.HasText("Outer dialog") {
		t.Fatal("Escape closed the outer dialog along with the inner one")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Outer dialog") {
		t.Fatal("second Escape did not close the outer dialog")
	}
}

// A suggestion list open inside a dialog is the innermost thing to escape
// from: the first Escape closes the list and leaves the dialog up, the second
// closes the dialog. Before the list claimed the key, one Escape took the
// dialog down with the list still showing inside it.
func TestDialogEscapeClosesTheSuggestionListFirst(t *testing.T) {
	a := escapeHarness(t, func(ctx widget.Ctx) {
		theme.ShowDialog(ctx, widget.Column(
			theme.Body("Dialog body"),
			widget.Sized{W: 200, Child: widget.Autocomplete{
				Placeholder: "City",
				Suggest:     func(string) []string { return []string{"Paris", "Prague"} },
			}},
		))
	})
	a.TapLabel("Open")
	settle(a)
	a.TapLabel("City")
	a.Key(shell.KeyDown) // moving into the list opens it
	settle(a)
	if !a.HasText("Paris") {
		t.Fatal("the suggestion list did not open")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Paris") {
		t.Fatal("Escape did not close the suggestion list")
	}
	if !a.HasText("Dialog body") {
		t.Fatal("Escape closed the dialog instead of the list inside it")
	}
	a.Key(shell.KeyEscape)
	settle(a)
	if a.HasText("Dialog body") {
		t.Fatal("second Escape did not close the dialog")
	}
}
