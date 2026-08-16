package main

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// TestCounter drives the whole app with no window, no GPU and no display —
// the same renderer that draws it on screen, running under `go test`.
func TestCounter(t *testing.T) {
	h, err := app.NewHeadless(Counter{Start: 3}, app.Config{
		Size: geom.Size{W: 320, H: 220}, Font: goregular.TTF,
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	// Find the button the way a screen reader would, rather than hardcoding a
	// pixel that moves the moment the layout changes.
	var btn geom.Rect
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Role == layout.RoleButton {
			btn = n.Rect
		}
	}
	h.Tap(geom.Pt{X: btn.Min.X + btn.Dx()/2, Y: btn.Min.Y + btn.Dy()/2})

	if !shows(h, "4") {
		t.Error("tapping Increment did not advance the count")
	}
	if img := h.Render(); img.Bounds().Empty() {
		t.Fatal("no frame rendered") // img is an image.Image — diff it against a golden
	}
}

// shows reports whether any semantic node carries this text.
func shows(h *app.Headless, text string) bool {
	for _, n := range layout.FlattenSemantics(h.Semantics()) {
		if n.Label == text {
			return true
		}
	}
	return false
}
