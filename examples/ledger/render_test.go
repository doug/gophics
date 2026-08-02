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

// TestLedgerRenders confirms the dashboard mounts and paints without panicking,
// and (with LEDGER_SHOT set) writes a PNG for visual inspection.
func TestLedgerRenders(t *testing.T) {
	size := geom.Size{W: 900, H: 820}
	h, err := app.NewHeadless(Ledger{}, app.Config{
		Size: size, Background: colBG,
		Font: goregular.TTF, FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	// Settle any mount animations.
	for i := 0; i < 60; i++ {
		h.Step(0.05)
	}
	img := h.Render()
	if img.Bounds().Empty() {
		t.Fatal("empty render")
	}
	if out := os.Getenv("LEDGER_SHOT"); out != "" {
		f, _ := os.Create(out)
		defer f.Close()
		_ = png.Encode(f, img)
	}
}
