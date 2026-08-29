package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/examples/solitaire/klondike"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// TestScreenshot renders a played board to a PNG for visual inspection.
// Run: SOLITAIRE_SHOT=<path> go test -run TestScreenshot ./examples/solitaire
func TestScreenshot(t *testing.T) {
	out := os.Getenv("SOLITAIRE_SHOT")
	if out == "" {
		t.Skip("set SOLITAIRE_SHOT=<path>")
	}
	makeStore = func() store { return &memStore{} }
	size := geom.Size{W: 1000, H: 760}
	var st *gameState
	stateHook = func(s *gameState) { st = s }
	h, _ := app.NewHeadless(Solitaire{Seed: 7}, app.Config{
		Size: size, Background: colFelt,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	stateHook = nil
	h.Render()
	for i := 0; i < 60 && st.dealing; i++ {
		h.Step(0.05)
		h.Render()
	}
	for round := range 60 {
		if acts := st.g.LegalActions(); len(acts) > 0 {
			a := acts[round%len(acts)]
			st.g.Move(a.From, a.FromIdx, a.To)
		} else {
			st.g.Draw()
		}
	}
	_ = klondike.Club
	img := h.Render()
	f, _ := os.Create(out)
	defer f.Close()
	_ = png.Encode(f, img)
}

// TestCascadeShot forces a won game and captures the win cascade at a few
// moments. Run: SOLITAIRE_CASCADE=<path-prefix> go test -run TestCascadeShot ./examples/solitaire
func TestCascadeShot(t *testing.T) {
	prefix := os.Getenv("SOLITAIRE_CASCADE")
	if prefix == "" {
		t.Skip("set SOLITAIRE_CASCADE=<path-prefix>")
	}
	makeStore = func() store { return &memStore{} }
	size := geom.Size{W: 1000, H: 760}
	var st *gameState
	stateHook = func(s *gameState) { st = s }
	h, err := app.NewHeadless(Solitaire{Seed: 3}, app.Config{
		Size: size, Background: colFelt,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	stateHook = nil
	if err != nil {
		t.Fatal(err)
	}
	// Force a completed game: all four foundations full, A→K.
	var snap klondike.Snapshot
	snap.DrawN = 1
	for suit := range 4 {
		for rank := 1; rank <= 13; rank++ {
			snap.Foundations[suit] = append(snap.Foundations[suit],
				klondike.Card{Suit: klondike.Suit(suit), Rank: uint8(rank), Up: true})
		}
	}
	st.g = klondike.Restore(snap)
	st.dealing = false
	h.Render() // lay out the full board + record size
	st.startCascade()

	shoot := map[int]string{50: "a", 120: "b", 220: "c"}
	for i := 1; i <= 220; i++ {
		h.Step(0.016)
		if tag, ok := shoot[i]; ok {
			f, _ := os.Create(prefix + tag + ".png")
			_ = png.Encode(f, h.Render())
			f.Close()
		}
	}
}

// TestCardSheet renders every rank in a red and a black suit, face-up, so the
// pip layouts and glyph centering can be inspected.
// Run: SOLITAIRE_SHEET=<path> go test -run TestCardSheet ./examples/solitaire
func TestCardSheet(t *testing.T) {
	out := os.Getenv("SOLITAIRE_SHEET")
	if out == "" {
		t.Skip("set SOLITAIRE_SHEET=<path>")
	}
	const cols, rows = 7, 4
	cw, ch := float32(150), float32(210)
	pad := float32(16)
	size := geom.Size{W: cols*cw + pad*float32(cols+1), H: rows*ch + pad*float32(rows+1)}
	suits := [2]klondike.Suit{klondike.Heart, klondike.Spade}

	sheet := widget.Canvas{Draw: func(c paint.Canvas, _ geom.Size) {
		c.Clear(colFeltHi)
		for rank := 1; rank <= 13; rank++ {
			for s, suit := range suits {
				i := (rank - 1)
				col := i % cols
				row := (i/cols)*2 + s
				x := pad + float32(col)*(cw+pad)
				y := pad + float32(row)*(ch+pad)
				drawCard(c, geom.RectXYWH(x, y, cw, ch),
					klondike.Card{Suit: suit, Rank: uint8(rank), Up: true})
			}
		}
	}}
	h, err := app.NewHeadless(sheet, app.Config{
		Size: size, Background: colFelt,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	img := h.Render()
	f, _ := os.Create(out)
	defer f.Close()
	_ = png.Encode(f, img)
}
