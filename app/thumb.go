package app

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"strconv"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// maybeCaptureThumb renders the app headless to a PNG and reports done=true when
// GOPHICS_THUMB is set, so a plain `go run` of any example produces a gallery
// thumbnail with no display, no browser, and no GPU. The CPU render is
// pixel-identical to every backend (the point of the stack), so the thumbnail
// matches what ships. Env knobs:
//
//	GOPHICS_THUMB          output PNG path (required to activate)
//	GOPHICS_THUMB_SIZE     logical WxH to render at (default: cfg.Size)
//	GOPHICS_THUMB_OUT      final WxH to downscale to (default: no downscale)
//	GOPHICS_THUMB_SCALE    device-pixel scale to render at (default: 2, supersamples)
//	GOPHICS_THUMB_SETTLE   max seconds of animation to step before capture (default: 8)
//	GOPHICS_THUMB_REALTIME if set, also sleep real time each step so async work
//	                       (e.g. a network fetch) can land before capture
func maybeCaptureThumb(root widget.Widget, cfg Config) (bool, error) {
	path := os.Getenv("GOPHICS_THUMB")
	if path == "" {
		return false, nil
	}

	size := cfg.Size
	if s := os.Getenv("GOPHICS_THUMB_SIZE"); s != "" {
		if w, h, ok := parseWH(s); ok {
			size = geom.Size{W: w, H: h}
		}
	}
	scale := float32(2)
	if s := os.Getenv("GOPHICS_THUMB_SCALE"); s != "" {
		if v, err := strconv.ParseFloat(s, 32); err == nil && v > 0 {
			scale = float32(v)
		}
	}
	settle := 8.0
	if s := os.Getenv("GOPHICS_THUMB_SETTLE"); s != "" {
		if v, err := strconv.ParseFloat(s, 64); err == nil && v >= 0 {
			settle = v
		}
	}
	realtime := os.Getenv("GOPHICS_THUMB_REALTIME") != ""

	// Deterministic CPU rasterization — no accelerator, no display.
	paint.UseCPU()

	tc := cfg
	tc.Size = size
	h, err := NewHeadless(root, tc, scale)
	if err != nil {
		return true, fmt.Errorf("thumb: headless: %w", err)
	}

	// Step animation forward so one-shot intros (e.g. a deal) settle and
	// continuous scenes evolve past their empty first frame. Stop early once
	// nothing is animating (and no realtime wait was requested).
	const dt = 1.0 / 60.0
	for t := 0.0; t < settle; t += dt {
		running := h.Step(dt)
		h.Render()
		if realtime {
			time.Sleep(time.Second / 60)
		} else if !running {
			break
		}
	}
	img := h.Render()

	if s := os.Getenv("GOPHICS_THUMB_OUT"); s != "" {
		if w, hh, ok := parseWH(s); ok {
			img = downscale(img, int(w), int(hh))
		}
	}

	if err := writePNG(path, img); err != nil {
		return true, fmt.Errorf("thumb: %w", err)
	}
	fmt.Fprintf(os.Stderr, "wrote %s (%dx%d)\n", path, img.Bounds().Dx(), img.Bounds().Dy())
	return true, nil
}

// parseWH parses a "WxH" string into positive dimensions.
func parseWH(s string) (w, h float32, ok bool) {
	i := strings.IndexAny(s, "xX")
	if i < 0 {
		return 0, 0, false
	}
	wv, err1 := strconv.ParseFloat(s[:i], 32)
	hv, err2 := strconv.ParseFloat(s[i+1:], 32)
	if err1 != nil || err2 != nil || wv <= 0 || hv <= 0 {
		return 0, 0, false
	}
	return float32(wv), float32(hv), true
}

// downscale resamples src to w×h with a high-quality filter.
func downscale(src image.Image, w, h int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Src, nil)
	return dst
}

func writePNG(path string, img image.Image) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return png.Encode(f, img)
}
