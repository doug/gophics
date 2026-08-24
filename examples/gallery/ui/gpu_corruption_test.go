//go:build gophics_gpu

package ui_test

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/examples/gallery/ui"
	"github.com/doug/gophics/geom"
	gtext "github.com/doug/gophics/internal/gfx/gg/text"
)

// TestGalleryTextSurvivesNavigation drives the real catalog through the real
// GPU path and checks that text which has not changed still renders the same.
//
// This exists because four fixes were shipped against a fault that only a
// phone could show, and none of them was the fault. A reproduction that runs
// here, in seconds, writing PNGs, is worth more than any of them: it proves
// the platform produces the corruption and it is the only way to know when it
// stops.
//
// The shape mirrors what a person does to trigger it: open a section, come
// back, open another, come back — each visit shaping text the atlas has not
// seen, then returning to a screen whose glyphs were rasterized long before.
//
// Set GALLERY_OUT to a directory to keep every frame as a PNG.
func TestGalleryTextSurvivesNavigation(t *testing.T) {
	out := os.Getenv("GALLERY_OUT")
	// @3x, as a phone renders. Glyph cache keys carry the device pixel size,
	// so scale decides how many distinct masks the atlas holds — at 1x this
	// catalog saturates at a couple of hundred and never pressures anything.
	a := apptest.New(t, ui.Gallery{}, apptest.Scale(3), apptest.WithConfig(app.Config{
		Title: "Gophics Catalog", Size: geom.Size{W: 420, H: 760},
		Font: goregular.TTF,
	}))
	if a.RenderGPU() == nil {
		t.Skip("no GPU adapter available")
	}

	save := func(name string, img image.Image) {
		if out == "" {
			return
		}
		f, err := os.Create(filepath.Join(out, name+".png"))
		if err != nil {
			t.Fatal(err)
		}
		defer f.Close()
		if err := png.Encode(f, img); err != nil {
			t.Fatal(err)
		}
	}

	sections := []string{}
	for _, name := range []string{
		"Buttons & tappables", "Form controls", "Selection", "Pickers",
		"Text input", "Typography", "Cards & surfaces", "Charts",
	} {
		if a.HasText(name) {
			sections = append(sections, name)
		}
	}
	if len(sections) < 2 {
		t.Skipf("only %d sections found; nothing to churn", len(sections))
	}

	// open visits a section and returns its rendered page. The page's own
	// content does not change between visits, so two renders of it must match
	// — unlike the catalog list, which keeps a press highlight and would
	// report a difference that means nothing.
	// A visit must actually land on the section's page. Without that check the
	// test compares the catalog list to itself, and the press highlight left on
	// every row reads as a forty-percent difference that means nothing — which
	// is exactly how this first "reproduced" the bug.
	// settle runs the frame clock forward. A Navigator push is animated, so a
	// tap followed by a single render still shows the catalog — which is why
	// this harness appeared unable to navigate at all.
	settle := func() {
		for i := 0; i < 40; i++ {
			a.Step(1.0 / 60)
		}
		a.RenderGPU()
	}

	open := func(t *testing.T, name string) (*image.RGBA, bool) {
		t.Helper()
		a.TapText(name)
		settle()
		landed := !a.HasText("A tour of the gophics component set")
		// Scroll the page as well: it is the motion that brings glyphs the
		// atlas has not seen, and the fault a device shows arrives while
		// scrolling rather than on arrival.
		a.Scroll(geom.Pt{Y: -200})
		settle()
		img := toRGBA(a.RenderGPU())
		a.Scroll(geom.Pt{Y: 200})
		settle()
		if a.HasText("Back") {
			a.TapText("Back")
			settle()
		}
		return img, landed
	}

	subject := sections[0]
	before, landed := open(t, subject)
	if !landed {
		t.Skip("tapping a catalog row did not open its page; this harness cannot " +
			"churn the atlas without navigating, and comparing the list to itself " +
			"only measures its press highlight")
	}
	save("00_subject_before", before)

	// Churn: every other section, several times over. Each page shapes text
	// the atlas has not seen, which is what moving around an app does.
	const rounds = 40
	// Cycling the theme as well. Each theme redraws every label in different
	// colours and the glass ones add their own chrome, so it shapes text the
	// atlas has not held — which is the pressure a device sees and a fixed
	// screen does not.
	themes := []string{"Light", "Dark", "Glass", "Glass Dark"}
	for r := 0; r < rounds; r++ {
		for _, name := range sections[1:] {
			open(t, name) //nolint:errcheck // churn only
		}
		if th := themes[r%len(themes)]; a.HasText(th) {
			a.TapText(th)
			settle()
		}
	}
	// Back to the theme the subject was captured under, or every pixel differs
	// for a reason that is not corruption.
	if a.HasText("Light") {
		a.TapText("Light")
		settle()
	}

	after, _ := open(t, subject)
	save("99_subject_after", after)

	ref, ev, cmp := gtext.AtlasStats()
	w, late, up := gtext.AtlasWriteStats()
	t.Logf("%d sections x %d rounds — atlas ref=%d ev=%d cmp=%d w=%d late=%d up=%d nilview=%d",
		len(sections), rounds, ref, ev, cmp, w, late, up, gtext.AtlasNilViews())

	diff := 0
	for i := range before.Pix {
		if before.Pix[i] != after.Pix[i] {
			diff++
		}
	}
	if diff > 0 {
		pct := float64(diff) * 100 / float64(len(before.Pix))
		save("98_subject_diff_marker", after)
		t.Errorf("%q renders differently after %d page visits: %d of %d bytes differ "+
			"(%.2f%%) — its content did not change, so its glyphs are no longer "+
			"being drawn from where they were put",
			subject, rounds*len(sections[1:]), diff, len(before.Pix), pct)
	}
}

func toRGBA(src image.Image) *image.RGBA {
	if r, ok := src.(*image.RGBA); ok {
		return r
	}
	b := src.Bounds()
	dst := image.NewRGBA(b)
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dst.Set(x, y, src.At(x, y))
		}
	}
	return dst
}
