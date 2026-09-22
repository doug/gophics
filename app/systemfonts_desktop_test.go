//go:build !js && !android && !ios

package app

import (
	"sync"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/text"
	"github.com/doug/gophics/widget"
)

// TestSystemFontsSharedMapIsSafeAcrossApps: several headless apps with
// SystemFonts set, shaping concurrently on their own goroutines, must not
// share the scanned font map. Under -race this is the test that caught the
// process-wide map: fontscan hands out cached *font.Face values and harfbuzz
// writes their cmap caches while shaping, so a lock around the lookup was
// not enough — the map has to be per Painter.
//
// A machine with no scannable fonts (a bare container) skips: that is a
// capability the test needs, not a defect in the code under test. The probe
// scans the default cache dir — the one Config.SystemFonts uses — once,
// serially, so the apps below only load the index.
func TestSystemFontsSharedMapIsSafeAcrossApps(t *testing.T) {
	f, err := text.Parse(goregular.TTF)
	if err != nil {
		t.Fatal(err)
	}
	if err := text.NewShaper(f).UseSystemFonts(""); err != nil {
		t.Skipf("capability: system font scan unavailable here (%v); concurrent system-font shaping is not exercised", err)
	}

	const apps = 4
	var wg sync.WaitGroup
	errs := make(chan error, apps)
	for i := 0; i < apps; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			h, err := NewHeadless(widget.Text{Value: "日本語 ✓ x", Color: paint.RGB(0, 0, 0)}, Config{
				Size:        geom.Size{W: 200, H: 60},
				Font:        goregular.TTF,
				SystemFonts: true,
			}, 1)
			if err != nil {
				errs <- err
				return
			}
			p := h.Owner().Painter
			// Runes Go Regular lacks go to the system map on every shape; the
			// same string through several shapers is the contended path.
			for _, s := range []string{"日本語", "한국어", "✓", "😀 abc", "Ωmega"} {
				p.ShapeIn("", s, 16)
				p.MeasureWidthIn("", s, 14)
			}
			h.Render()
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
}
