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
	}))

	// Tap the button the way a screen reader would find it, rather than
	// hardcoding a pixel that moves the moment the layout changes.
	a.TapLabel("Increment")

	if !a.HasLabel("4") {
		t.Errorf("tapping Increment did not advance the count. Labels: %v", a.Labels())
	}

	// The frame itself is checked against a committed golden, so a change in
	// how it looks fails here rather than being noticed later by a person.
	a.Golden("counter")
}
