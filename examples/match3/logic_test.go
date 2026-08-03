package main

import (
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

// TestMatches checks run detection in both axes on a base pattern that has no
// accidental runs (each cell = (r*cols+c) mod numTypes: horizontal neighbors
// differ by 1, vertical by cols mod numTypes = 2).
func TestMatches(t *testing.T) {
	g := &game{}
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			g.grid[r][c] = int8((r*cols + c) % numTypes)
		}
	}
	if g.hasMatch() {
		t.Fatal("base pattern should have no matches")
	}

	g.grid[2][5] = 4
	g.grid[3][5] = 4
	g.grid[4][5] = 4 // vertical triple in column 5
	m := g.matches()
	if !(m[2][5] && m[3][5] && m[4][5]) {
		t.Error("vertical triple not detected")
	}
	if m[0][0] {
		t.Error("unrelated cell flagged as matched")
	}
}

// TestPlay drives real swipes through the widget/headless harness and steps the
// full animation pipeline (swap → clear → fall → cascade), asserting it stays
// alive and rendering — a smoke test over the input + phase machine + paint path.
func TestPlay(t *testing.T) {
	h, err := app.NewHeadless(Match3{Seed: 3}, app.Config{
		Size: geom.Size{W: 480, H: 760}, Background: colBG,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Step(0.016)

	// Board layout mirrors game.layout for Size 480x760: pad 16, top 96 → cell 56.
	const pad, top, cell = 16.0, 96.0, 56.0
	center := func(r, c int) geom.Pt {
		return geom.Pt{X: pad + float32(c)*cell + cell/2, Y: top + float32(r)*cell + cell/2}
	}
	// Try a handful of horizontal swaps; step enough frames to resolve each
	// (swap 140ms + clear 200 + fall 260 ≈ 40 frames), then confirm it renders.
	for _, sw := range [][2]int{{4, 3}, {2, 1}, {5, 4}, {1, 2}} {
		h.Drag(center(sw[0], sw[1]), center(sw[0], sw[1]+1))
		for i := 0; i < 60; i++ {
			h.Step(0.016)
		}
	}
	if h.Render().Bounds().Empty() {
		t.Fatal("empty render after play")
	}
}
