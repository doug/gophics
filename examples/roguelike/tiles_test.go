package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// TestTileSheet renders the generated atlas, scaled up, for inspection.
// Run: ROGUE_TILES=<path> go test -run TestTileSheet ./examples/roguelike
func TestTileSheet(t *testing.T) {
	out := os.Getenv("ROGUE_TILES")
	if out == "" {
		t.Skip("set ROGUE_TILES=<path>")
	}
	atlas := buildAtlas()
	const scale = float32(6)
	size := geom.Size{W: float32(int(tileCount))*(tile*scale+14) + 26, H: tile*scale + 40}
	sheet := widget.Canvas{Draw: func(c paint.Canvas, _ geom.Size) {
		c.Clear(paint.RGB(0.10, 0.10, 0.13))
		for i := TileID(0); i < tileCount; i++ {
			x := 13 + float32(int(i))*(tile*scale+14)
			c.DrawSprite(atlas, paint.Sprite{
				Src: src(i), Dst: geom.RectXYWH(x, 20, tile*scale, tile*scale), Nearest: true})
		}
	}}
	h, err := app.NewHeadless(sheet, app.Config{Size: size, Font: goregular.TTF}, 2)
	if err != nil {
		t.Fatal(err)
	}
	f, _ := os.Create(out)
	defer f.Close()
	_ = png.Encode(f, h.Render())
}
