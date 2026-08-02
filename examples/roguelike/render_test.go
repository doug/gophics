package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
)

func mount(t *testing.T, seed int64) (*app.Headless, *gameState) {
	t.Helper()
	var st *gameState
	stateHook = func(s *gameState) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(Roguelike{Seed: seed}, app.Config{
		Size: geom.Size{W: 900, H: 680}, Background: colBG,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()
	if st == nil {
		t.Fatal("state not mounted")
	}
	return h, st
}

// TestRoguelikeRenders mounts the game and paints a frame; with ROGUE_SHOT set,
// it plays a few steps and writes a PNG.
func TestRoguelikeRenders(t *testing.T) {
	h, st := mount(t, 7)
	if len(st.g.d.rooms) == 0 {
		t.Fatal("dungeon has no rooms")
	}
	if img := h.Render(); img.Bounds().Empty() {
		t.Fatal("empty render")
	}
	if out := os.Getenv("ROGUE_SHOT"); out != "" {
		for _, d := range [][2]int{{1, 0}, {1, 0}, {0, 1}, {0, 1}, {1, 0}, {0, 1}, {1, 0}} {
			st.g.Move(d[0], d[1])
		}
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, h.Render())
	}
}

// TestWin verifies the amulet on the deepest level wins the game.
func TestWin(t *testing.T) {
	g := newGame(1)
	g.depth = maxDepth - 1
	g.descend() // builds the final level, which carries the amulet
	var am *Item
	for _, it := range g.items {
		if it.Amulet {
			am = it
		}
	}
	if am == nil {
		t.Fatalf("no amulet on level %d", g.depth)
	}
	g.player.X, g.player.Y = am.X, am.Y
	g.pickup()
	if !g.won {
		t.Fatal("claiming the amulet should win the game")
	}
}

// TestCombatAndFOV exercises the pure game: movement reveals cells, and a d20
// attack eventually kills an adjacent rat.
func TestCombatAndFOV(t *testing.T) {
	g := newGame(3)
	if !g.visibleAt(g.player.X, g.player.Y) {
		t.Fatal("player's own cell should be visible")
	}
	seen := 0
	for _, v := range g.seen {
		if v {
			seen++
		}
	}
	if seen == 0 {
		t.Fatal("FOV revealed nothing")
	}

	// Place a rat next to the player and attack until it dies (bounded).
	rat := &Entity{X: g.player.X + 1, Y: g.player.Y, Tile: TRat, Name: "rat",
		HP: 3, MaxHP: 3, Atk: 0, AC: 1, Damage: 3, Alive: true} // AC 1 → always hit
	g.monsters = append(g.monsters, rat)
	for i := 0; i < 50 && rat.Alive; i++ {
		g.Move(1, 0) // bump-attack (rat blocks the tile)
	}
	if rat.Alive {
		t.Fatal("rat should have died from repeated hits")
	}
}
