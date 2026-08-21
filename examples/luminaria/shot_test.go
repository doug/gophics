package main

import (
	"image/png"
	"os"
	"testing"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/sound"
	"github.com/doug/gophics/theme"
)

func TestShot(t *testing.T) {
	out := os.Getenv("SHOT")
	if out == "" {
		t.Skip()
	}
	var st *lum
	stateHook = func(s *lum) { st = s }
	defer func() { stateHook = nil }()
	h, err := app.NewHeadless(App{Mixer: sound.NewMixer()}, app.Config{
		Size: geom.Size{W: 1040, H: 720}, Font: goregular.TTF,
		FontFamilies: map[string][]byte{theme.FontBold: gobold.TTF},
	}, 2)
	if err != nil {
		t.Fatal(err)
	}
	h.Resize(geom.Size{W: 1040, H: 720})
	h.Render()
	_ = st
	for i := 0; i < 200; i++ {
		h.Step(1.0 / 60)
	}
	f, _ := os.Create(out)
	defer f.Close()
	png.Encode(f, h.Render())
}
