//go:build !js

package desktop

import (
	"testing"

	"github.com/doug/gophics/internal/gfx/gogpu"
	"github.com/doug/gophics/shell"
)

// convertItem casts shell.MenuRole straight to gogpu.MenuRole, so the two
// enums must agree value for value. They are declared independently, in
// different trees, both as bare iota blocks — nothing makes them agree except
// that they currently happen to.
//
// Getting this wrong is silent and specific: insert one role in the middle of
// either list and every role after it shifts by one, so Quit reaches the
// platform as Close and About as Preferences. The menu still builds, the items
// still appear, and the wrong ones fire.
func TestMenuRolesMatchTheWindowingLayer(t *testing.T) {
	roles := []struct {
		name   string
		ours   shell.MenuRole
		theirs gogpu.MenuRole
	}{
		{"None", shell.RoleNone, gogpu.RoleNone},
		{"About", shell.RoleAbout, gogpu.RoleAbout},
		{"Preferences", shell.RolePreferences, gogpu.RolePreferences},
		{"Services", shell.RoleServices, gogpu.RoleServices},
		{"Hide", shell.RoleHide, gogpu.RoleHide},
		{"HideOthers", shell.RoleHideOthers, gogpu.RoleHideOthers},
		{"ShowAll", shell.RoleShowAll, gogpu.RoleShowAll},
		{"Quit", shell.RoleQuit, gogpu.RoleQuit},
		{"Close", shell.RoleClose, gogpu.RoleClose},
		{"Minimize", shell.RoleMinimize, gogpu.RoleMinimize},
		{"Zoom", shell.RoleZoom, gogpu.RoleZoom},
		{"FullScreen", shell.RoleFullScreen, gogpu.RoleFullScreen},
		{"BringAllToFront", shell.RoleBringAllToFront, gogpu.RoleBringAllToFront},
	}
	for _, r := range roles {
		if gogpu.MenuRole(r.ours) != r.theirs {
			t.Errorf("Role%s: shell=%d but gogpu=%d — the cast in convertItem "+
				"would send this to the platform as a different role",
				r.name, r.ours, r.theirs)
		}
	}

	// A role added to one list and not the other is the failure this guards,
	// and it only shows up as a count.
	if got, want := int(shell.RoleBringAllToFront)+1, len(roles); got != want {
		t.Errorf("shell declares %d roles but this test covers %d — a new role "+
			"was added without extending the table", got, want)
	}
}

// A menu bar is a list of top-level menus, but the platform underneath models
// it as one root whose items each carry a submenu. Getting the nesting wrong
// produces a bar with one entry, or entries with nothing under them.
func TestSetBarNestsMenusUnderOneRoot(t *testing.T) {
	bar := []shell.Menu{
		{Title: "File", Items: []shell.MenuItem{{ID: 1, Title: "Open"}}},
		{Title: "Edit", Items: []shell.MenuItem{{ID: 2, Title: "Undo"}}},
	}

	root := gogpu.NewMenu()
	for _, menu := range bar {
		root.AddItem(gogpu.MenuItem{
			Title:   menu.Title,
			Submenu: convertMenu(menu.Title, menu.Items, nil),
		})
	}

	if len(root.Items) != 2 {
		t.Fatalf("root has %d items, want one per top-level menu (2)", len(root.Items))
	}
	for i, want := range []string{"File", "Edit"} {
		if root.Items[i].Title != want {
			t.Errorf("root item %d is %q, want %q", i, root.Items[i].Title, want)
		}
		if root.Items[i].Submenu == nil {
			t.Fatalf("%q has no submenu, so its items are unreachable", want)
		}
	}
	if n := len(root.Items[0].Submenu.Items); n != 1 {
		t.Errorf("File has %d items, want 1", n)
	}
}

// The ID an app sets is what comes back when the user picks the item. This is
// the whole contract of the capability, and it survives the widget tree that
// described it being rebuilt.
func TestConvertItemInvokesWithItsOwnID(t *testing.T) {
	var got []int
	invoke := func(id int) { got = append(got, id) }

	items := []shell.MenuItem{
		{ID: 7, Title: "Open"},
		{ID: 9, Title: "Save"},
	}
	m := convertMenu("File", items, invoke)

	if len(m.Items) != 2 {
		t.Fatalf("converted %d items, want 2", len(m.Items))
	}
	for i, it := range m.Items {
		if it.Action == nil {
			t.Fatalf("item %d has no action, so choosing it does nothing", i)
		}
		it.Action()
	}
	if len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Errorf("invoked with %v, want [7 9] — each item must close over its "+
			"own ID, not the loop variable", got)
	}
}

// An item with a role is placed and performed by the operating system: Quit
// really quits. Attaching an action as well means the app's handler and the
// platform's both run.
func TestRoleItemsCarryNoAction(t *testing.T) {
	fired := 0
	it := convertItem(shell.MenuItem{ID: 3, Title: "Quit", Role: shell.RoleQuit},
		func(int) { fired++ })

	if it.Action != nil {
		t.Error("a role item was given an action; the OS already performs it, " +
			"so the app's handler would run as well as the platform's")
	}
	if it.Role != gogpu.RoleQuit {
		t.Errorf("role reached the platform as %d, want Quit (%d)", it.Role, gogpu.RoleQuit)
	}
}

// Separators are structural: every other field is meaningless on them, and an
// action on one is unreachable.
func TestSeparatorsConvertToSeparators(t *testing.T) {
	it := convertItem(shell.MenuItem{Separator: true, Title: "ignored", ID: 5},
		func(int) { t.Error("a separator invoked its action") })
	if !it.Separator {
		t.Fatal("a separator did not survive conversion")
	}
	if it.Action != nil {
		t.Error("a separator was given an action")
	}
}

// Submenus nest to arbitrary depth, and an item that has one is a container:
// it opens rather than firing.
func TestSubmenusNestAndDoNotFire(t *testing.T) {
	var got []int
	it := convertItem(shell.MenuItem{
		Title: "Recent",
		Submenu: []shell.MenuItem{
			{ID: 11, Title: "a.txt"},
			{Title: "More", Submenu: []shell.MenuItem{{ID: 12, Title: "b.txt"}}},
		},
	}, func(id int) { got = append(got, id) })

	if it.Submenu == nil {
		t.Fatal("submenu was dropped")
	}
	if it.Action != nil {
		t.Error("an item with a submenu was also given an action; it should open, not fire")
	}
	if n := len(it.Submenu.Items); n != 2 {
		t.Fatalf("submenu has %d items, want 2", n)
	}
	deep := it.Submenu.Items[1].Submenu
	if deep == nil || len(deep.Items) != 1 {
		t.Fatal("the second level of nesting was lost")
	}
	deep.Items[0].Action()
	if len(got) != 1 || got[0] != 12 {
		t.Errorf("nested item invoked with %v, want [12]", got)
	}
}

// Disabled has to survive, or a menu shows every item as available.
func TestDisabledSurvivesConversion(t *testing.T) {
	if !convertItem(shell.MenuItem{Title: "Paste", Disabled: true}, nil).Disabled {
		t.Error("Disabled was dropped in conversion")
	}
}

// A nil invoke is what the capability sees before an app installs a handler.
// Converting must not panic, and the items must simply do nothing.
func TestNilInvokeProducesNoAction(t *testing.T) {
	it := convertItem(shell.MenuItem{ID: 1, Title: "Open"}, nil)
	if it.Action != nil {
		t.Error("an action was attached with no invoke to call")
	}
}
