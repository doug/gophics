package main

import (
	"testing"

	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/layout"
)

// TestSemantics asserts on the accessibility tree the app publishes — the same
// tree the macOS, Windows, Linux, web and Android backends hand to the
// platform. It needs no window, no GPU and no assistive technology installed.
func TestSemantics(t *testing.T) {
	a := apptest.New(t, Rating{}, apptest.WithConfig(config()))

	// Every star is named. A control announced as a bare "radio button" is
	// the defect this test exists to catch.
	for _, label := range []string{
		"1 of 5 stars", "2 of 5 stars", "3 of 5 stars",
		"4 of 5 stars", "5 of 5 stars",
	} {
		a.AssertLabel(label)
	}

	// The role is what decides how the platform announces and operates it.
	if n := a.MustNode("3 of 5 stars"); n.Role != layout.RoleRadio {
		t.Errorf("star role = %v, want RoleRadio", n.Role)
	}

	// The heading is the landmark a screen-reader user navigates by.
	if a.Role(layout.RoleHeading) == nil {
		t.Errorf("no heading in the tree. Labels: %v", a.Labels())
	}

	// Tapping by label is how the test finds the control, rather than by a
	// pixel that moves the moment the layout changes — and it only works
	// because the control is labelled, so this doubles as the check.
	a.TapLabel("4 of 5 stars")
	a.AssertLabel("Rated 4 of 5")

	// Checked has to follow the state, or a screen reader reads the rating
	// the user had before the tap.
	n := a.MustNode("4 of 5 stars")
	if n.Checked == nil || !*n.Checked {
		t.Errorf("4th star Checked = %v, want true", n.Checked)
	}
	if n := a.MustNode("5 of 5 stars"); n.Checked == nil || *n.Checked {
		t.Errorf("5th star Checked = %v, want false", n.Checked)
	}

	// The decorative rule is hidden, so it must not appear as a node. Nothing
	// labels it, which is exactly why a positive assertion is impossible here
	// and the count of nodes is the available check.
	for _, got := range a.Nodes() {
		if got.Role == layout.RoleImage {
			t.Errorf("decorative canvas reached the semantics tree as %v", got)
		}
	}
}
