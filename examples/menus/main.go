// Command menus demonstrates the native menu bar capability.
//
// The menu bar is the one desktop affordance an app cannot draw for itself: on
// macOS it lives outside the window entirely. So this publishes a description
// and the platform builds the real thing — which is also why it is worth
// looking at rather than only testing.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// Item IDs. They travel to the platform menu and back through invoke, so they
// must outlive the build that created them — which is why the capability takes
// IDs rather than closures.
const (
	itemNew = iota + 1
	itemOpen
	itemSaveA
	itemSaveB
	itemBold
	itemItalic
)

type Menus struct{}

func (Menus) CreateState() widget.State { return &menusState{} }

type menusState struct {
	widget.StateBase[Menus]
	published bool
	trayShown bool
	last      string
	bold      bool
	italic    bool
}

func (s *menusState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)

	// A tray item, if the platform has one. Same shape as the menu bar: it is
	// published once and persists until replaced, because it outlives the
	// build that described it.
	if tr := ctx.Tray(); tr != nil && !s.trayShown {
		s.trayShown = true
		tr.Show(shell.TrayItem{
			Title:   "◆ gophics",
			Tooltip: "gophics menu demo",
			Menu: []shell.MenuItem{
				{ID: itemNew, Title: "New"},
				{Separator: true},
				{Role: shell.RoleQuit, Title: "Quit"},
			},
		}, func(id int) { s.SetState(func() { s.last = label(id) + " (tray)" }) })
	}

	// Publish once. The bar persists until replaced, so republishing every
	// frame would rebuild the platform's menus sixty times a second.
	if m := ctx.Menus(); m != nil && !s.published {
		s.published = true
		m.SetBar(menuBar(s), func(id int) {
			s.SetState(func() {
				switch id {
				case itemBold:
					s.bold = !s.bold
				case itemItalic:
					s.italic = !s.italic
				}
				s.last = label(id)
			})
		})
	}

	status := "choose something from the menu bar"
	if s.last != "" {
		status = "you chose: " + s.last
	}
	marks := ""
	if s.bold {
		marks += " bold"
	}
	if s.italic {
		marks += " italic"
	}
	if marks != "" {
		status += "   ·  " + marks
	}

	body := widget.Column(
		theme.Title("Native menus"),
		widget.Sized{H: 10},
		theme.Body(status),
	)
	if ctx.Menus() == nil {
		body = widget.Column(
			theme.Title("Native menus"),
			widget.Sized{H: 10},
			theme.Body("this platform has no menu bar — ctx.Menus() is nil"),
		)
	}
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Center(body)}}
}

// menuBar describes the bar. Roles are left to the OS: Quit really quits, and
// About and Preferences are placed where the platform expects them.
func menuBar(s *menusState) []shell.Menu {
	return []shell.Menu{
		{Title: "File", Items: []shell.MenuItem{
			{ID: itemNew, Title: "New"},
			{ID: itemOpen, Title: "Open…"},
			{Separator: true},
			{Title: "Save As", Submenu: []shell.MenuItem{
				{ID: itemSaveA, Title: "Markdown"},
				{ID: itemSaveB, Title: "Plain text"},
			}},
			{Separator: true},
			{Role: shell.RoleQuit, Title: "Quit"},
		}},
		{Title: "Format", Items: []shell.MenuItem{
			{ID: itemBold, Title: "Bold"},
			{ID: itemItalic, Title: "Italic"},
			{Separator: true},
			{ID: 0, Title: "Unavailable", Disabled: true},
		}},
	}
}

func label(id int) string {
	switch id {
	case itemNew:
		return "New"
	case itemOpen:
		return "Open"
	case itemSaveA:
		return "Save As ▸ Markdown"
	case itemSaveB:
		return "Save As ▸ Plain text"
	case itemBold:
		return "Bold"
	case itemItalic:
		return "Italic"
	}
	return fmt.Sprintf("item %d", id)
}

func main() {
	if err := app.Run(Menus{}, app.Config{
		Title:      "gophics menus",
		Size:       geom.Size{W: 460, H: 240},
		Background: theme.Light().Bg,
		Font:       goregular.TTF,
	}); err != nil {
		log.Fatal(err)
	}
}
