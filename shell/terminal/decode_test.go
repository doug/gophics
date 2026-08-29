package terminal

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"image"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

// kittyTerm is a minimal, spec-following decoder for the subset of the kitty
// graphics protocol this backend emits: full-image transmit+display (a=T) and
// in-place region compose onto the root frame (a=f), over temp-file (t=t) or
// chunked inline transfer. It reconstructs the displayed framebuffer so tests
// can verify the encoder produces output a correct terminal would render right —
// the stand-in for a real terminal we can't drive here.
type kittyTerm struct {
	w, h int
	buf  []byte // displayed RGBA (image id 1's root frame)

	pendKeys map[string]string
	pendData []byte
	pending  bool
}

func (k *kittyTerm) apply(t *testing.T, data []byte) {
	t.Helper()
	s := string(data)
	for {
		i := strings.Index(s, "\x1b_G")
		if i < 0 {
			return
		}
		rest := s[i+3:]
		before, after, ok := strings.Cut(rest, "\x1b\\")
		if !ok {
			return
		}
		k.command(t, before)
		s = after
	}
}

func (k *kittyTerm) command(t *testing.T, body string) {
	ctrl, payload := body, ""
	if before, after, ok := strings.Cut(body, ";"); ok {
		ctrl, payload = before, after
	}
	keys := parseKeys(ctrl)

	if _, isStart := keys["a"]; isStart {
		if keys["t"] == "t" { // temp-file transfer: payload is the base64 path
			fdata, err := os.ReadFile(string(mustB64(t, payload)))
			if err != nil {
				t.Fatalf("kitty t=t: read file: %v", err)
			}
			k.process(t, keys, fdata)
			return
		}
		data := mustB64(t, payload)
		if keys["m"] == "1" { // more chunks follow
			k.pendKeys, k.pendData, k.pending = keys, data, true
			return
		}
		k.finish(t, keys, data)
		return
	}
	if k.pending { // continuation chunk (only m=… present)
		k.pendData = append(k.pendData, mustB64(t, payload)...)
		if keys["m"] == "0" {
			k.pending = false
			k.finish(t, k.pendKeys, k.pendData)
		}
	}
}

// finish decompresses the accumulated payload when o=z, then applies it.
func (k *kittyTerm) finish(t *testing.T, keys map[string]string, data []byte) {
	if keys["o"] == "z" {
		r, err := zlib.NewReader(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("zlib reader: %v", err)
		}
		if data, err = io.ReadAll(r); err != nil {
			t.Fatalf("zlib read: %v", err)
		}
	}
	k.process(t, keys, data)
}

func (k *kittyTerm) process(t *testing.T, keys map[string]string, data []byte) {
	switch keys["a"] {
	case "T": // transmit + display a full image
		k.w, k.h = atoi(keys["s"]), atoi(keys["v"])
		k.buf = make([]byte, k.w*k.h*4)
		copy(k.buf, data)
	case "f": // compose a rectangle onto the root frame in place
		x, y := atoi(keys["x"]), atoi(keys["y"])
		w, h := atoi(keys["s"]), atoi(keys["v"])
		for row := range h {
			dst := ((y+row)*k.w + x) * 4
			src := row * w * 4
			copy(k.buf[dst:dst+w*4], data[src:src+w*4])
		}
	case "d": // delete: irrelevant to reconstruction
	}
}

func (k *kittyTerm) pixels() []byte { return k.buf }

func parseKeys(ctrl string) map[string]string {
	m := map[string]string{}
	for kv := range strings.SplitSeq(ctrl, ",") {
		if before, after, ok := strings.Cut(kv, "="); ok {
			m[before] = after
		}
	}
	return m
}

func atoi(s string) int { n, _ := strconv.Atoi(s); return n }

func mustB64(t *testing.T, s string) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(s)
	if err != nil {
		t.Fatalf("bad base64 payload: %v", err)
	}
	return b
}

// TestPartialUpdateReconstructs is the core correctness check: it plays a
// sequence of frames (initial, two small local changes, then a large change)
// through present() and confirms a spec-correct terminal reconstructs each frame
// exactly — for both inline and temp-file transfer.
func TestPartialUpdateReconstructs(t *testing.T) {
	const w, h = 40, 30
	base := gradient(w, h)
	f1 := clone(base)
	fillRect(f1, 10, 8, 5, 5, 255, 0, 0)
	f2 := clone(f1)
	fillRect(f2, 30, 22, 4, 4, 0, 0, 255)
	f3 := solid(w, h, 0, 200, 0) // whole-screen change → full re-transmit

	frames := []*image.RGBA{base, f1, f2, f3}

	for _, tc := range []struct {
		name string
		dir  string
	}{
		{"inline", ""},
		{"tempfile", t.TempDir()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			ts := &termState{out: &out, imageID: 1, dir: tc.dir}
			term := &kittyTerm{}
			for i, f := range frames {
				out.Reset()
				ts.present(f)
				term.apply(t, out.Bytes())
				if got, want := term.pixels(), tightPixels(f); !bytes.Equal(got, want) {
					t.Fatalf("frame %d: reconstruction mismatch (%d differing bytes)",
						i, countDiff(got, want))
				}
			}
		})
	}
}

func TestPartialUpdateSendsOnlyDamage(t *testing.T) {
	const w, h = 256, 256 // several tiles across
	base := solid(w, h, 20, 20, 20)
	next := clone(base)
	fillRect(next, 100, 100, 4, 4, 255, 255, 255) // one tile's worth of change

	var out bytes.Buffer
	ts := &termState{out: &out, imageID: 1} // inline, so payload size is visible
	ts.present(base)
	firstLen := out.Len()
	out.Reset()
	ts.present(next)

	if !strings.Contains(out.String(), "a=f") {
		t.Errorf("small change should compose (a=f), got: %q", firstBytes(out.String()))
	}
	small := out.Len()

	// The claim is that the update carries the damage, not the frame. Asserting
	// a fraction of the full frame measured the compressor instead: a solid
	// 256×256 base is almost pure redundancy, and Go 1.27's flate shrank it far
	// more than the one-tile patch — which is mostly escape sequences and
	// base64 — so the ratio moved while both got smaller.
	//
	// The invariant that actually holds is that the same one-tile change costs
	// the same on a much bigger canvas.
	const w2, h2 = 1024, 1024 // 16× the area
	base2 := solid(w2, h2, 20, 20, 20)
	next2 := clone(base2)
	fillRect(next2, 100, 100, 4, 4, 255, 255, 255)

	var out2 bytes.Buffer
	ts2 := &termState{out: &out2, imageID: 1}
	ts2.present(base2)
	out2.Reset()
	ts2.present(next2)

	t.Logf("one-tile change: %d B on %dx%d, %d B on %dx%d (full base was %d B)",
		small, w, h, out2.Len(), w2, h2, firstLen)
	if out2.Len() > small*2 {
		t.Errorf("one-tile update grew from %d B to %d B on a 16× larger frame; "+
			"it should carry the damage, not the frame", small, out2.Len())
	}
}

// TestScatteredChangesSendFewTiles verifies that two far-apart small changes
// transmit only their two tiles — not one bounding box spanning the gap — and
// still reconstruct exactly.
func TestScatteredChangesSendFewTiles(t *testing.T) {
	const w, h = 320, 320
	base := solid(w, h, 10, 10, 10)
	next := clone(base)
	fillRect(next, 20, 20, 6, 6, 255, 0, 0)   // top-left tile
	fillRect(next, 280, 280, 6, 6, 0, 0, 255) // bottom-right tile

	var out bytes.Buffer
	ts := &termState{out: &out, imageID: 1}
	ts.present(base)
	out.Reset()
	ts.present(next)

	// Exactly two compose commands (one per touched tile).
	if n := strings.Count(out.String(), "a=f"); n != 2 {
		t.Errorf("scattered changes should send 2 tiles, got %d a=f commands", n)
	}
	// A single-bbox strategy would span 260×260 ≈ 67k px; two tiles are 2×64² ≈
	// 8k px, so the payload must be far smaller than that bbox would imply.
	term := &kittyTerm{}
	fresh := &termState{out: &bytes.Buffer{}, imageID: 1}
	// Reconstruct from scratch through the decoder to confirm correctness.
	var full bytes.Buffer
	fresh.out = &full
	fresh.present(base)
	term.apply(t, full.Bytes())
	full.Reset()
	fresh.present(next)
	term.apply(t, full.Bytes())
	if !bytes.Equal(term.pixels(), tightPixels(next)) {
		t.Fatalf("scattered-change reconstruction mismatch (%d bytes differ)",
			countDiff(term.pixels(), tightPixels(next)))
	}
}

// --- image helpers ---

func gradient(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			o := y*img.Stride + x*4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] =
				byte(x*6), byte(y*8), byte(x+y), 255
		}
	}
	return img
}

func solid(w, h int, r, g, b byte) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for i := 0; i < len(img.Pix); i += 4 {
		img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = r, g, b, 255
	}
	return img
}

func clone(src *image.RGBA) *image.RGBA {
	dst := image.NewRGBA(src.Bounds())
	copy(dst.Pix, src.Pix)
	return dst
}

func fillRect(img *image.RGBA, x, y, w, h int, r, g, b byte) {
	for dy := range h {
		for dx := range w {
			o := (y+dy)*img.Stride + (x+dx)*4
			img.Pix[o], img.Pix[o+1], img.Pix[o+2], img.Pix[o+3] = r, g, b, 255
		}
	}
}

func countDiff(a, b []byte) int {
	n := 0
	for i := range a {
		if a[i] != b[i] {
			n++
		}
	}
	return n
}

func firstBytes(s string) string {
	if len(s) > 80 {
		return s[:80]
	}
	return s
}
