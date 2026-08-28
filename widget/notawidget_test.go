package widget_test

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A value that is not a widget shows up as a broken subtree, not as a hang.
//
// Widget is any, so Padding{Child: "hello"} type-checks and only fails when the
// tree is mounted. That used to panic — and the panic landed in the worst
// possible place: safeBuild has already returned by then, so it escaped to the
// frame-level recover, which drops the frame and rate-limits its log to once
// every five seconds. Every following frame did the same. Nothing crashed and
// nothing appeared on screen to say why; the UI just stopped changing, which
// reads as a hang and sends you looking anywhere but at the field you mistyped.
//
// The policy safeBuild already states is the right one: one failing subtree
// must not take down the app.
func TestANonWidgetRendersAsAnErrorNotAHang(t *testing.T) {
	col := widget.Column(
		widget.Semantics{Label: "SIBLING", Child: widget.Sized{W: 50, H: 20}},
		widget.Padding{All: 4, Child: "hello"}, // not a widget
	)
	col.CrossAlign = layout.CrossStart

	h, err := app.NewHeadless(col, app.Config{
		Size: geom.Size{W: 200, H: 200}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// The failure is visible and in place.
	if !anyLabelContains(h, "build failed") {
		t.Errorf("the broken subtree renders nothing saying so; labels were %v", labelsOf(h))
	}

	// And it is localized: the sibling still rendered.
	if !anyLabelContains(h, "SIBLING") {
		t.Error("a sibling of the broken subtree stopped rendering; the failure was not contained")
	}

	// Frames keep coming — the old behaviour dropped every one of them.
	for range 3 {
		if h.Render() == nil {
			t.Fatal("a later frame was dropped; the app is hung, which is the bug")
		}
	}
}

func labelsOf(h *app.Headless) []string {
	var out []string
	for _, n := range h.Semantics() {
		if n.Label != "" {
			out = append(out, n.Label)
		}
	}
	return out
}

func anyLabelContains(h *app.Headless, sub string) bool {
	for _, n := range h.Semantics() {
		if strings.Contains(n.Label, sub) {
			return true
		}
	}
	return false
}

// And it says what it substituted, rather than swallowing it. Checked on a
// bare Owner because Headless mounts during construction, before a hook can be
// installed on it — the first version of this test set OnBuildPanic afterwards
// and saw nothing, which is the hook working correctly and the test asking too
// late.
func TestANonWidgetIsReported(t *testing.T) {
	var reported []string
	o := &widget.Owner{OnBuildPanic: func(r any) { reported = append(reported, r.(string)) }}
	o.SetRoot(widget.Padding{All: 4, Child: "hello"})
	o.FlushBuilds()

	if len(reported) == 0 {
		t.Fatal("a non-widget was substituted silently; nothing reported it")
	}
	if !strings.Contains(reported[0], "string") {
		t.Errorf("report %q does not name the offending type", reported[0])
	}
}
