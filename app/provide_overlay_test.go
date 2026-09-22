package app

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// provideAPI stands in for an app-lifetime handle passed through
// Config.Provide — the kind of value a dialog's confirm button needs.
type provideAPI struct{ name string }

// apiLabel reads the handle with MustOf from wherever it is mounted.
type apiLabel struct{}

func (apiLabel) Build(ctx widget.Ctx) widget.Widget {
	return widget.Text{Value: "api:" + ctx.MustOf[*provideAPI]().name}
}

func tapSemLabel(t *testing.T, h *Headless, label string) {
	t.Helper()
	for _, n := range layout.FlattenSemantics(h.core.Semantics()) {
		if n.Label == label {
			h.Tap(geom.Pt{X: (n.Rect.Min.X + n.Rect.Max.X) / 2, Y: (n.Rect.Min.Y + n.Rect.Max.Y) / 2})
			return
		}
	}
	t.Fatalf("no semantics node labelled %q", label)
}

// Config.Provide values are visible inside overlay entries, including an
// unscoped Show: they wrap the OverlayHost, not just the app. Before that a
// dialog's MustOf for an app-lifetime handle panicked.
func TestConfigProvideVisibleInOverlay(t *testing.T) {
	for _, scoped := range []bool{false, true} {
		root := widget.Provide[theme.Theme]{Value: theme.Dark(), Child: widget.Center(opener{
			onOpen: func(ctx widget.Ctx) {
				ov := ctx.MustOf[widget.Overlay]()
				if scoped {
					ov.ShowFrom(ctx, apiLabel{})
				} else {
					ov.Show(apiLabel{})
				}
			},
		})}
		h, err := NewHeadless(root, Config{
			Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF,
			Provide: []any{&provideAPI{name: "online"}},
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		h.Render()
		tapSemLabel(t, h, "Open")
		h.Render()
		if !hasSemLabel(h, "api:online") {
			t.Errorf("scoped=%v: overlay entry could not read the Config.Provide value", scoped)
		}
	}
}
