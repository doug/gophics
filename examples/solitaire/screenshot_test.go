package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/examples/solitaire/klondike"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
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
	for round := 0; round < 60; round++ {
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
