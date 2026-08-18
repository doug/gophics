//go:build !js

package desktop

import (
	"testing"

	"github.com/doug/gophics/shell"
)

func TestConvertItemSeparator(t *testing.T) {
	got := convertItem(shell.MenuItem{Separator: true, Title: "ignored"}, nil)
	if !got.Separator {
		t.Error("a separator did not convert to one")
	}
}

// An item's ID is what comes back through invoke, so the closure must capture
// this item's ID and not the loop's last one — the classic way a menu ends up
// firing the wrong action for every row.
func TestConvertItemRoutesItsOwnID(t *testing.T) {
	var got []int
	invoke := func(id int) { got = append(got, id) }

	menu := convertMenu("File", []shell.MenuItem{
		{ID: 7, Title: "Open"},
		{ID: 9, Title: "Save"},
	}, invoke)

	if len(menu.Items) != 2 {
		t.Fatalf("converted %d items, want 2", len(menu.Items))
	}
	menu.Items[0].Action()
	menu.Items[1].Action()
	if len(got) != 2 || got[0] != 7 || got[1] != 9 {
		t.Errorf("invoked %v, want [7 9]", got)
	}
}

// A role is performed by the OS. Attaching an action as well would run the
// app's handler and the platform's — quitting twice, or worse.
func TestRoleItemsCarryNoAction(t *testing.T) {
	fired := false
	got := convertItem(shell.MenuItem{Role: shell.RoleQuit, Title: "Quit"},
		func(int) { fired = true })
	if got.Action != nil {
		got.Action()
		if fired {
			t.Error("a role item invoked the app's handler as well as the OS's")
		}
		t.Error("a role item should carry no action")
	}
	if got.Role != 7 { // gogpu.RoleQuit
		t.Errorf("role = %d, want RoleQuit", got.Role)
	}
}

func TestConvertNestedSubmenu(t *testing.T) {
	var got []int
	menu := convertMenu("File", []shell.MenuItem{
		{Title: "Recent", Submenu: []shell.MenuItem{
			{ID: 3, Title: "a.txt"},
			{ID: 4, Title: "b.txt"},
		}},
	}, func(id int) { got = append(got, id) })

	if len(menu.Items) != 1 || menu.Items[0].Submenu == nil {
		t.Fatal("submenu did not convert")
	}
	sub := menu.Items[0].Submenu
	if len(sub.Items) != 2 {
		t.Fatalf("submenu has %d items, want 2", len(sub.Items))
	}
	sub.Items[1].Action()
	if len(got) != 1 || got[0] != 4 {
		t.Errorf("nested invoke gave %v, want [4]", got)
	}
}

// Disabled has to survive the trip, or a greyed-out item is clickable.
func TestDisabledSurvives(t *testing.T) {
	if !convertItem(shell.MenuItem{Title: "Undo", Disabled: true}, nil).Disabled {
		t.Error("Disabled was lost in conversion")
	}
}

// A nil invoke must not produce a nil-dereferencing action.
func TestNilInvokeIsSafe(t *testing.T) {
	it := convertItem(shell.MenuItem{ID: 1, Title: "Open"}, nil)
	if it.Action != nil {
		t.Error("an item got an action with no invoke to call")
	}
}
