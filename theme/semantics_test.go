package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type controlsApp struct{}

func (controlsApp) Build(widget.Ctx) widget.Widget {
	return widget.Column(
		theme.Switch{On: true, Label: "Wi-Fi", OnChange: func(bool) {}},
		theme.Checkbox{Checked: false, Label: "Remember me", OnChange: func(bool) {}},
		theme.Radio{Selected: true, Label: "Standard shipping", OnSelect: func() {}},
		theme.Slider{Value: 0.25, Label: "Volume", OnChange: func(float32) {}},
		theme.Progress{Value: 0.5, Label: "Uploading"},
	)
}

func find(nodes []layout.SemNode, role layout.Role) *layout.SemNode {
	flat := layout.FlattenSemantics(nodes)
	for i := range flat {
		if flat[i].Role == role {
			return &flat[i]
		}
	}
	return nil
}

// Every drawn control has to say what it is and what its value is: gophics
// paints pixels, so a screen reader has nothing else to go on.
func TestControlsPublishRoleAndState(t *testing.T) {
	h, err := app.NewHeadless(controlsApp{}, app.Config{
		Size: geom.Size{W: 320, H: 400}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	sem := h.Semantics()

	sw := find(sem, layout.RoleSwitch)
	if sw == nil {
		t.Fatal("switch published no switch role")
	}
	if sw.Label != "Wi-Fi" {
		t.Errorf("switch label = %q, want %q", sw.Label, "Wi-Fi")
	}
	if sw.Checked == nil || !*sw.Checked {
		t.Errorf("switch checked = %v, want true", sw.Checked)
	}

	cb := find(sem, layout.RoleCheckbox)
	if cb == nil {
		t.Fatal("checkbox published no checkbox role")
	}
	if cb.Checked == nil || *cb.Checked {
		t.Errorf("checkbox checked = %v, want false", cb.Checked)
	}
	if cb.Label != "Remember me" {
		t.Errorf("checkbox label = %q", cb.Label)
	}

	rd := find(sem, layout.RoleRadio)
	if rd == nil || !rd.Selected {
		t.Errorf("radio = %+v, want selected", rd)
	}

	sl := find(sem, layout.RoleSlider)
	if sl == nil {
		t.Fatal("slider published no slider role")
	}
	if sl.Value != "25%" {
		t.Errorf("slider value = %q, want %q", sl.Value, "25%")
	}
	if sl.Label != "Volume" {
		t.Errorf("slider label = %q", sl.Label)
	}

	pr := find(sem, layout.RoleProgress)
	if pr == nil {
		t.Fatal("progress published no progressbar role")
	}
	if pr.Label != "Uploading, 50%" {
		t.Errorf("progress label = %q, want %q", pr.Label, "Uploading, 50%")
	}
}

// A control declaring its own semantics must produce one node, not a control
// nested inside the button Interactive would otherwise infer.
func TestControlSemanticsAreNotNested(t *testing.T) {
	h, err := app.NewHeadless(controlsApp{}, app.Config{
		Size: geom.Size{W: 320, H: 400}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Role == layout.RoleButton {
			t.Errorf("control published a button node as well: %+v", n)
		}
	}
}

type spinnerApp struct{}

func (spinnerApp) Build(widget.Ctx) widget.Widget {
	return theme.Spinner{Label: "Loading messages"}
}

// An indeterminate indicator still has to name what it is waiting on.
func TestSpinnerPublishesLabel(t *testing.T) {
	h, err := app.NewHeadless(spinnerApp{}, app.Config{
		Size: geom.Size{W: 100, H: 100}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	n := find(h.Semantics(), layout.RoleProgress)
	if n == nil || n.Label != "Loading messages" {
		t.Errorf("spinner semantics = %+v", n)
	}
}

// An indeterminate Progress animates; a determinate one must not, or a static
// page never stops requesting frames.
func TestDeterminateProgressDoesNotAnimate(t *testing.T) {
	h, err := app.NewHeadless(determinateApp{}, app.Config{
		Size: geom.Size{W: 200, H: 60}, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if h.Step(1.0 / 60) {
		t.Error("determinate Progress kept the frame loop awake")
	}
}

type determinateApp struct{}

func (determinateApp) Build(widget.Ctx) widget.Widget {
	return theme.Progress{Value: 0.4, Label: "Copying"}
}
