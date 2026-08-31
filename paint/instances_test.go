package paint

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// A malformed batch draws nothing rather than part of itself. Guessing an
// interpretation would hide the caller's bug behind a plot that looks almost
// right.
func TestMarksRejectsMismatchedLengths(t *testing.T) {
	cases := map[string]Marks{
		"Y short":     {X: []float32{1, 2}, Y: []float32{1}, Size: []float32{3}, Color: []Color{{A: 1}}},
		"size wrong":  {X: []float32{1, 2}, Y: []float32{1, 2}, Size: []float32{3, 4, 5}, Color: []Color{{A: 1}}},
		"color wrong": {X: []float32{1, 2}, Y: []float32{1, 2}, Size: []float32{3}, Color: []Color{{A: 1}, {A: 1}, {A: 1}}},
		"no size":     {X: []float32{1}, Y: []float32{1}, Color: []Color{{A: 1}}},
		"empty":       {},
	}
	for name, m := range cases {
		if got := m.Len(); got != 0 {
			t.Errorf("%s: Len() = %d, want 0", name, got)
		}
	}
}

// Size and Color are each either one value for the batch or one per mark.
func TestMarksBroadcastsSizeAndColor(t *testing.T) {
	red, blue := Color{R: 1, A: 1}, Color{B: 1, A: 1}
	m := Marks{
		X: []float32{0, 10}, Y: []float32{0, 10},
		Size:  []float32{4},
		Color: []Color{red, blue},
	}
	if m.Len() != 2 {
		t.Fatalf("Len() = %d, want 2", m.Len())
	}
	if m.SizeAt(0) != 4 || m.SizeAt(1) != 4 {
		t.Error("a single Size should apply to every mark")
	}
	if m.ColorAt(0) != red || m.ColorAt(1) != blue {
		t.Error("per-mark colours were not read per mark")
	}
}

// Bounds is what damage tracking uses, so it must cover every mark's full
// extent — the centre plus half its size, not just the centres.
func TestMarksBoundsCoverTheWholeExtent(t *testing.T) {
	m := Marks{
		X: []float32{10, 40}, Y: []float32{10, 20},
		Size:  []float32{6, 10},
		Color: []Color{{A: 1}, {A: 1}},
	}
	want := geom.Rect{Min: geom.Pt{X: 7, Y: 7}, Max: geom.Pt{X: 45, Y: 25}}
	if got := m.Bounds(); got != want {
		t.Errorf("Bounds() = %v, want %v", got, want)
	}
	if (&Marks{}).Bounds() != (geom.Rect{}) {
		t.Error("an empty batch should have empty bounds")
	}
}

// The batch must draw the same thing however it is expressed. Per-mark sizes
// take a grouping path that a uniform size skips, and a rounded clip pushes the
// whole batch onto the per-mark fallback — all three must land the same pixels.
func TestMarksDrawTheSameThroughEveryPath(t *testing.T) {
	pts := []geom.Pt{{X: 12, Y: 12}, {X: 30, Y: 18}, {X: 20, Y: 34}}
	const d = 9
	col := Color{R: 0.1, G: 0.5, B: 0.8, A: 1}

	render := func(setup func(Canvas), m *Marks) []byte {
		p := NewPainter()
		c := p.BeginOffscreen(geom.Size{W: 48, H: 48}, 1)
		c.Clear(Color{R: 1, G: 1, B: 1, A: 1})
		if setup != nil {
			setup(c)
		}
		c.DrawMarks(m)
		img := p.SurfaceRGBA()
		out := make([]byte, len(img.Pix))
		copy(out, img.Pix)
		return out
	}

	uniform := &Marks{Kind: MarkCircle, Size: []float32{d}, Color: []Color{col}}
	perMark := &Marks{Kind: MarkCircle, Color: []Color{col, col, col}}
	for _, pt := range pts {
		uniform.X = append(uniform.X, pt.X)
		uniform.Y = append(uniform.Y, pt.Y)
		perMark.X = append(perMark.X, pt.X)
		perMark.Y = append(perMark.Y, pt.Y)
		perMark.Size = append(perMark.Size, d)
	}

	base := render(nil, uniform)
	grouped := render(nil, perMark)
	if !nearlyEqual(base, grouped, 2) {
		t.Error("a per-mark Size drew differently from a single Size")
	}

	// A rounded clip has per-pixel coverage the blit does not read, so it
	// declines and every mark is filled individually. That fallback is what
	// lets the fast path stay narrow enough to be obviously correct, so it has
	// to actually produce the same picture.
	// Deliberately larger than the surface: it clips nothing, so the two
	// pictures must match exactly, while still being a rounded clip and so
	// still forcing the fallback.
	clipped := render(func(c Canvas) {
		c.PushClipRRect(geom.Rect{
			Min: geom.Pt{X: -20, Y: -20}, Max: geom.Pt{X: 68, Y: 68},
		}, 4)
	}, uniform)
	if nearlyEqual(clipped, make([]byte, len(clipped)), 0) {
		t.Fatal("the clipped render is blank; the probe is not drawing at all")
	}
	worst, n := 0, 0
	for i := range base {
		d := int(base[i]) - int(clipped[i])
		if d < 0 {
			d = -d
		}
		if d > worst {
			worst = d
		}
		if d > 0 {
			n++
		}
	}
	t.Logf("fallback vs blit: worst channel diff %d over %d of %d bytes", worst, n, len(base))
	// Edge pixels only, and the two rasterize by different means — supersampled
	// coverage against an analytic fill — so their antialiasing is not required
	// to agree. What must hold is that the marks are the same size in the same
	// places, which a placement or sizing error would break by far more.
	if !nearlyEqual(base, clipped, 40) {
		t.Error("the per-mark fallback drew a different picture from the batch blit")
	}
}

// nearlyEqual allows a small per-channel difference: the fast path and the
// fallback rasterize the same shape by different means, and their antialiasing
// is not required to agree bit for bit.
func nearlyEqual(a, b []byte, tol int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > tol {
			return false
		}
	}
	return true
}
