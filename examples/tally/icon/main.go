// Command icon renders Tally's app icon and writes the platform asset sets.
//
// The icon is drawn by gophics itself rather than exported from a design tool:
// the renderer that draws the app draws its icon, at any size, from one
// description. That also means the icon is diffable source rather than a binary
// blob, and regenerating every platform's sizes is one command.
//
//	go run ./icon -out .        # writes icon.png + platform asset sets
package main

import (
	"flag"
	"fmt"
	"image"
	"image/png"
	"log"
	"os"
	"path/filepath"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Palette — the app's warm neutral over a deep ink ground, so the mark reads at
// 16px in a menu bar as well as 1024px in the store listing.
var (
	bgTop    = paint.RGB(0.129, 0.129, 0.145)
	bgBottom = paint.RGB(0.078, 0.078, 0.090)
	ink      = paint.RGB(0.973, 0.965, 0.949)
	accent   = paint.RGB(0.851, 0.451, 0.325)
	muted    = paint.RGB(0.545, 0.541, 0.529)
)

// draw paints the icon into a square canvas of side s.
//
// The mark is a stack of ledger rules with a rising line over them: a ledger and
// its trend, which is what the app is. Everything is expressed as a fraction of
// the side so it renders identically at every size.
func draw(c paint.Canvas, s float32) {
	u := func(f float32) float32 { return f * s } // fraction of the side

	// Ground: a subtle vertical gradient so the icon has depth without detail
	// that would muddy at small sizes. The corner radius matches Apple's
	// squircle closely enough at icon sizes; the platforms mask it anyway.
	full := geom.Rect{Max: geom.Pt{X: s, Y: s}}
	c.FillRRectGradient(full, u(0.22), bgTop, bgBottom, false)

	// Ledger rules: four lines of varying length, like entries on a page. All one
	// muted tone — an accented rule sat under the trend line and read as an
	// artifact rather than emphasis, so the colour is spent on the endpoint
	// instead, where nothing overlaps it.
	lineH := u(0.045)
	left := u(0.22)
	widths := []float32{0.56, 0.44, 0.50, 0.34}
	top := u(0.30)
	gap := u(0.115)
	for i, w := range widths {
		y := top + float32(i)*gap
		c.FillRRect(geom.Rect{
			Min: geom.Pt{X: left, Y: y},
			Max: geom.Pt{X: left + u(w), Y: y + lineH},
		}, lineH/2, muted)
	}

	// The trend line rising across the rules, drawn in the app's ink so it reads
	// as the subject rather than decoration.
	pts := []geom.Pt{
		{X: u(0.20), Y: u(0.72)},
		{X: u(0.38), Y: u(0.60)},
		{X: u(0.54), Y: u(0.66)},
		{X: u(0.80), Y: u(0.30)},
	}
	width := u(0.075)
	for i := 1; i < len(pts); i++ {
		c.Line(pts[i-1], pts[i], width, ink)
		// Round the joints so the polyline reads as one stroke at small sizes.
		dot(c, pts[i-1], width/2, ink)
	}
	dot(c, pts[len(pts)-1], width*0.85, accent)
}

// dot fills a circle; the canvas draws rounded rects, and a square one with a
// half-side radius is exactly a circle.
func dot(c paint.Canvas, at geom.Pt, r float32, col paint.Color) {
	c.FillRRect(geom.Rect{
		Min: geom.Pt{X: at.X - r, Y: at.Y - r},
		Max: geom.Pt{X: at.X + r, Y: at.Y + r},
	}, r, col)
}

// render rasterizes the icon at one size.
func render(size int) (image.Image, error) {
	p := paint.NewPainter()
	s := float32(size)
	c := p.BeginOffscreen(geom.Size{W: s, H: s}, 1)
	draw(c, s)
	img := p.SurfaceRGBA()
	if img == nil {
		return nil, fmt.Errorf("icon: renderer produced no surface")
	}
	// Copy: the painter reuses its surface between renders.
	out := image.NewRGBA(img.Bounds())
	copy(out.Pix, img.Pix)
	return out, nil
}

func writePNG(path string, size int) error {
	img, err := render(size)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}

func main() {
	out := flag.String("out", ".", "directory to write assets into")
	flag.Parse()

	// The master, plus a macOS iconset (iconutil turns it into .icns), the iOS
	// app-icon slot, and Android mipmaps.
	jobs := []struct {
		path string
		size int
	}{
		{"icon.png", 1024},
		{"icon.iconset/icon_16x16.png", 16},
		{"icon.iconset/icon_16x16@2x.png", 32},
		{"icon.iconset/icon_32x32.png", 32},
		{"icon.iconset/icon_32x32@2x.png", 64},
		{"icon.iconset/icon_128x128.png", 128},
		{"icon.iconset/icon_128x128@2x.png", 256},
		{"icon.iconset/icon_256x256.png", 256},
		{"icon.iconset/icon_256x256@2x.png", 512},
		{"icon.iconset/icon_512x512.png", 512},
		{"icon.iconset/icon_512x512@2x.png", 1024},
		// iOS takes a single 1024 master and slices the rest itself.
		{"ios/Tally/Assets.xcassets/AppIcon.appiconset/icon-1024.png", 1024},
		// Android mipmap densities.
		{"android/app/src/main/res/mipmap-mdpi/ic_launcher.png", 48},
		{"android/app/src/main/res/mipmap-hdpi/ic_launcher.png", 72},
		{"android/app/src/main/res/mipmap-xhdpi/ic_launcher.png", 96},
		{"android/app/src/main/res/mipmap-xxhdpi/ic_launcher.png", 144},
		{"android/app/src/main/res/mipmap-xxxhdpi/ic_launcher.png", 192},
	}
	for _, j := range jobs {
		if err := writePNG(filepath.Join(*out, j.path), j.size); err != nil {
			log.Fatalf("%s: %v", j.path, err)
		}
	}
	fmt.Printf("wrote %d icon files under %s\n", len(jobs), *out)
}
