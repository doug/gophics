package app

import (
	"fmt"
	"sort"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// Frame recording, damage, replay, and the inspection/stat counters around
// them.

// SetDebugPaint toggles the box-bounds debug overlay at runtime.
func (c *core) SetDebugPaint(on bool) { c.debugPaint = on }

// SetInspect toggles the interactive widget inspector: while on, the box
// under the pointer is highlighted and labeled with its type and size (like
// Flutter's widget inspector). Pairs with InspectTree for the full dump.
func (c *core) SetInspect(on bool) {
	c.inspect = on
	c.Owner.RequestFrameThreadSafe()
}

// InspectTree returns the current render tree as a flat, depth-ordered dump
// (types, rects, semantics) — the data behind a widget inspector. Call
// after a frame. Runs headless.
func (c *core) InspectTree() []layoutbox.InspectNode {
	box := c.Owner.RootBox()
	if box == nil {
		return nil
	}
	return layoutbox.Inspect(box)
}

// FrameStats summarises recent frame times (ms) as percentiles.
//
// Percentiles rather than a mean, because a mean answers the wrong question.
// Stutter is a handful of frames far above the rest, and averaging them into
// the sixty good ones around them is exactly how they stop being visible in
// the number while staying visible on the screen. p50 says what the frame
// normally costs; p99 and the worst say what is being felt.
func (c *core) FrameStats() (s FrameSummary) {
	var buf [len(c.frameTimes)]float32
	n, worstAt := 0, -1
	for i, t := range c.frameTimes {
		if t <= 0 {
			continue
		}
		buf[n] = t
		n++
		if worstAt < 0 || t > c.frameTimes[worstAt] {
			worstAt = i
		}
	}
	if n == 0 {
		return s
	}
	sort.Slice(buf[:n], func(i, j int) bool { return buf[i] < buf[j] })
	at := func(f float32) float32 { return buf[int(f*float32(n-1))] }
	s.P50, s.P95, s.P99, s.Worst = at(0.50), at(0.95), at(0.99), buf[n-1]
	// The worst frame's own scene, not the window's: the question a spike
	// raises is what *that* frame was drawing.
	s.WorstOps, s.WorstBlurs = int(c.frameOps[worstAt]), int(c.frameBlurs[worstAt])
	s.WorstMade = c.frameMade[worstAt]
	// The median scene size, to compare the worst frame against.
	var ops [len(c.frameOps)]int32
	m := 0
	for i, t := range c.frameTimes {
		if t > 0 {
			ops[m] = c.frameOps[i]
			m++
		}
	}
	sort.Slice(ops[:m], func(i, j int) bool { return ops[i] < ops[j] })
	s.MedianOps = int(ops[m/2])
	return s
}

// FrameSummary is a window of frame times with what the worst frame drew.
//
// The ops counts are the point: a spike beside a median-sized scene is a
// discrete event — a layer resolved, an atlas grown, a glyph rasterized for
// the first time — where a spike beside a much larger scene is simply a
// heavier frame. Reporting the time alone cannot tell those apart, which is
// what made "occasional stutter" hard to act on.
type FrameSummary struct {
	P50, P95, P99, Worst float32
	WorstOps, WorstBlurs int
	MedianOps            int
	// WorstMade is what GPU resources the worst frame had to create, by kind.
	WorstMade MadeCounts
}

// MadeCounts is the GPU resources one frame created, by kind.
type MadeCounts struct {
	Textures   int
	Pipelines  int
	Buffers    int
	BindGroups int
}

// Total is every kind together.
func (m MadeCounts) Total() int { return m.Textures + m.Pipelines + m.Buffers + m.BindGroups }

// String renders the breakdown for a log line, naming only the kinds that are
// non-zero — a frame that made nothing but buffers should say so in as many
// words, not bury it in three zeroes.
func (m MadeCounts) String() string {
	if m.Total() == 0 {
		return "0 gpu objects"
	}
	parts := make([]string, 0, 4)
	for _, k := range []struct {
		n         int
		one, many string
	}{
		{m.Buffers, "buffer", "buffers"}, {m.BindGroups, "bind group", "bind groups"},
		{m.Textures, "texture", "textures"}, {m.Pipelines, "pipeline", "pipelines"},
	} {
		if k.n == 1 {
			parts = append(parts, "1 "+k.one)
		} else if k.n > 1 {
			parts = append(parts, fmt.Sprintf("%d %s", k.n, k.many))
		}
	}
	return strings.Join(parts, " + ")
}

func (c *core) recordFrameTime(ms float32) { c.recordFrame(ms, 0, 0) }

func (c *core) recordFrame(ms float32, ops, blurs int) {
	c.recordFrameMade(ms, ops, blurs, MadeCounts{})
}

func (c *core) recordFrameMade(ms float32, ops, blurs int, made MadeCounts) {
	c.frameTimes[c.frameHead] = ms
	c.frameOps[c.frameHead] = int32(ops)     //nolint:gosec // scene sizes are small
	c.frameBlurs[c.frameHead] = int32(blurs) //nolint:gosec
	c.frameMade[c.frameHead] = made
	c.frameHead = (c.frameHead + 1) % len(c.frameTimes)
}

// RecordScene records the current tree into a display list and diffs it
// against the previous frame's. It reports whether rasterization is needed
// and the (surface-clamped) damage rect. A size or scale change forces full
// damage, since the painter's retained surface is reallocated.
func (c *core) RecordScene(size geom.Size, scale float32) (changed bool, damage geom.Rect) {
	return c.recordScene(size, scale, false)
}

// RecordSceneGPU records and change-detects a frame the presenter will replay
// on the GPU. Change detection still runs (an unchanged scene lets the GPU
// present skip its full re-raster), but the damage rect a CPU present would
// need is not computed: text ops aren't measured for bounds (the expensive part
// of Diff), and the layered-scene full-damage rule doesn't apply — both exist
// only for CPU partial replay.
func (c *core) RecordSceneGPU(size geom.Size, scale float32) (changed bool) {
	changed, _ = c.recordScene(size, scale, true)
	return changed
}

// nullMeasurer satisfies scene.Measurer with zero extents — used when Diff runs
// only for its changed bool and the damage bounds are discarded (GPU present).
type nullMeasurer struct{}

func (nullMeasurer) MeasureWidthIn(string, string, float32) float32 { return 0 }
func (nullMeasurer) MetricsIn(string, float32) paint.TextMetrics    { return paint.TextMetrics{} }

// bg is the background for this frame: the dark variant when the platform
// reports a dark colour scheme and one was given, else the light one.
func (c *core) bg() paint.Color {
	if c.Owner.DarkMode && c.backgroundDark != (paint.Color{}) {
		return c.backgroundDark
	}
	return c.background
}

func (c *core) recordScene(size geom.Size, scale float32, gpu bool) (changed bool, damage geom.Rect) {
	c.cur.Reset()
	rec := c.cur.Recorder()
	surface := geom.RectFromSize(size)
	// Background as FillRect, not Clear: Clear ignores clips, which would
	// wipe retained pixels outside the damage region during partial replay.
	//
	// Opaque unless the app asked otherwise: the surface is retained across
	// frames, so a translucent background composites over the previous frame
	// and ghosts. Config.Transparent opts into translucency and turns retention
	// off to pay for it.
	bg := c.bg()
	if !c.transparent {
		bg.A = 1
	}
	rec.FillRect(surface, bg)
	if box := c.Owner.RootBox(); box != nil {
		box.Paint(rec, geom.Pt{})
		if c.debugPaint {
			layoutbox.DebugPaint(box, rec)
		}
		if c.inspect {
			layoutbox.InspectOverlay(box, c.lastPos, rec, c.Painter)
		}
	}

	var m scene.Measurer = c.Painter
	if gpu {
		m = nullMeasurer{} // damage bounds are discarded; skip text measurement
	}
	damage, changed = c.cur.Diff(c.prev, m)
	if debugNoDamage && changed {
		damage = surface // debug: force full repaint to isolate damage bugs
	}
	if c.transparent && changed {
		// Translucency and partial replay are incompatible: a blended
		// background over pixels kept from the previous frame ghosts it. A
		// changed frame is replayed whole.
		damage = surface
	}
	if size != c.lastPaintSize || scale != c.lastScale {
		changed, damage = true, surface
	}
	if !gpu && c.cur.HasLayers() {
		// Transform groups can't be partially replayed: their ops are recorded
		// in a transformed coordinate space, so their bounds can't feed the
		// surface-space damage rect — repaint the whole surface this frame.
		// (Opacity groups no longer set HasLayers: they record in surface
		// coordinates and Diff computes tight damage for them.) The GPU present
		// replays the whole scene anyway, so layers don't force it to treat an
		// unchanged frame as changed.
		changed, damage = true, surface
	}
	c.lastPaintSize, c.lastScale = size, scale
	damage = damage.Intersect(surface)
	if changed && damage.IsEmpty() {
		// Changed ops with degenerate bounds: repaint everything rather
		// than nothing.
		damage = surface
	}
	if gpu {
		// The GPU path re-rasters the full surface when anything changed;
		// report that honestly in the damage stats.
		if changed {
			damage = surface
		} else {
			damage = geom.Rect{}
		}
	}
	c.cur, c.prev = c.prev, c.cur // prev now holds the current scene
	c.LastDamage, c.Skipped = damage, !changed
	return changed, damage
}

// ReplayDamaged replays the current scene clipped to the damage rect,
// culling ops that don't intersect it. Pixels outside damage are untouched
// and remain valid from the previous frame (the painter's surface is
// retained across frames).
func (c *core) ReplayDamaged(canvas paint.Canvas, damage geom.Rect) {
	canvas.PushClip(damage)
	c.prev.ReplayDamage(canvas, damage, c.Painter)
	canvas.PopClip()
}

// ReplayScene replays the most recent recorded scene in full onto canvas. The
// GPU present path rasterizes the whole frame on the GPU each frame, so it
// uses this rather than damage-culled partial replay. Call after RecordScene.
func (c *core) ReplayScene(canvas paint.Canvas) {
	c.prev.Replay(canvas)
}
