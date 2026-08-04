package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

type semApp struct{}

func (semApp) Build(widget.Ctx) widget.Widget {
	button := func(label string) widget.Widget {
		return widget.Interactive{
			Handler: widget.Handler{OnTap: func() {}},
			Child: widget.Decorated{Color: paint.RGB(0.2, 0.2, 0.2), Radius: 4,
				Child: widget.Padding{All: 6, Child: widget.Text{S: label}}},
		}
	}
	return widget.Column(
		widget.Text{S: "title"},
		button("Save"),
		button("Cancel"),
		widget.Semantics{Hidden: true, Child: widget.Text{S: "decorative"}},
		widget.Semantics{Label: "custom group", Child: widget.Sized{W: 10, H: 10}},
	)
}

func TestSemanticsRolesAndLabels(t *testing.T) {
	h, err := NewHeadless(semApp{}, Config{Size: geom.Size{W: 200, H: 200}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	flat := layout.FlattenSemantics(h.Core.Semantics())

	var labels []string
	byRole := map[layout.Role]int{}
	for _, n := range flat {
		byRole[n.Role]++
		if n.Role == layout.RoleButton {
			labels = append(labels, n.Label)
			if n.Rect.IsEmpty() {
				t.Fatal("button rect empty")
			}
		}
		if n.Label == "decorative" {
			t.Fatal("hidden subtree leaked into semantics")
		}
	}
	if byRole[layout.RoleButton] != 2 {
		t.Fatalf("buttons = %d, want 2", byRole[layout.RoleButton])
	}
	if len(labels) != 2 || labels[0] != "Save" || labels[1] != "Cancel" {
		t.Fatalf("button labels = %v (inherited from Text children)", labels)
	}
	if byRole[layout.RoleGroup] != 1 {
		t.Fatalf("groups = %d, want 1 custom group", byRole[layout.RoleGroup])
	}
}

func TestFieldSemantics(t *testing.T) {
	h, _ := fieldHarness(t)
	h.Type("read me")
	h.Render()

	flat := layout.FlattenSemantics(h.Core.Semantics())
	var field *layout.SemNode
	for i := range flat {
		if flat[i].Role == layout.RoleTextField {
			field = &flat[i]
		}
	}
	if field == nil {
		t.Fatal("no textfield node in semantics tree")
	}
	if field.Value != "read me" {
		t.Fatalf("field value = %q", field.Value)
	}
	if !field.Focused {
		t.Fatal("focused field must report Focused")
	}
	if field.Rect.IsEmpty() {
		t.Fatal("semantic node must carry its rect")
	}
}
