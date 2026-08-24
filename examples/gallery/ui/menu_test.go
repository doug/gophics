package ui

import (
	"testing"

	"github.com/doug/gophics/shell"
)

// fakeMenus captures what the app publishes instead of building a real menu bar.
type fakeMenus struct {
	bar    []shell.Menu
	invoke func(id int)
}

func (f *fakeMenus) SetBar(bar []shell.Menu, invoke func(id int)) {
	f.bar, f.invoke = bar, invoke
}

// Choosing a theme from the native menu must change the theme, the same as the
// in-app switcher does.
//
// This is the round trip the capability exists for: the platform calls invoke
// with an ID that outlived the build which described it, and the handler turns
// that back into widget state. The gallery is the only thing in the repo that
// publishes a menu at all, so without this the whole path is unexercised.
func TestMenuThemeSelectionUpdatesState(t *testing.T) {
	var st *galleryState
	rootHook = func(s *galleryState) { st = s }
	defer func() { rootHook = nil }()

	// Mount enough to get a state; the menu capability is nil headlessly, so
	// publishMenus returns early and the handler is driven directly below.
	galleryApp(t, Gallery{}).Render()
	if st == nil {
		t.Fatal("gallery root state never mounted")
	}

	for _, tc := range []struct {
		id   int
		want themeMode
		name string
	}{
		{menuThemeDark, modeDark, "Dark"},
		{menuThemeGlass, modeGlass, "Glass"},
		{menuThemeGlassDark, modeGlassDark, "Glass Dark"},
		{menuThemeLight, modeLight, "Light"},
	} {
		st.onMenu(tc.id)
		if st.mode != tc.want {
			t.Errorf("choosing %q left mode = %v, want %v", tc.name, st.mode, tc.want)
		}
	}
}

// An ID the gallery does not know must be ignored rather than falling through
// to a theme. IDs cross a platform boundary, so an unexpected one is possible.
func TestUnknownMenuIDIsIgnored(t *testing.T) {
	if _, ok := menuThemeMode(9999); ok {
		t.Error("an unknown menu ID mapped to a theme")
	}
	if _, ok := menuThemeMode(0); ok {
		t.Error("ID zero mapped to a theme; it is the unset value and must not select one")
	}
}

// The published bar is what the user actually sees. Getting the roles wrong is
// invisible in a screenshot and wrong on macOS, where the platform owns them.
func TestPublishedMenuBarShape(t *testing.T) {
	f := &fakeMenus{}
	s := &galleryState{}
	s.publishBar(f)

	if len(f.bar) != 3 {
		t.Fatalf("published %d top-level menus, want 3", len(f.bar))
	}
	if f.invoke == nil {
		t.Fatal("published a bar with no invoke; every item would be inert")
	}

	// The application menu must carry the roles the OS places itself.
	roles := map[shell.MenuRole]bool{}
	for _, it := range f.bar[0].Items {
		roles[it.Role] = true
	}
	for _, want := range []shell.MenuRole{shell.RoleAbout, shell.RoleQuit, shell.RoleHide} {
		if !roles[want] {
			t.Errorf("the application menu is missing role %d", want)
		}
	}

	// Every theme item must carry a non-zero ID, or choosing it does nothing.
	view := f.bar[1]
	if view.Title != "View" {
		t.Fatalf("second menu is %q, want View", view.Title)
	}
	n := 0
	for _, it := range view.Items {
		if it.Separator {
			continue
		}
		if it.ID == 0 {
			t.Errorf("View item %q has no ID, so choosing it is a no-op", it.Title)
		}
		if _, ok := menuThemeMode(it.ID); !ok {
			t.Errorf("View item %q has ID %d, which maps to no theme", it.Title, it.ID)
		}
		n++
	}
	if n != 4 {
		t.Errorf("View has %d actionable items, want one per theme (4)", n)
	}
}

// Every item the user can choose must reach a handler. An ID published in the
// bar but missing from the switch is the failure that leaves a menu item dead.
func TestEveryPublishedIDIsHandled(t *testing.T) {
	f := &fakeMenus{}
	(&galleryState{}).publishBar(f)

	var seen []int
	for _, m := range f.bar {
		for _, it := range m.Items {
			if it.Separator || it.Role != shell.RoleNone {
				continue
			}
			if it.ID != 0 {
				seen = append(seen, it.ID)
			}
		}
	}
	if len(seen) == 0 {
		t.Fatal("no actionable items found; this test would pass without checking anything")
	}
	for _, id := range seen {
		if _, ok := menuThemeMode(id); !ok {
			t.Errorf("ID %d is published but no handler claims it", id)
		}
	}
}
