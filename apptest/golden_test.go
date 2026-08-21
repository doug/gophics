package apptest

import (
	"image"
	"image/color"
	"os"
	"path/filepath"
	"testing"
)

func solid(w, h int, c color.RGBA) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

var (
	black = color.RGBA{A: 255}
	white = color.RGBA{R: 255, G: 255, B: 255, A: 255}
)

func TestCompareIdenticalImages(t *testing.T) {
	a := solid(8, 8, black)
	b := solid(8, 8, black)

	res := compare(a, b)
	if res.differing != 0 || res.maxChannelDiff != 0 {
		t.Errorf("identical images compared unequal: %s", res)
	}
	if !res.ok(Exact) {
		t.Error("identical images failed Exact tolerance")
	}
}

func TestCompareCountsDifferingPixels(t *testing.T) {
	a := solid(10, 10, black)
	b := solid(10, 10, black)
	b.SetRGBA(0, 0, white)
	b.SetRGBA(1, 0, white)
	b.SetRGBA(2, 0, white)

	res := compare(a, b)
	if res.differing != 3 {
		t.Errorf("differing = %d, want 3", res.differing)
	}
	if res.total != 100 {
		t.Errorf("total = %d, want 100", res.total)
	}
	if got, want := res.diffPct(), 3.0; got != want {
		t.Errorf("diffPct() = %v, want %v", got, want)
	}
	if res.maxChannelDiff != 255 {
		t.Errorf("maxChannelDiff = %d, want 255", res.maxChannelDiff)
	}
}

// The two tolerance knobs must be independent: each has to be able to fail a
// comparison the other would pass, or one of them is decoration.
func TestToleranceKnobsAreIndependent(t *testing.T) {
	// Many pixels, each barely wrong: passes a channel bound, fails a count bound.
	wide := solid(10, 10, color.RGBA{R: 100, G: 100, B: 100, A: 255})
	wideOff := solid(10, 10, color.RGBA{R: 102, G: 100, B: 100, A: 255})
	res := compare(wide, wideOff)

	if !res.ok(Tolerance{MaxChannelDiff: 2, MaxDiffPct: 100}) {
		t.Errorf("small-but-everywhere change should pass a generous count bound: %s", res)
	}
	if res.ok(Tolerance{MaxChannelDiff: 2, MaxDiffPct: 50}) {
		t.Errorf("100%% of pixels changed should fail a 50%% count bound: %s", res)
	}

	// One pixel, badly wrong: passes a count bound, fails a channel bound.
	narrow := solid(10, 10, black)
	narrowOff := solid(10, 10, black)
	narrowOff.SetRGBA(5, 5, white)
	res2 := compare(narrow, narrowOff)

	if !res2.ok(Tolerance{MaxChannelDiff: 255, MaxDiffPct: 5}) {
		t.Errorf("one changed pixel should pass a generous channel bound: %s", res2)
	}
	if res2.ok(Tolerance{MaxChannelDiff: 2, MaxDiffPct: 5}) {
		t.Errorf("a 255-level change should fail a 2-level channel bound: %s", res2)
	}
}

// A size change is never within tolerance. Scaling an image to compare it would
// hide exactly the regression worth catching.
func TestSizeMismatchAlwaysFails(t *testing.T) {
	res := compare(solid(8, 8, black), solid(9, 8, black))

	if !res.sizeMismatch {
		t.Fatal("differing sizes not reported as a size mismatch")
	}
	if res.ok(Tolerance{MaxChannelDiff: 255, MaxDiffPct: 100}) {
		t.Error("size mismatch passed the most generous tolerance possible")
	}
	if got := res.String(); got == "" || !contains(got, "size changed") {
		t.Errorf("String() = %q, want it to mention the size change", got)
	}
}

// The diff map has to mark the pixels that changed, or it is a decoration that
// sends the reader looking in the wrong place.
func TestDiffMapMarksChangedPixels(t *testing.T) {
	a := solid(6, 6, black)
	b := solid(6, 6, black)
	b.SetRGBA(2, 3, white)

	res := compare(a, b)
	if res.diff == nil {
		t.Fatal("no diff map produced")
	}

	changed := res.diff.RGBAAt(2, 3)
	if changed.R <= changed.G || changed.R <= changed.B {
		t.Errorf("changed pixel is not red: %+v", changed)
	}

	unchanged := res.diff.RGBAAt(0, 0)
	if unchanged.R != unchanged.G || unchanged.G != unchanged.B {
		t.Errorf("unchanged pixel is not grey: %+v", unchanged)
	}
}

// A missing golden must surface as os.IsNotExist so Golden can turn it into a
// failure with instructions. Silently passing is the failure mode this whole
// package exists to avoid.
func TestMissingGoldenReportsNotExist(t *testing.T) {
	_, err := compareAgainst(filepath.Join(t.TempDir(), "absent.png"), solid(4, 4, black), Exact)

	if err == nil {
		t.Fatal("a missing golden returned no error — it would have passed silently")
	}
	if !os.IsNotExist(err) {
		t.Errorf("err = %v, want an os.IsNotExist error so Golden can explain how to create it", err)
	}
}

func TestRoundTripThroughPNG(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "img.png") // sub/ does not exist yet
	src := solid(5, 7, color.RGBA{R: 10, G: 200, B: 30, A: 255})

	if err := writePNG(path, src); err != nil {
		t.Fatalf("writePNG: %v", err)
	}

	res, err := compareAgainst(path, src, Exact)
	if err != nil {
		t.Fatalf("compareAgainst: %v", err)
	}
	if !res.ok(Exact) {
		t.Errorf("image did not survive a PNG round trip: %s", res)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
