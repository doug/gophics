package main

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
)

// TestCounter drives the whole app with no window, no GPU and no display —
// the same renderer that draws it on screen, running under `go test`.
func TestCounter(t *testing.T) {
	a := apptest.New(t, Counter{Start: 3}, apptest.WithConfig(app.Config{
		Size: geom.Size{W: 320, H: 220}, Font: goregular.TTF,
	}), apptest.Tol(apptest.AntiAliased))

	// Tap the button the way a screen reader would find it, rather than
	// hardcoding a pixel that moves the moment the layout changes.
	a.TapLabel("Increment")

	if !a.HasLabel("4") {
		t.Errorf("tapping Increment did not advance the count. Labels: %v", a.Labels())
	}

	// The frame itself is checked against a committed golden, so a change in
	// how it looks fails here rather than being noticed later by a person.
	//
	// AntiAliased rather than Exact, because the golden is committed from one
	// machine and checked on another: CI runs Linux and this file's reference
	// image was rendered on macOS. Exact failed there on 4 of 70,400 pixels
	// differing by 1/255 — sub-LSB float rounding in the rasteriser, not a
	// change anyone made. Tolerance's own documentation draws this line:
	// "use it when comparing across machines; prefer Exact within one."
	//
	// The bound is still tight enough to do its job. 2/255 on at most 0.5% of
	// pixels catches a colour shift or a moved element; it does not catch a
	// last-bit difference in how two libms round the same blend.
	a.Golden("counter")
}
