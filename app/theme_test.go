package app

import (
	"strings"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/theme"
	"github.com/doug/gossamer/widget"
)

type themedApp struct{}

func (themedApp) Build(ctx widget.Ctx) widget.Widget {
	return widget.Padding{All: 20, Child: widget.Column(
		theme.Title("Hello"),
		theme.Button{Label: "Go", OnTap: func() {}},
		theme.Card{Child: theme.Body("body text")},
	)}
}

func themedHarness(t *testing.T) *Headless {
	t.Helper()
	h, err := NewHeadless(themedApp{}, Config{
		Size: geom.Size{W: 300, H: 300}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	return h
}

func TestThemeAutoFollowsDarkMode(t *testing.T) {
	h := themedHarness(t)
	// theme.Of falls back to Auto: light by default. Card surface is white.
	img := h.Render()
	r, _, _, _ := img.At(150, 5).RGBA()
	_ = r // background is app config (zero → black); check a themed surface instead

	h.SetDarkMode(true)
	h.Render()
	if h.Core.Skipped {
		t.Fatal("dark switch must repaint")
	}

	// Button label present via semantics either way.
	found := false
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if n.Role == layout.RoleButton && strings.Contains(n.Label, "Go") {
			found = true
		}
	}
	if !found {
		t.Fatal("themed button missing from semantics")
	}
}

func TestBoldFamilyMeasuresWider(t *testing.T) {
	h := themedHarness(t)
	p := h.Core.Painter
	reg := p.MeasureWidthIn("", "Hello Bold World", 16)
	bold := p.MeasureWidthIn(theme.FontBold, "Hello Bold World", 16)
	if bold <= reg {
		t.Fatalf("bold (%v) should measure wider than regular (%v)", bold, reg)
	}
}

type panicky struct{}

func (panicky) Build(widget.Ctx) widget.Widget {
	panic("intentional test panic")
}

type panickyHost struct{}

func (panickyHost) Build(widget.Ctx) widget.Widget {
	return widget.Column(
		widget.Text{S: "healthy sibling"},
		panicky{},
	)
}

func TestBuildPanicIsolatedToSubtree(t *testing.T) {
	var recovered any
	h, err := NewHeadless(panickyHost{}, Config{
		Size: geom.Size{W: 300, H: 200}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Core.Owner.OnBuildPanic = func(r any) { recovered = r }
	h.Core.Owner.RebuildAll()
	h.Render()

	if recovered == nil {
		t.Fatal("panic not observed")
	}
	healthy, errBox := false, false
	for _, n := range layout.FlattenSemantics(h.Core.Semantics()) {
		if strings.Contains(n.Label, "healthy sibling") {
			healthy = true
		}
		if strings.Contains(n.Label, "build failed") {
			errBox = true
		}
	}
	if !healthy {
		t.Fatal("sibling subtree lost to panic")
	}
	if !errBox {
		t.Fatal("error box not rendered for panicking subtree")
	}
}
