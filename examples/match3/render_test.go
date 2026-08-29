package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
)

// TestRenders mounts the game, advances a few frames, and (with MATCH3_SHOT
// set) writes a PNG — a smoke test that the board builds and draws.
func TestRenders(t *testing.T) {
	h, err := app.NewHeadless(Match3{Seed: 7}, app.Config{
		Size: geom.Size{W: 480, H: 760}, Background: colBG,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	for range 20 {
		h.Step(0.016)
	}
	if h.Render().Bounds().Empty() {
		t.Fatal("empty render")
	}
	if out := os.Getenv("MATCH3_SHOT"); out != "" {
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, h.Render())
	}
}
