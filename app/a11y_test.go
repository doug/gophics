package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/widget"
)

type a11yApp struct{ activated *int }

func (a a11yApp) Build(widget.Ctx) widget.Widget {
	act := a.activated
	return widget.Padding{All: 10, Child: widget.Column(
		widget.Text{S: "Inbox"},
		widget.Interactive{Handler: widget.Handler{OnTap: func() { *act++ }},
			Child: widget.Padding{All: 6, Child: widget.Text{S: "Compose"}}},
	)}
}

func TestA11yTreeAndActivate(t *testing.T) {
	var activated int
	h, err := NewHeadless(a11yApp{activated: &activated}, Config{
		Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	nodes := h.Core.A11yTree(2)
	if len(nodes) < 3 {
		t.Fatalf("a11y nodes = %d", len(nodes))
	}
	// Root is synthetic group, parent -1, physical size 400x400.
	root := nodes[len(nodes)-1] // synthetic root appended before children walk? find parent -1
	var found *A11yNode
	var button *A11yNode
	for i := range nodes {
		if nodes[i].ParentID == -1 {
			found = &nodes[i]
		}
		if nodes[i].Role == "button" {
			button = &nodes[i]
		}
	}
	_ = root
	if found == nil || found.W != 400 {
		t.Fatalf("root node = %+v", found)
	}
	if button == nil {
		t.Fatal("no button node")
	}
	if button.Label != "Compose" {
		t.Fatalf("button label = %q (should inherit child text)", button.Label)
	}
	if !button.Tappable || button.W <= 0 {
		t.Fatalf("button not tappable/sized: %+v", button)
	}
	// Physical rect: button is below the "Inbox" text inside padding(10).
	if button.X < 20 { // 10 logical padding * scale 2
		t.Fatalf("button x=%d, expected >= 20 physical", button.X)
	}
	// Activation invokes OnTap.
	h.Core.A11yActivate(button.ID)
	if activated != 1 {
		t.Fatalf("activate did not fire OnTap: %d", activated)
	}
	// Hit test lands on the button.
	if id := h.Core.A11yHitTest(button.X+button.W/2, button.Y+button.H/2, 2); id != button.ID {
		t.Fatalf("hit test = %d, want button %d", id, button.ID)
	}
}
