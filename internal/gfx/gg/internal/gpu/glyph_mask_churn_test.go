//go:build !nogpu

package gpu

import (
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/text"
)

// TestGlyphMaskSurvivesAtlasChurn reproduces, without a phone, the corruption a
// real device shows after a while of use: text that has been on screen all
// along starts drawing as other text and does not recover.
//
// The shape of the reproduction is the shape of the bug. One line is rendered
// and its pixels kept. Then many frames of *different* text are rendered, which
// is what scrolling a catalog does — new glyphs, new pages, compaction running.
// Then the original line is rendered again. Nothing about it changed, so it
// must come back identical. When it does not, the glyphs it references are no
// longer where its quads say they are.
//
// This runs the real Metal/Vulkan pipeline headlessly, so it iterates in
// seconds and needs nobody watching a screen.
func TestGlyphMaskSurvivesAtlasChurn(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48
	face := reproFont(t)
	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue, testSampleCount(t, device))

	if err := session.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensureClipBindLayout: %v", err)
	}
	if err := session.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline: %v", err)
	}

	// render draws one line and returns the framebuffer.
	render := func(t *testing.T, line string) []uint8 {
		t.Helper()
		var batches []GlyphMaskBatch
		x := 6.0
		for _, w := range strings.Fields(line) {
			var glyphs []text.ShapedGlyph
			for g := range face.Glyphs(w) {
				glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
			}
			b, err := engine.LayoutShapedGlyphs(face, glyphs, x, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
			if err != nil {
				t.Fatalf("LayoutShapedGlyphs %q: %v", w, err)
			}
			if len(b.Quads) > 0 {
				batches = append(batches, b)
			}
			adv, _ := text.Measure(w+" ", face)
			x += adv
		}
		if len(batches) == 0 {
			t.Fatalf("no batches for %q", line)
		}
		if err := engine.SyncAtlasTextures(device, queue); err != nil {
			t.Fatalf("SyncAtlasTextures: %v", err)
		}
		for i, b := range batches {
			view := engine.PageTextureView(b.AtlasPageIndex)
			if view == nil {
				t.Fatalf("nil atlas view for batch %d", i)
			}
			session.SetGlyphMaskAtlasView(i, view, b.IsLCD)
		}
		data := make([]uint8, W*H*4)
		for i := range data {
			data[i] = 255
		}
		target := gg.GPURenderTarget{Data: data, Width: W, Height: H, Stride: W * 4}
		group := ScissorGroup{GlyphMaskBatches: batches}
		if err := session.RenderFrameGrouped(target, []ScissorGroup{group}, nil, nil); err != nil {
			t.Fatalf("RenderFrameGrouped: %v", err)
		}
		engine.AdvanceFrame()
		return data
	}

	const subject = "The quick brown fox"
	before := render(t, subject)

	// Churn: many frames of text the subject shares no glyphs with where it can
	// be helped, which is what moving around an app produces.
	// Distinct glyphs are what fill an atlas, so the churn walks a wide slice
	// of Unicode rather than repeating the same letters at different numbers.
	const churnFrames = 400
	runes := []rune{}
	for r := rune(0x21); r < 0x24F; r++ {
		runes = append(runes, r)
	}
	for i := 0; i < churnFrames; i++ {
		var sb strings.Builder
		for j := 0; j < 24; j++ {
			sb.WriteRune(runes[(i*24+j)%len(runes)])
			if j%4 == 3 {
				sb.WriteByte(' ')
			}
		}
		render(t, sb.String())
	}
	ref, ev, cmp := text.AtlasStats()
	t.Logf("after churn: ref=%d ev=%d cmp=%d uploads=%d", ref, ev, cmp, text.AtlasUploads())

	after := render(t, subject)

	diff := 0
	for i := range before {
		if before[i] != after[i] {
			diff++
		}
	}
	if diff > 0 {
		pct := float64(diff) * 100 / float64(len(before))
		t.Errorf("the same line rendered differently after %d frames of other text: "+
			"%d of %d bytes differ (%.1f%%) — its glyphs are no longer where its "+
			"quads point", churnFrames, diff, len(before), pct)
	}
}

// TestGlyphBatchWithNoAtlasViewIsNotDrawnFromAnotherPage pins the hazard the
// counters pointed at.
//
// Bind groups live in a slice indexed by batch position and persist between
// frames. The live path sets a batch's atlas view only when the engine has one
// — and when it does not, the batch is still drawn, with whatever page the
// same index was given on an earlier frame. Its glyphs then come out of
// another page: text rendered as other text, with the CPU atlas entirely
// correct and no allocation having failed, which is precisely the state the
// device counters showed.
//
// The test renders a line, then renders a different line whose view is
// deliberately withheld, and asserts the second does not come back as the
// first.
func TestGlyphBatchWithNoAtlasViewIsNotDrawnFromAnotherPage(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48
	face := reproFont(t)
	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue, testSampleCount(t, device))
	if err := session.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensureClipBindLayout: %v", err)
	}
	if err := session.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline: %v", err)
	}

	batchFor := func(t *testing.T, line string) GlyphMaskBatch {
		t.Helper()
		var glyphs []text.ShapedGlyph
		for g := range face.Glyphs(line) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		b, err := engine.LayoutShapedGlyphs(face, glyphs, 6, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs %q: %v", line, err)
		}
		if len(b.Quads) == 0 {
			t.Fatalf("no quads for %q", line)
		}
		return b
	}

	draw := func(t *testing.T, b GlyphMaskBatch, bindView bool) []uint8 {
		t.Helper()
		if err := engine.SyncAtlasTextures(device, queue); err != nil {
			t.Fatalf("SyncAtlasTextures: %v", err)
		}
		if bindView {
			if v := engine.PageTextureView(b.AtlasPageIndex); v != nil {
				session.SetGlyphMaskAtlasView(0, v, b.IsLCD)
			}
		}
		data := make([]uint8, W*H*4)
		for i := range data {
			data[i] = 255
		}
		target := gg.GPURenderTarget{Data: data, Width: W, Height: H, Stride: W * 4}
		if err := session.RenderFrameGrouped(target,
			[]ScissorGroup{{GlyphMaskBatches: []GlyphMaskBatch{b}}}, nil, nil); err != nil {
			t.Fatalf("RenderFrameGrouped: %v", err)
		}
		engine.AdvanceFrame()
		return data
	}

	first := draw(t, batchFor(t, "AAAAAAAA"), true)

	// Second line, its view deliberately not rebound — the state the live path
	// leaves a batch in when PageTextureView returns nil.
	second := batchFor(t, "wwwwwwww")
	stale := draw(t, second, false)

	same := 0
	for i := range first {
		if first[i] == stale[i] {
			same++
		}
	}
	if same == len(first) {
		t.Error("a batch drawn without its atlas view rebound produced pixel-identical " +
			"output to the previous, different line — it sampled the earlier " +
			"batch's atlas page, which is text drawn as other text")
	}
}

// TestGlyphWrittenAfterUploadDrawsGarbage renders the one ordering the device
// counters leave room for, and writes both frames out as PNGs so the failure
// can be looked at rather than described.
//
// The live frame is: lay out every glyph, upload the dirty atlas pages, bind,
// draw. A glyph that reaches the atlas after that upload is in the CPU page
// and not in the GPU texture, so the quad drawn for it samples whatever those
// texels held before — nothing, or an older glyph. That is text drawn as other
// text with no allocation failing and no page moving, which is exactly the
// state the device reported: ref=0 ev=0 cmp=0.
//
// Set GMCHURN_OUT to a directory to keep the PNGs.
func TestGlyphWrittenAfterUploadDrawsGarbage(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48
	face := reproFont(t)
	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue, testSampleCount(t, device))
	if err := session.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensureClipBindLayout: %v", err)
	}
	if err := session.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline: %v", err)
	}

	layout := func(t *testing.T, line string) GlyphMaskBatch {
		t.Helper()
		var glyphs []text.ShapedGlyph
		for g := range face.Glyphs(line) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		b, err := engine.LayoutShapedGlyphs(face, glyphs, 6, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs %q: %v", line, err)
		}
		return b
	}

	white := func() []uint8 {
		d := make([]uint8, W*H*4)
		for i := range d {
			d[i] = 255
		}
		return d
	}
	ink := func(d []uint8) int {
		n := 0
		for i := 0; i < len(d); i += 4 {
			if d[i] != 255 || d[i+1] != 255 || d[i+2] != 255 {
				n++
			}
		}
		return n
	}

	// Correct ordering: lay out, then upload, then draw.
	good := layout(t, "correct ordering")
	if err := engine.SyncAtlasTextures(device, queue); err != nil {
		t.Fatalf("sync: %v", err)
	}
	if v := engine.PageTextureView(good.AtlasPageIndex); v != nil {
		session.SetGlyphMaskAtlasView(0, v, good.IsLCD)
	}
	okFrame := white()
	if err := session.RenderFrameGrouped(gg.GPURenderTarget{Data: okFrame, Width: W, Height: H, Stride: W * 4},
		[]ScissorGroup{{GlyphMaskBatches: []GlyphMaskBatch{good}}}, nil, nil); err != nil {
		t.Fatalf("render good: %v", err)
	}
	engine.AdvanceFrame()

	// Wrong ordering: upload first, then bring in glyphs never seen before, then
	// draw them without a second upload.
	if err := engine.SyncAtlasTextures(device, queue); err != nil {
		t.Fatalf("sync: %v", err)
	}
	lateBatch := layout(t, "zyxwvu qponml") // glyphs the atlas has not held before
	if v := engine.PageTextureView(lateBatch.AtlasPageIndex); v != nil {
		session.SetGlyphMaskAtlasView(0, v, lateBatch.IsLCD)
	}
	badFrame := white()
	if err := session.RenderFrameGrouped(gg.GPURenderTarget{Data: badFrame, Width: W, Height: H, Stride: W * 4},
		[]ScissorGroup{{GlyphMaskBatches: []GlyphMaskBatch{lateBatch}}}, nil, nil); err != nil {
		t.Fatalf("render late: %v", err)
	}

	if out := os.Getenv("GMCHURN_OUT"); out != "" {
		writePNG(t, filepath.Join(out, "ordering_good.png"), okFrame, W, H)
		writePNG(t, filepath.Join(out, "ordering_late.png"), badFrame, W, H)
		t.Logf("wrote PNGs to %s", out)
	}

	goodInk, badInk := ink(okFrame), ink(badFrame)
	t.Logf("ink: correct=%d late=%d", goodInk, badInk)
	if badInk == 0 && goodInk > 0 {
		t.Error("glyphs rasterized after the frame's upload drew nothing at all — " +
			"the GPU texture does not have them yet")
	}
}

func writePNG(t *testing.T, path string, rgba []uint8, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	copy(img.Pix, rgba)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
}

// TestTextSurvivesADeviceChange exercises the one path a phone takes and a
// desktop never does.
//
// When the surface is recreated — backgrounding, rotation, memory pressure —
// the wgpu device is recreated with it, and SyncAtlasTextures drops every page
// texture and re-uploads. The CPU atlas is untouched by that, which is exactly
// the state the device counters reported during the corruption: no refusals,
// no evictions, no compactions, and wrong pixels.
//
// The bind groups are the hazard. They live in a slice indexed by batch and
// persist across frames, each holding a texture view. After a device change
// those views belong to a destroyed device, so a batch drawn before its bind
// group is rebuilt samples released memory.
func TestTextSurvivesADeviceChange(t *testing.T) {
	device, queue, cleanup := reproRealDevice(t)
	defer cleanup()

	const W, H = 512, 48
	face := reproFont(t)
	engine := NewGlyphMaskEngine()
	session := NewGPURenderSession(device, queue, testSampleCount(t, device))
	if err := session.ensureClipBindLayout(); err != nil {
		t.Fatalf("ensureClipBindLayout: %v", err)
	}
	if err := session.ensureGlyphMaskPipeline(false); err != nil {
		t.Fatalf("ensureGlyphMaskPipeline: %v", err)
	}

	batch := func(t *testing.T, line string) GlyphMaskBatch {
		t.Helper()
		var glyphs []text.ShapedGlyph
		for g := range face.Glyphs(line) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		b, err := engine.LayoutShapedGlyphs(face, glyphs, 6, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs: %v", err)
		}
		return b
	}
	frame := func(t *testing.T, b GlyphMaskBatch) []uint8 {
		t.Helper()
		if err := engine.SyncAtlasTextures(device, queue); err != nil {
			t.Fatalf("SyncAtlasTextures: %v", err)
		}
		if v := engine.PageTextureView(b.AtlasPageIndex); v != nil {
			session.SetGlyphMaskAtlasView(0, v, b.IsLCD)
		}
		data := make([]uint8, W*H*4)
		for i := range data {
			data[i] = 255
		}
		if err := session.RenderFrameGrouped(gg.GPURenderTarget{Data: data, Width: W, Height: H, Stride: W * 4},
			[]ScissorGroup{{GlyphMaskBatches: []GlyphMaskBatch{b}}}, nil, nil); err != nil {
			t.Fatalf("RenderFrameGrouped: %v", err)
		}
		engine.AdvanceFrame()
		return data
	}

	line := batch(t, "device change")
	before := frame(t, line)

	// A second device, as a recreated surface produces. The engine notices and
	// drops its page textures.
	device2, queue2, cleanup2 := reproRealDevice(t)
	defer cleanup2()
	if err := engine.SyncAtlasTextures(device2, queue2); err != nil {
		t.Fatalf("SyncAtlasTextures after device change: %v", err)
	}

	// Draw again on the original device, as a frame in flight across the change
	// would. Whatever happens, it must not read freed memory or come back blank.
	after := frame(t, batch(t, "device change"))

	ink := func(d []uint8) int {
		n := 0
		for i := 0; i < len(d); i += 4 {
			if d[i] != 255 || d[i+1] != 255 || d[i+2] != 255 {
				n++
			}
		}
		return n
	}
	if b, a := ink(before), ink(after); a == 0 && b > 0 {
		t.Errorf("text drew %d ink pixels before a device change and %d after; "+
			"the atlas page textures were dropped and the batch drew from views "+
			"belonging to a destroyed device", b, a)
	}
}

// TestBatchesReferenceThePageTheirGlyphsAreOn is the test that would have
// caught the fault a phone took twenty minutes of navigation to show.
//
// Every batch used to report page 0 — "currently single page support" — while
// the atlas has always had four and fills the first before opening the second.
// Once a run's glyphs landed on page 1, the batch still bound page 0, and
// since texture coordinates are normalised per page those glyphs sampled
// whatever else occupied that spot: the same wrong letter, every time, with a
// perfectly correct atlas and every allocation counter reading zero.
func TestBatchesReferenceThePageTheirGlyphsAreOn(t *testing.T) {
	_, _, cleanup := reproRealDevice(t)
	defer cleanup()

	face := reproFont(t)
	engine := NewGlyphMaskEngine()

	layout := func(s string) GlyphMaskBatch {
		var glyphs []text.ShapedGlyph
		for g := range face.Glyphs(s) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		b, err := engine.LayoutShapedGlyphs(face, glyphs, 6, 24, gg.RGBA{A: 1}, gg.Identity(), 1.0, false)
		if err != nil {
			t.Fatalf("LayoutShapedGlyphs %q: %v", s, err)
		}
		return b
	}

	// Fill page 0. Distinct glyphs at a large device scale are what consume an
	// atlas page; a few thousand of them spill onto the next.
	var glyphs []text.ShapedGlyph
	for r := rune(0x21); r < 0x2000; r++ {
		for g := range face.Glyphs(string(r)) {
			glyphs = append(glyphs, text.ShapedGlyph{GID: g.GID, X: g.X, Y: g.Y})
		}
		if engine.Atlas().PageCount() > 1 {
			break
		}
		if len(glyphs) > 0 {
			if _, err := engine.LayoutShapedGlyphs(face, glyphs, 6, 24,
				gg.RGBA{A: 1}, gg.Identity(), 8.0, false); err != nil {
				t.Fatalf("fill: %v", err)
			}
			glyphs = glyphs[:0]
		}
	}
	pages := engine.Atlas().PageCount()
	if pages < 2 {
		t.Skipf("atlas stayed on one page (%d); cannot exercise page spill here", pages)
	}
	t.Logf("atlas spilled to %d pages", pages)

	// Something must now report a page other than zero — either a batch whose
	// glyphs are all on a later page, or a spanning run split across pages.
	sawNonZero := false
	for r := rune(0x2000); r < 0x2200 && !sawNonZero; r++ {
		b := layout(string(r))
		if b.AtlasPageIndex != 0 {
			sawNonZero = true
		}
		for _, e := range b.Extra {
			if e.AtlasPageIndex != 0 {
				sawNonZero = true
			}
		}
	}
	if !sawNonZero {
		t.Error("every batch still reports atlas page 0 after the atlas spilled to " +
			"a second page; glyphs living on page 1 will be sampled from page 0 " +
			"and drawn as whatever else occupies those coordinates")
	}
}
