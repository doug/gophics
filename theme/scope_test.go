package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// scopeModel stands in for the app-level value a dialog needs — a session, a
// client — provided in the tree below the root.
type scopeModel struct{ name string }

// modelDialog is dialog content that reads the model with MustOf, which
// panicked inside every dialog before overlay entries took the opener's scope.
type modelDialog struct{}

func (modelDialog) Build(ctx widget.Ctx) widget.Widget {
	return theme.Body("hello " + ctx.MustOf[*scopeModel]().name)
}

type modelOpener struct{}

func (modelOpener) Build(ctx widget.Ctx) widget.Widget {
	return theme.Button{Label: "Open", OnTap: func() { theme.ShowDialog(ctx, modelDialog{}) }}
}

func scopeConfig() apptest.Option {
	return apptest.WithConfig(app.Config{
		Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	})
}

func TestShowDialogSeesProvideFromOpener(t *testing.T) {
	root := widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Provide[*scopeModel]{
		Value: &scopeModel{name: "doug"},
		Child: widget.Fill{Color: theme.Light().Bg, Child: widget.Center(modelOpener{})},
	}}
	a := apptest.New(t, root, scopeConfig(), apptest.Scale(1))
	a.Render()

	a.TapLabel("Open")
	a.Render()
	a.AssertText("hello doug")
}

// navHome/navDetail/backDialog: a detail page opens a confirm dialog whose
// button pops the navigator — the dialog must reach the page's Nav.
type navHome struct{ nav *widget.Nav }

func (h navHome) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	*h.nav = nav
	return widget.Center(theme.Button{Label: "Go", OnTap: func() { nav.Push(navDetail{}) }})
}

type navDetail struct{}

func (navDetail) Build(ctx widget.Ctx) widget.Widget {
	return widget.Center(theme.Button{Label: "Ask", OnTap: func() {
		var dismiss func()
		dismiss = theme.ShowDialog(ctx, backDialog{dismiss: &dismiss})
	}})
}

type backDialog struct{ dismiss *func() }

func (d backDialog) Build(ctx widget.Ctx) widget.Widget {
	nav := ctx.MustOf[widget.Nav]()
	return theme.Button{Label: "Back", OnTap: func() {
		(*d.dismiss)()
		nav.Pop()
	}}
}

func settleNav(a *apptest.App) {
	for range 60 {
		a.Step(1.0 / 60)
		a.Render()
	}
}

func TestDialogButtonPopsOpenerNavigator(t *testing.T) {
	var nav widget.Nav
	root := widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Fill{
		Color: theme.Light().Bg, Child: widget.Navigator{Home: navHome{nav: &nav}},
	}}
	a := apptest.New(t, root, scopeConfig(), apptest.Scale(1))
	a.Render()

	a.TapLabel("Go")
	settleNav(a)
	if nav.Depth() != 2 {
		t.Fatalf("setup: depth = %d, want 2", nav.Depth())
	}
	a.TapLabel("Ask")
	a.Render()
	a.AssertLabel("Back")

	a.TapLabel("Back")
	settleNav(a)
	if nav.Depth() != 1 {
		t.Fatalf("depth after the dialog's Back = %d, want 1", nav.Depth())
	}
	a.AssertNoLabel("Back")
	a.AssertLabel("Go")
}
