package main

import (
	"math/rand"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

func clueCount(g Grid) int {
	n := 0
	for _, v := range g {
		if v != 0 {
			n++
		}
	}
	return n
}

// TestGeneratedPuzzleIsValidAndUnique is the core generator guarantee: the
// solution is a valid complete grid, the puzzle is a strict subset of it, and
// the puzzle has exactly one solution.
func TestGeneratedPuzzleIsValidAndUnique(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for trial := range 5 {
		puzzle, solution := generate(rng, clueTarget)

		if !solution.solved() {
			t.Fatalf("trial %d: generated solution is not a valid complete grid", trial)
		}
		for i := range 81 {
			if puzzle[i] != 0 && puzzle[i] != solution[i] {
				t.Fatalf("trial %d: clue at %d (%d) disagrees with solution (%d)", trial, i, puzzle[i], solution[i])
			}
		}
		if n := clueCount(puzzle); n < 17 || n >= 81 {
			t.Fatalf("trial %d: implausible clue count %d", trial, n)
		}
		if got := countSolutions(puzzle, 2); got != 1 {
			t.Fatalf("trial %d: puzzle has %d solutions, want exactly 1", trial, got)
		}
	}
}

func TestCanPlaceConstraints(t *testing.T) {
	var g Grid
	g[idx(0, 0)] = 5           // row 0, col 0, box 0
	if canPlace(&g, 0, 4, 5) { // same row
		t.Error("allowed a repeat in the row")
	}
	if canPlace(&g, 4, 0, 5) { // same column
		t.Error("allowed a repeat in the column")
	}
	if canPlace(&g, 1, 1, 5) { // same box
		t.Error("allowed a repeat in the box")
	}
	if !canPlace(&g, 4, 4, 5) { // clear cell in a different row/col/box
		t.Error("rejected a legal placement")
	}
}

func TestConflictsFlagsDuplicates(t *testing.T) {
	var g Grid
	g[idx(0, 0)] = 3
	g[idx(0, 5)] = 3 // duplicate in row 0
	bad := g.conflicts()
	if !bad[idx(0, 0)] || !bad[idx(0, 5)] {
		t.Fatal("row duplicates not both flagged")
	}
	if bad[idx(4, 4)] {
		t.Fatal("flagged an empty cell")
	}
}

// TestInputWiring drives the real Core dispatch (app.Headless): tapping a cell
// selects it, typed digits reach OnText and fill it, a clue cell can't be
// overwritten, Notes mode records pencil marks, and New resets the board.
func TestInputWiring(t *testing.T) {
	var g *game
	stateHook = func(gg *game) { g = gg }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(Game{}, app.Config{Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render() // mount (Init hook), layout, autofocus, draw (sets board/button rects)
	if g == nil {
		t.Fatal("Init hook never fired — state not mounted")
	}

	center := func(i int) geom.Pt {
		r, c := i/9, i%9
		return geom.Pt{
			X: g.boardRect.Min.X + (float32(c)+0.5)*g.cell,
			Y: g.boardRect.Min.Y + (float32(r)+0.5)*g.cell,
		}
	}

	// Find an empty cell and a clue cell.
	empty, clue := -1, -1
	for i := range 81 {
		if g.given[i] {
			if clue < 0 {
				clue = i
			}
		} else if empty < 0 {
			empty = i
		}
	}
	if empty < 0 || clue < 0 {
		t.Fatal("puzzle lacks both an empty cell and a clue")
	}

	// Tap the empty cell → it becomes selected.
	h.Tap(center(empty))
	if g.sel != empty {
		t.Fatalf("tap didn't select the empty cell: sel=%d want %d", g.sel, empty)
	}
	// Type a digit → it fills (via OnText).
	h.Type("7")
	if g.board[empty] != 7 {
		t.Fatalf("typed digit didn't fill the cell: board=%d want 7", g.board[empty])
	}
	// Typing the same digit again clears it.
	h.Type("7")
	if g.board[empty] != 0 {
		t.Fatalf("retyping the digit didn't clear the cell: board=%d", g.board[empty])
	}

	// A clue cell can't be overwritten.
	h.Tap(center(clue))
	before := g.board[clue]
	h.Type("1")
	if g.board[clue] != before {
		t.Fatalf("a clue cell was overwritten: %d -> %d", before, g.board[clue])
	}

	// Notes mode records a pencil mark instead of filling.
	h.Tap(center(empty))
	nb := g.notesBtn
	h.Tap(geom.Pt{X: nb.Min.X + nb.Dx()/2, Y: nb.Min.Y + nb.Dy()/2})
	if !g.noteMode {
		t.Fatal("Notes button didn't toggle note mode")
	}
	h.Type("4")
	if g.board[empty] != 0 || g.notes[empty]&(1<<4) == 0 {
		t.Fatalf("note not recorded: board=%d notes=%b", g.board[empty], g.notes[empty])
	}

	// New resets the board to its (fresh) puzzle.
	g.board[empty] = 9 // dirty the board
	g.won = true
	newBtn := g.newBtn
	h.Tap(geom.Pt{X: newBtn.Min.X + newBtn.Dx()/2, Y: newBtn.Min.Y + newBtn.Dy()/2})
	if g.won {
		t.Fatal("New didn't reset the won flag")
	}
	if g.board != g.puzzle {
		t.Fatal("New didn't reset the board to the puzzle")
	}
}

// TestEraseClearsSelection covers the Backspace path through OnKey.
func TestEraseClearsSelection(t *testing.T) {
	var g *game
	stateHook = func(gg *game) { g = gg }
	defer func() { stateHook = nil }()

	h, err := app.NewHeadless(Game{}, app.Config{Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF}, 1)
	if err != nil {
		t.Fatal(err)
	}
	h.Render()

	empty := -1
	for i := range 81 {
		if !g.given[i] {
			empty = i
			break
		}
	}
	g.sel = empty
	g.board[empty] = 5
	h.Key(shell.KeyBackspace)
	if g.board[empty] != 0 {
		t.Fatalf("Backspace didn't clear the cell: board=%d", g.board[empty])
	}
}
