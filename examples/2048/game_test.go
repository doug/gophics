package main

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func TestSlideMergeUp(t *testing.T) {
	// two 2s stacked in column 0 → merge to a 4 at the top, score 4.
	var g [boardN][boardN]int
	g[1][0] = 2
	g[2][0] = 2
	out, _, gained, moved := slide(g, 2) // up
	if !moved || gained != 4 || out[0][0] != 4 {
		t.Fatalf("up merge: moved=%v gained=%d top=%d", moved, gained, out[0][0])
	}
	// only one tile remains.
	n := 0
	for r := range boardN {
		for c := range boardN {
			if out[r][c] != 0 {
				n++
			}
		}
	}
	if n != 1 {
		t.Fatalf("expected 1 tile after merge, got %d", n)
	}
}

func TestSlideLeftCompacts(t *testing.T) {
	// 0 2 0 2 → 4 0 0 0 (merge), row-wise.
	var g [boardN][boardN]int
	g[0][1], g[0][3] = 2, 2
	out, _, gained, moved := slide(g, 0) // left
	if !moved || gained != 4 || out[0][0] != 4 || out[0][1] != 0 {
		t.Fatalf("left: moved=%v gained=%d row=%v", moved, gained, out[0])
	}
}

func TestSlideNoMove(t *testing.T) {
	// already packed left with no equal neighbours → no move.
	var g [boardN][boardN]int
	g[0][0], g[0][1] = 2, 4
	_, _, _, moved := slide(g, 0)
	if moved {
		t.Fatal("expected no move for an already-settled row")
	}
}

func TestSlideSingleMergePerMove(t *testing.T) {
	// 2 2 2 2 left → 4 4 0 0 (each pair merges once, not into an 8).
	var g [boardN][boardN]int
	g[0] = [boardN]int{2, 2, 2, 2}
	out, _, gained, _ := slide(g, 0)
	if out[0] != [boardN]int{4, 4, 0, 0} || gained != 8 {
		t.Fatalf("quad merge: row=%v gained=%d", out[0], gained)
	}
}

// TestInputWiring proves the input path end to end through the real Core
// dispatch (app.Headless): a key event reaches OnKey and moves the board, and a
// tap on New Game reaches OnPress+OnTap and resets it. This is the reliable
// interactivity check (browser automation can't synthesize the pointerdown/
// keydown the web canvas listens for).
func TestInputWiring(t *testing.T) {
	var g *game
	stateHook = func(gg *game) { g = gg }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(Game{}, app.Config{Size: geom.Size{W: 420, H: 560}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount (Init hook fires) + layout + autofocus + draw (sets newBtn)
	if g == nil {
		t.Fatal("Init hook never fired — state not mounted")
	}

	// Keyboard: a lone tile on the right must slide left on KeyLeft (via OnKey).
	g.grid = [boardN][boardN]int{}
	g.grid[0][3] = 2
	g.sliding, g.spawning, g.over = false, false, false
	h.Key(shell.KeyLeft)
	if g.grid[0][0] != 2 {
		t.Fatalf("KeyLeft didn't reach OnKey/move: grid=%v", g.grid)
	}

	// Pointer: tapping the New Game button (via OnPress+OnTap) resets the board.
	g.grid = [boardN][boardN]int{}
	g.grid[1][1] = 8
	g.over, g.sliding = false, false
	c := g.newBtn
	h.Tap(geom.Pt{X: c.Min.X + c.Dx()/2, Y: c.Min.Y + c.Dy()/2})
	if g.grid[1][1] == 8 {
		t.Fatalf("New Game tap didn't reach OnPress/OnTap/reset: grid=%v", g.grid)
	}
}
