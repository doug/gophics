package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// pickerApp hosts a Tabs, a Segmented and a Switch, controlled from state so
// a test can drive selection through semantics and watch the rebuild.
type pickerApp struct{ hook func(*pickerState) }

func (a pickerApp) CreateState() widget.State { return &pickerState{hook: a.hook} }

type pickerState struct {
	widget.StateBase[pickerApp]
	hook    func(*pickerState)
	tab     int
	segment int
	on      bool
}

func (s *pickerState) Init(widget.Ctx) { s.hook(s) }

func (s *pickerState) Build(widget.Ctx) widget.Widget {
	col := widget.Column(
		theme.Tabs{Tabs: []string{"All", "Unread", "Flagged"}, Selected: s.tab,
			OnChange: func(i int) { s.SetState(func() { s.tab = i }) }},
		widget.Sized{H: 20},
		theme.Segmented{Options: []string{"Day", "Week", "Month"}, Selected: s.segment,
			OnChange: func(i int) { s.SetState(func() { s.segment = i }) }},
		widget.Sized{H: 20},
		theme.Switch{On: s.on, Label: "Wi-Fi", OnChange: func(v bool) { s.SetState(func() { s.on = v }) }},
	)
	col.CrossAlign = layout.CrossStretch
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Padding{All: 20, Child: col}}
}

func pickerHarness(t *testing.T) (*apptest.App, *pickerState) {
	t.Helper()
	var st *pickerState
	a := apptest.New(t, pickerApp{hook: func(s *pickerState) { st = s }},
		apptest.WithConfig(app.Config{
			Size: geom.Size{W: 360, H: 300}, Font: goregular.TTF,
			FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
		}))
	a.Render()
	return a, st
}

func nodesWithRole(a *apptest.App, r layout.Role) []layout.SemNode {
	var out []layout.SemNode
	for _, n := range a.Nodes() {
		if n.Role == r {
			out = append(out, n)
		}
	}
	return out
}

// Tabs used to be one hit area whose index came from the pointer's x, so a
// screen reader saw one unlabeled button and activating it re-reported the
// last pressed tab. Each tab is now its own node with the tab role, the
// active one marked selected, and activation picks that tab.
func TestTabsExposeEachTabToAssistiveTech(t *testing.T) {
	a, st := pickerHarness(t)
	tabs := nodesWithRole(a, layout.RoleTab)
	if len(tabs) != 3 {
		t.Fatalf("%d tab nodes, want 3: labels %v", len(tabs), a.Labels())
	}
	for i, n := range tabs {
		if n.Selected != (i == 0) {
			t.Errorf("tab %q selected=%v before any change", n.Label, n.Selected)
		}
	}
	if tabs[2].Label != "Flagged" || tabs[2].OnActivate == nil {
		t.Fatalf("third tab is %q with activate=%v", tabs[2].Label, tabs[2].OnActivate != nil)
	}
	tabs[2].OnActivate()
	a.Render()
	if st.tab != 2 {
		t.Fatalf("activating the third tab selected %d", st.tab)
	}
	if again := nodesWithRole(a, layout.RoleTab); !again[2].Selected || again[0].Selected {
		t.Fatal("selection did not move to the activated tab")
	}
}

// Segmented options are radios: one of a set, the chosen one checked.
func TestSegmentedExposesOptionsAsRadios(t *testing.T) {
	a, st := pickerHarness(t)
	opts := nodesWithRole(a, layout.RoleRadio)
	if len(opts) != 3 {
		t.Fatalf("%d radio nodes, want 3: labels %v", len(opts), a.Labels())
	}
	for i, n := range opts {
		if n.Checked == nil || *n.Checked != (i == 0) {
			t.Errorf("option %q checked=%v before any change", n.Label, n.Checked)
		}
	}
	opts[1].OnActivate()
	a.Render()
	if st.segment != 1 {
		t.Fatalf("activating the second option selected %d", st.segment)
	}
}

// Under reduce motion the sliding indicators and the switch knob jump: the
// rebuild that changes the selection leaves nothing animating.
func TestSelectionControlsJumpUnderReduceMotion(t *testing.T) {
	a, st := pickerHarness(t)
	a.Owner().ReduceMotion = true
	a.Owner().RebuildAll()
	a.Render()
	settle(a)

	st.SetState(func() { st.tab, st.segment, st.on = 2, 1, true })
	a.Render()
	if a.Owner().TickersActive() {
		t.Fatal("a selection change started an animation under reduce motion")
	}
	// Without the preference the same change animates, so the check above
	// is testing the preference and not an absence of animation.
	a.Owner().ReduceMotion = false
	st.SetState(func() { st.tab, st.segment, st.on = 0, 0, false })
	a.Render()
	if !a.Owner().TickersActive() {
		t.Fatal("a selection change did not animate with motion allowed")
	}
}

// The Dropdown control tells a screen reader whether its list is open.
func TestDropdownExposesExpandedState(t *testing.T) {
	h, _ := ddHarness(t)
	expanded := func() bool {
		n := h.NodeContaining("Pick a size")
		if n == nil || n.Expanded == nil {
			t.Fatalf("dropdown node missing or not expandable: %v", h.Labels())
		}
		return *n.Expanded
	}
	if expanded() {
		t.Fatal("closed dropdown reports expanded")
	}
	h.TapText("Pick a size")
	h.Render()
	if !expanded() {
		t.Fatal("open dropdown does not report expanded")
	}
}
