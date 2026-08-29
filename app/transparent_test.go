package app_test

import (
	"image"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// An overlay needs exactly what the runtime otherwise forbids.
//
// The background goes down as a blended FillRect over a surface retained across
// frames, so a translucent one composites over the previous frame and ghosts —
// which is why the runtime forced bg.A = 1 unconditionally. Config.Transparent
// opts into translucency and pays for it by replaying the whole scene each
// frame instead of the damaged region.
func TestTransparentBackgroundStaysTranslucent(t *testing.T) {
	cfg := func(transparent bool) app.Config {
		return app.Config{
			Size:        geom.Size{W: 60, H: 40},
			Background:  paint.Color{R: 0, G: 0, B: 0, A: 0},
			Font:        goregular.TTF,
			Transparent: transparent,
		}
	}

	// Opaque by default: a zero-alpha background is forced solid, because the
	// retained surface has no way to represent "see through to what was here".
	h, err := app.NewHeadless(widget.Sized{}, cfg(false), 1)
	if err != nil {
		t.Fatal(err)
	}
	if a := alphaAt(h.Render(), 3, 3); a != 0xffff {
		t.Errorf("default background alpha = %#04x, want opaque", a)
	}

	// Transparent: the alpha survives, which is what lets a host composite.
	h2, err := app.NewHeadless(widget.Sized{}, cfg(true), 1)
	if err != nil {
		t.Fatal(err)
	}
	if a := alphaAt(h2.Render(), 3, 3); a != 0 {
		t.Errorf("transparent background alpha = %#04x, want 0 — a host cannot "+
			"composite a UI that painted itself opaque", a)
	}
}

func alphaAt(img image.Image, x, y int) uint32 {
	_, _, _, a := img.At(x, y).RGBA()
	return a
}
