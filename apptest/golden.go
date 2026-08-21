package apptest

import (
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
)

// UpdateEnv is the environment variable that makes Golden write files instead
// of comparing them:
//
//	GOPHICS_UPDATE_GOLDEN=1 go test ./...
//
// It is an environment variable rather than a flag because this is a library:
// registering a global -update flag would panic if the importing package
// defined one too.
const UpdateEnv = "GOPHICS_UPDATE_GOLDEN"

// Tolerance is how much difference Golden accepts before failing.
//
// Two knobs, because they catch different things. MaxChannelDiff bounds how
// wrong any single pixel may be, which catches a colour shift that touches
// everything slightly. MaxDiffPct bounds how many pixels may differ at all,
// which catches a small element moving or disappearing. A renderer change
// usually trips one and not the other.
type Tolerance struct {
	// MaxChannelDiff is the largest per-channel difference allowed in any
	// pixel. 0 requires exactness.
	MaxChannelDiff uint8
	// MaxDiffPct is the share of pixels allowed to differ, 0-100.
	MaxDiffPct float64
}

// Exact requires an identical image. The default.
var Exact = Tolerance{}

// AntiAliased tolerates the sub-pixel differences that appear when the same
// scene is rasterised by different backends or on different GPUs. Use it when
// comparing across machines; prefer Exact within one.
var AntiAliased = Tolerance{MaxChannelDiff: 2, MaxDiffPct: 0.5}

// Golden renders the current frame and compares it with dir/name.png.
//
// With GOPHICS_UPDATE_GOLDEN=1 it writes the file instead and reports what it
// wrote. Review those diffs like any other change: an update flow that is
// invisible is how a rendering regression gets committed as the new reference.
//
// A missing golden file is a failure, not a skip. Skipping is how the shader
// fixtures in this repo sat dead for months while their package reported ok —
// a test that silently does nothing is worse than no test, because it also
// reports success.
func (a *App) Golden(name string) {
	a.tb.Helper()

	got := toRGBA(a.Render())
	path := filepath.Join(a.opts.Dir, name+".png")

	if os.Getenv(UpdateEnv) != "" {
		if err := writePNG(path, got); err != nil {
			a.tb.Fatalf("apptest: writing golden %s: %v", path, err)
		}
		a.tb.Logf("apptest: wrote %s (%dx%d) — review it before committing",
			path, got.Bounds().Dx(), got.Bounds().Dy())
		return
	}

	res, err := compareAgainst(path, got, a.opts.Tol)
	if err != nil {
		if os.IsNotExist(err) {
			a.tb.Fatalf("apptest: no golden at %s.\n"+
				"Create it with:  %s=1 go test -run %s ./...\n"+
				"then look at the image before committing it.",
				path, UpdateEnv, a.tb.Name())
		}
		a.tb.Fatalf("apptest: reading golden %s: %v", path, err)
	}
	if res.ok(a.opts.Tol) {
		return
	}

	gotPath := filepath.Join(a.opts.Dir, name+".got.png")
	diffPath := filepath.Join(a.opts.Dir, name+".diff.png")
	_ = writePNG(gotPath, got)
	_ = writePNG(diffPath, res.diff)

	a.tb.Errorf("apptest: %s does not match the golden.\n"+
		"  %s\n"+
		"  tolerance:  max channel diff %d, at most %.3f%% of pixels\n"+
		"  rendered:   %s\n"+
		"  difference: %s (red marks changed pixels)\n"+
		"  golden:     %s\n"+
		"If the new rendering is correct, update with %s=1.",
		name, res, a.opts.Tol.MaxChannelDiff, a.opts.Tol.MaxDiffPct,
		gotPath, diffPath, path, UpdateEnv)
}

// compareAgainst loads the golden at path and compares got with it.
//
// It exists separately from Golden because testing.TB cannot be implemented
// outside the testing package, so a mock cannot capture what Golden reports.
// Keeping the decision here — load, compare, judge against tolerance — leaves
// Golden as reporting only, and lets the part that can be wrong be tested
// directly. The tolerance argument is unused in the comparison itself and is
// taken so callers cannot forget which one applied.
func compareAgainst(path string, got *image.RGBA, _ Tolerance) (result, error) {
	want, err := readPNG(path)
	if err != nil {
		return result{}, err
	}
	return compare(got, want), nil
}

// result is the outcome of comparing two images.
type result struct {
	sizeMismatch   bool
	gotW, gotH     int
	wantW, wantH   int
	total          int
	differing      int
	maxChannelDiff uint8
	diff           *image.RGBA
}

func (r result) diffPct() float64 {
	if r.total == 0 {
		return 0
	}
	return 100 * float64(r.differing) / float64(r.total)
}

func (r result) ok(t Tolerance) bool {
	if r.sizeMismatch {
		return false
	}
	return r.maxChannelDiff <= t.MaxChannelDiff && r.diffPct() <= t.MaxDiffPct
}

func (r result) String() string {
	if r.sizeMismatch {
		return fmt.Sprintf("size changed: rendered %dx%d, golden %dx%d",
			r.gotW, r.gotH, r.wantW, r.wantH)
	}
	return fmt.Sprintf("%d of %d pixels differ (%.3f%%), largest channel difference %d",
		r.differing, r.total, r.diffPct(), r.maxChannelDiff)
}

// compare diffs two images and builds a map of where they disagree: the golden
// dimmed to grey for context, with changed pixels in red. Somewhere to look
// first, rather than two images to flip between.
func compare(got, want *image.RGBA) result {
	gb, wb := got.Bounds(), want.Bounds()
	res := result{
		gotW: gb.Dx(), gotH: gb.Dy(),
		wantW: wb.Dx(), wantH: wb.Dy(),
	}
	if gb.Dx() != wb.Dx() || gb.Dy() != wb.Dy() {
		res.sizeMismatch = true
		return res
	}

	w, h := gb.Dx(), gb.Dy()
	res.total = w * h
	res.diff = image.NewRGBA(image.Rect(0, 0, w, h))

	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g := got.RGBAAt(x+gb.Min.X, y+gb.Min.Y)
			v := want.RGBAAt(x+wb.Min.X, y+wb.Min.Y)

			d := maxU8(
				absU8(g.R, v.R),
				absU8(g.G, v.G),
				absU8(g.B, v.B),
				absU8(g.A, v.A),
			)
			if d > res.maxChannelDiff {
				res.maxChannelDiff = d
			}

			if d == 0 {
				// Unchanged: dim the golden so changes stand out against it.
				lum := uint8((uint16(v.R) + uint16(v.G) + uint16(v.B)) / 3)
				q := 40 + lum/4
				res.diff.SetRGBA(x, y, color.RGBA{R: q, G: q, B: q, A: 255})
				continue
			}
			res.differing++
			// Changed: red, brighter the larger the difference.
			res.diff.SetRGBA(x, y, color.RGBA{R: 128 + d/2, G: 0, B: 0, A: 255})
		}
	}
	return res
}

func absU8(a, b uint8) uint8 {
	if a > b {
		return a - b
	}
	return b - a
}

func maxU8(vals ...uint8) uint8 {
	var m uint8
	for _, v := range vals {
		if v > m {
			m = v
		}
	}
	return m
}

func toRGBA(img image.Image) *image.RGBA {
	if r, ok := img.(*image.RGBA); ok {
		return r
	}
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			out.Set(x, y, img.At(x+b.Min.X, y+b.Min.Y))
		}
	}
	return out
}

func readPNG(path string) (*image.RGBA, error) {
	f, err := os.Open(path) //nolint:gosec // test-controlled path
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only

	img, err := png.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decoding %s: %w", path, err)
	}
	return toRGBA(img), nil
}

func writePNG(path string, img image.Image) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path) //nolint:gosec // test-controlled path
	if err != nil {
		return err
	}
	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}
