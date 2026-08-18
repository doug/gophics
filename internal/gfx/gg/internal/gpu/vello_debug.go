//go:build !nogpu

// Stage-level comparison between the compute pipeline and its CPU port.
//
// The compute pipeline has a CPU implementation of every stage in
// tilecompute, sharing the exact same structs — Tile, SegmentCount,
// PathSegment, BumpAllocators — because the Go stages are direct ports of the
// shaders. Until now that correspondence was only ever used to compare
// finished images, which is the weakest possible use of it: a wrong picture
// tells you the pipeline is broken and nothing more.
//
// It cost most of a day to find that path_tiling filled 8 of the 16 segment
// slots coarse had reserved. Every structural check — buffer sizes, dispatch
// shapes, strides, struct layouts — passed, because none of them was wrong.
// What was needed was the observation that two stages disagreed, and that is
// exactly what diffing these buffers produces automatically.
//
// The intermediates survive to the end of the frame, so this needs no stepwise
// dispatch: coarse rewrites Tile.SegmentCountOrIx from a count into an
// inverted index, and the CPU port does the same in the same order, so the
// final states are directly comparable.

package gpu

import (
	"encoding/binary"
	"fmt"
	"math"

	"github.com/doug/gophics/internal/gfx/gg/internal/gpu/tilecompute"
)

// ComputeStageSnapshot is the GPU pipeline's intermediate state, in the same
// types the CPU port uses.
type ComputeStageSnapshot struct {
	WidthInTiles  uint32
	HeightInTiles uint32

	Bump      tilecompute.BumpAllocators
	Tiles     []tilecompute.Tile
	SegCounts []tilecompute.SegmentCount
	Segments  []tilecompute.PathSegment
}

// captureStages reads the intermediate buffers back into a snapshot. Called
// from dispatchComputeScene only when a capture has been requested, so a normal
// frame pays nothing: each readback is a submit, a wait for idle and a map.
func (a *VelloAccelerator) captureStages(bufs *VelloComputeBuffers, config VelloComputeConfig, totalPathTiles uint32) *ComputeStageSnapshot {
	le := binary.LittleEndian
	snap := &ComputeStageSnapshot{
		WidthInTiles:  config.WidthInTiles,
		HeightInTiles: config.HeightInTiles,
	}

	if b, err := a.readbackBuffer(bufs.BumpAlloc, 16); err == nil {
		snap.Bump = tilecompute.BumpAllocators{
			Lines:     config.NumLines,
			SegCounts: le.Uint32(b[0:4]),
			Segments:  le.Uint32(b[4:8]),
		}
	}

	if totalPathTiles > 0 {
		if b, err := a.readbackBuffer(bufs.Tiles, uint64(totalPathTiles)*8); err == nil {
			snap.Tiles = make([]tilecompute.Tile, totalPathTiles)
			for i := range snap.Tiles {
				snap.Tiles[i] = tilecompute.Tile{
					Backdrop:         int32(le.Uint32(b[i*8 : i*8+4])),
					SegmentCountOrIx: le.Uint32(b[i*8+4 : i*8+8]),
				}
			}
		}
	}

	if n := snap.Bump.SegCounts; n > 0 {
		if b, err := a.readbackBuffer(bufs.SegCounts, uint64(n)*8); err == nil {
			snap.SegCounts = make([]tilecompute.SegmentCount, n)
			for i := range snap.SegCounts {
				snap.SegCounts[i] = tilecompute.SegmentCount{
					LineIx: le.Uint32(b[i*8 : i*8+4]),
					Counts: le.Uint32(b[i*8+4 : i*8+8]),
				}
			}
		}
	}

	if n := snap.Bump.Segments; n > 0 {
		if b, err := a.readbackBuffer(bufs.Segments, uint64(n)*velloPathSegmentSize); err == nil {
			snap.Segments = make([]tilecompute.PathSegment, n)
			for i := range snap.Segments {
				o := i * velloPathSegmentSize
				f := func(k int) float32 { return math.Float32frombits(le.Uint32(b[o+k*4 : o+k*4+4])) }
				snap.Segments[i] = tilecompute.PathSegment{
					Point0: [2]float32{f(0), f(1)},
					Point1: [2]float32{f(2), f(3)},
					YEdge:  f(4),
				}
			}
		}
	}

	return snap
}

// ComputeUnavailableReason returns why CanCompute reports false, or nil when
// compute is available or was never initialised.
//
// Init deliberately degrades: a pipeline that fails to build leaves the
// renderer working without it. What it must not do is discard the reason.
// "CanCompute() is false" looks like a statement about the machine, and for a
// long time it was actually three bugs in shader translation — findable only
// by re-running init by hand and reading a log nobody had enabled.
func (a *VelloAccelerator) ComputeUnavailableReason() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.dispatcher != nil && a.dispatcher.initialized {
		return nil
	}
	return a.computeErr
}

// DebugComputeStages renders a scene and returns the pipeline's intermediate
// buffers alongside the image, for diffing against tilecompute's CPU port with
// DiffComputeStages.
//
// Expensive by construction — several GPU stalls — so it is a debugging entry
// point, not something a renderer calls.
func (a *VelloAccelerator) DebugComputeStages(
	width, height int,
	bgColor [4]uint8,
	paths []tilecompute.PathDef,
) (*ComputeStageSnapshot, error) {
	a.mu.Lock()
	if !a.gpuReady || a.dispatcher == nil {
		a.mu.Unlock()
		return nil, fmt.Errorf("vello-compute: GPU not ready")
	}
	a.wantStageCapture = true
	a.stageCapture = nil
	defer func() { a.wantStageCapture = false }()

	_, err := a.dispatchComputeScene(width, height, bgColor, paths)
	snap := a.stageCapture
	a.mu.Unlock()

	if err != nil {
		return nil, err
	}
	if snap == nil {
		return nil, fmt.Errorf("vello-compute: stage capture produced nothing")
	}
	return snap, nil
}

// StageDiff names one disagreement between the two pipelines.
type StageDiff struct {
	Stage string // the stage whose output disagrees
	Field string // which buffer
	Index int    // first differing element, or -1 for a whole-buffer property
	GPU   string
	CPU   string
}

func (d StageDiff) String() string {
	if d.Index < 0 {
		return fmt.Sprintf("%s: %s differs — GPU %s, CPU %s", d.Stage, d.Field, d.GPU, d.CPU)
	}
	return fmt.Sprintf("%s: %s[%d] differs — GPU %s, CPU %s", d.Stage, d.Field, d.Index, d.GPU, d.CPU)
}

// DiffComputeStages compares a GPU snapshot against the CPU port's capture and
// returns the disagreements in pipeline order, so the first entry names the
// earliest stage that went wrong.
//
// It compares only order-invariant properties, which is the whole subtlety of
// diffing a parallel pipeline against a sequential one. Several values here are
// produced by atomic counters — the per-tile segment order in path_count, the
// slot bases handed out by coarse — and their actual values depend on which
// invocation reached the atomic first. The GPU races; the CPU walks lines in a
// loop. Comparing them elementwise reports a divergence on every scene while
// the rendered images agree pixel for pixel, which is worse than no tool at
// all: it trains you to ignore it.
//
// So the questions asked are the ones that survive reordering. Do the two agree
// on how much work there is? On each tile's backdrop, which is what fills a
// tile solid? On the set of segments produced, regardless of where they landed?
// And is every slot that was reserved actually written — the invariant that
// located the M11 bug.
func DiffComputeStages(gpu *ComputeStageSnapshot, cpu *tilecompute.StageCapture) []StageDiff {
	var diffs []StageDiff

	// path_count: total crossings found. Order-independent.
	if gpu.Bump.SegCounts != cpu.Bump.SegCounts {
		diffs = append(diffs, StageDiff{"path_count", "total crossings", -1,
			fmt.Sprint(gpu.Bump.SegCounts), fmt.Sprint(cpu.Bump.SegCounts)})
	}

	// backdrop drives whether a tile fills solid, and is a sum — independent
	// of the order the contributions arrived in. This is the field that was
	// wrong when the compute path bled fills to the tile edge.
	if d, ok := diffBackdrops(gpu.Tiles, cpu.Tiles); ok {
		diffs = append(diffs, d)
	}

	// coarse: total slots reserved, and how many tiles carry segments.
	if gpu.Bump.Segments != cpu.Bump.Segments {
		diffs = append(diffs, StageDiff{"coarse", "segment slots reserved", -1,
			fmt.Sprint(gpu.Bump.Segments), fmt.Sprint(cpu.Bump.Segments)})
	}
	if g, c := tilesWithSegments(gpu.Tiles), tilesWithSegments(cpu.Tiles); g != c {
		diffs = append(diffs, StageDiff{"coarse", "tiles carrying segments", -1,
			fmt.Sprint(g), fmt.Sprint(c)})
	}

	// path_tiling: every reserved slot must be written. This is the invariant
	// that found the M11 bug — coarse reserved 16 and path_tiling wrote 8 —
	// and it holds regardless of which slot each segment landed in.
	if unwritten := countUnwritten(gpu.Segments); unwritten > 0 {
		diffs = append(diffs, StageDiff{"path_tiling", "unwritten reserved slots", -1,
			fmt.Sprintf("%d of %d left zeroed", unwritten, len(gpu.Segments)), "0"})
	}

	// path_tiling: the geometry produced, as a set.
	if d, ok := diffSegmentSets(gpu.Segments, cpu.Segments); ok {
		diffs = append(diffs, d)
	}

	return diffs
}

// diffBackdrops compares per-tile backdrop, which is position-indexed and so
// directly comparable.
func diffBackdrops(gpu, cpu []tilecompute.Tile) (StageDiff, bool) {
	if len(gpu) != len(cpu) {
		return StageDiff{"path_count", "tile count", -1, fmt.Sprint(len(gpu)), fmt.Sprint(len(cpu))}, true
	}
	for i := range gpu {
		if gpu[i].Backdrop != cpu[i].Backdrop {
			return StageDiff{"path_count", "tile backdrop", i,
				fmt.Sprint(gpu[i].Backdrop), fmt.Sprint(cpu[i].Backdrop)}, true
		}
	}
	return StageDiff{}, false
}

// tilesWithSegments counts tiles coarse gave a segment run to. After coarse the
// field holds ~base, so a tile with segments is one whose value is not zero.
func tilesWithSegments(tiles []tilecompute.Tile) int {
	n := 0
	for _, t := range tiles {
		if t.SegmentCountOrIx != 0 {
			n++
		}
	}
	return n
}

// countUnwritten reports slots left entirely zero. A real segment always has a
// non-zero field: yEdge is either a coordinate or the 1e9 sentinel, never 0.
func countUnwritten(segs []tilecompute.PathSegment) int {
	n := 0
	for _, s := range segs {
		if s.Point0 == [2]float32{} && s.Point1 == [2]float32{} && s.YEdge == 0 {
			n++
		}
	}
	return n
}

// diffSegmentSets compares the segments as a multiset, by greedy matching
// rather than by sorting and pairing off.
//
// Sorting first is the obvious approach and it is wrong here: the comparison is
// tolerant, so a single genuinely-different element shifts every later one by a
// position and the diff cascades — 5 of 52 segments "differ" when 2 do. Greedy
// matching reports only the elements that have no partner, which is the honest
// count. O(n²) is irrelevant at debugging scale.
func diffSegmentSets(gpu, cpu []tilecompute.PathSegment) (StageDiff, bool) {
	if len(gpu) != len(cpu) {
		return StageDiff{"path_tiling", "segment count", -1, fmt.Sprint(len(gpu)), fmt.Sprint(len(cpu))}, true
	}

	used := make([]bool, len(gpu))
	unmatched := 0
	firstIdx := -1
	for i, c := range cpu {
		found := false
		for j, g := range gpu {
			if !used[j] && segNear(g, c) {
				used[j], found = true, true
				break
			}
		}
		if !found {
			unmatched++
			if firstIdx < 0 {
				firstIdx = i
			}
		}
	}
	if unmatched == 0 {
		return StageDiff{}, false
	}

	c := cpu[firstIdx]
	return StageDiff{"path_tiling", fmt.Sprintf("segment geometry (%d of %d unmatched)", unmatched, len(cpu)), firstIdx,
		"no segment within tolerance",
		fmt.Sprintf("%v->%v yEdge %g", c.Point0, c.Point1, c.YEdge)}, true
}

// segEpsilon bounds float disagreement between the shader and the Go port.
// They do the same arithmetic but not necessarily with the same rounding.
const segEpsilon = 1e-4

func segNear(a, b tilecompute.PathSegment) bool {
	return near(a.Point0[0], b.Point0[0]) && near(a.Point0[1], b.Point0[1]) &&
		near(a.Point1[0], b.Point1[0]) && near(a.Point1[1], b.Point1[1]) &&
		near(a.YEdge, b.YEdge)
}

func near(a, b float32) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return float64(d) <= segEpsilon
}
