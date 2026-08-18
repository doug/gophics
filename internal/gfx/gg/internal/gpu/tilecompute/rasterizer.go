// Copyright 2026 The gogpu Authors
// SPDX-License-Identifier: MIT

// Rasterizer ties together all Vello CPU pipeline stages into a single API.
// This is NOT part of the Vello source — it's our integration layer.

package tilecompute

import (
	"image"
	"image/color"
	"math"
)

// Rasterizer runs the complete Vello CPU rasterization pipeline.
type Rasterizer struct {
	width, height int

	// capture, when non-nil, receives each stage's intermediate state.
	// Set only through RasterizeCapturing; nil on every normal render, so
	// the copies below cost nothing in the hot path.
	capture *StageCapture
}

// NewRasterizer creates a new rasterizer for the given canvas size.
func NewRasterizer(width, height int) *Rasterizer {
	return &Rasterizer{width: width, height: height}
}

// Rasterize runs the full pipeline and returns per-pixel alpha values [0.0, 1.0].
// The result is a flat array of width*height float32 values in row-major order.
// StageCapture holds the pipeline's intermediate state, so a GPU run can be
// diffed against this one stage by stage rather than only at the final image.
//
// Every field mirrors a GPU buffer exactly — same struct, same order — because
// the CPU stages are direct ports of the shaders. That correspondence is the
// most useful debugging asset this package has, and until now it was only ever
// used to compare finished pictures. A wrong picture says the pipeline is
// broken; a diff of these says which stage broke it and at which element.
type StageCapture struct {
	Path      Path
	Bump      BumpAllocators
	Tiles     []Tile
	SegCounts []SegmentCount
	Segments  []PathSegment

	// TilesAfterPathCount is Tiles before coarse rewrites SegmentCountOrIx
	// from a count into an inverted index. Both are worth comparing: a
	// divergence in the counts is a path_count bug, a divergence in the
	// indices with matching counts is a coarse bug.
	TilesAfterPathCount []Tile
}

// Rasterize renders lines to a coverage buffer. If capture is non-nil it is
// filled with the intermediate state of each stage; pass nil in normal use.
func (r *Rasterizer) RasterizeCapturing(lines []LineSoup, fillRule FillRule, capture *StageCapture) []float32 {
	r.capture = capture
	defer func() { r.capture = nil }()
	return r.Rasterize(lines, fillRule)
}

func (r *Rasterizer) Rasterize(lines []LineSoup, fillRule FillRule) []float32 {
	if len(lines) == 0 {
		return make([]float32, r.width*r.height)
	}

	// Step 0: Compute bounding box in tile coordinates
	path, tilesX, tilesY := r.computePath(lines)

	tileCount := tilesX * tilesY
	if tileCount == 0 {
		return make([]float32, r.width*r.height)
	}

	// Allocate tiles
	tiles := make([]Tile, tileCount)

	// Step 1: pathCount — DDA walk, backdrop, segment counting
	// We need to estimate max segCounts size.
	// Each line produces at most span_x + span_y segments.
	maxSegCounts := uint32(0)
	for _, line := range lines {
		p0 := vec2FromArray(line.P0)
		p1 := vec2FromArray(line.P1)
		var xy0, xy1 vec2
		if p1.y >= p0.y {
			xy0, xy1 = p0, p1
		} else {
			xy0, xy1 = p1, p0
		}
		s0 := xy0.mul(tileScale)
		s1 := xy1.mul(tileScale)
		countX := span(s0.x, s1.x) - 1
		count := countX + span(s0.y, s1.y)
		maxSegCounts += count
	}

	segCounts := make([]SegmentCount, maxSegCounts)
	bump := &BumpAllocators{
		Lines: uint32(len(lines)),
	}
	paths := []Path{path}

	pathCountMain(bump, lines, paths, tiles, segCounts)

	if r.capture != nil {
		r.capture.Path = path
		r.capture.TilesAfterPathCount = append([]Tile(nil), tiles...)
	}

	// Step 2: Coarse allocation — convert counts to indices
	// (Port of coarse.rs segment allocation, lines 79-83)
	nextSegIx := uint32(0)
	for i := range tiles {
		nSegs := tiles[i].SegmentCountOrIx
		if nSegs != 0 {
			tiles[i].SegmentCountOrIx = ^nextSegIx // !seg_ix in Rust
			nextSegIx += nSegs
		}
	}
	totalSegments := nextSegIx

	// Step 3: pathTiling — segment clipping + yEdge
	segments := make([]PathSegment, totalSegments)
	pathTilingMain(bump, segCounts, lines, paths, tiles, segments)

	if r.capture != nil {
		r.capture.Bump = *bump
		// The CPU port keeps its segment total in a local rather than in the
		// bump allocator — the GPU has to use an atomic there, this does not.
		// Recorded explicitly so the two are comparable.
		r.capture.Bump.Segments = totalSegments
		r.capture.Tiles = append([]Tile(nil), tiles...)
		r.capture.SegCounts = append([]SegmentCount(nil), segCounts[:bump.SegCounts]...)
		r.capture.Segments = append([]PathSegment(nil), segments...)
	}

	// Step 4: Backdrop prefix sum
	// (Port of backdrop.rs)
	bboxW := int(path.BBox[2] - path.BBox[0])
	bboxH := int(path.BBox[3] - path.BBox[1])
	base := int(path.Tiles)
	for y := 0; y < bboxH; y++ {
		sum := int32(0)
		for x := 0; x < bboxW; x++ {
			idx := base + y*bboxW + x
			sum += tiles[idx].Backdrop
			tiles[idx].Backdrop = sum
		}
	}

	if r.capture != nil {
		// Captured here, after the prefix sum, because the GPU runs its
		// backdrop stage between path_count and coarse while this port runs it
		// last. The two orders are equivalent — the prefix sum touches only
		// Backdrop, and the stages between touch only SegmentCountOrIx — but a
		// snapshot taken before it would be comparing the same field at two
		// different points in the pipeline, which reads as a divergence on
		// every scene.
		r.capture.Tiles = append([]Tile(nil), tiles...)
	}

	// Step 5: Fine rasterization — per tile fill_path
	result := make([]float32, r.width*r.height)
	area := make([]float32, TileWidth*TileHeight)

	bboxX0 := int(path.BBox[0])
	bboxY0 := int(path.BBox[1])

	for ty := 0; ty < tilesY; ty++ {
		for tx := 0; tx < tilesX; tx++ {
			tileIdx := base + ty*bboxW + tx
			tile := tiles[tileIdx]

			// Gather segments for this tile
			var tileSegments []PathSegment
			segStart := ^tile.SegmentCountOrIx
			if int32(segStart) >= 0 {
				// Count how many segments this tile has by looking at
				// the next tile's segStart (or totalSegments)
				segEnd := totalSegments
				// Find next tile that has segments
				for nextIdx := tileIdx + 1; nextIdx < len(tiles); nextIdx++ {
					nextStart := ^tiles[nextIdx].SegmentCountOrIx
					if int32(nextStart) >= 0 {
						segEnd = nextStart
						break
					}
				}
				if segStart < segEnd {
					tileSegments = segments[segStart:segEnd]
				}
			}

			// Clear area
			for i := range area {
				area[i] = 0
			}

			// Fill
			fillPath(area, tileSegments, tile.Backdrop, fillRule == FillRuleEvenOdd)

			// Write to result
			globalTileX := (bboxX0 + tx) * TileWidth
			globalTileY := (bboxY0 + ty) * TileHeight
			for ly := 0; ly < TileHeight; ly++ {
				py := globalTileY + ly
				if py >= r.height {
					break
				}
				for lx := 0; lx < TileWidth; lx++ {
					px := globalTileX + lx
					if px >= r.width {
						break
					}
					result[py*r.width+px] = area[ly*TileWidth+lx]
				}
			}
		}
	}

	return result
}

// RasterizeScene renders multiple paths with compositing onto a single image.
// Paths are composited in order (painter's algorithm, back-to-front) using
// premultiplied source-over blending. Each path is independently rasterized
// through the full Vello pipeline, then composited onto the output.
// Returns the final composited RGBA image.
func (r *Rasterizer) RasterizeScene(bgColor [4]uint8, paths []PathDef) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))

	// Fill background
	bg := color.RGBA{R: bgColor[0], G: bgColor[1], B: bgColor[2], A: bgColor[3]}
	for y := 0; y < r.height; y++ {
		for x := 0; x < r.width; x++ {
			img.SetRGBA(x, y, bg)
		}
	}

	// Composite each path in order (painter's algorithm)
	for _, pd := range paths {
		if len(pd.Lines) == 0 {
			continue
		}

		// Run the existing 5-stage pipeline to get alpha values
		alphas := r.Rasterize(pd.Lines, pd.FillRule)

		// Composite path over current image
		for y := 0; y < r.height; y++ {
			for x := 0; x < r.width; x++ {
				alpha := alphas[y*r.width+x]
				if alpha <= 0 {
					continue
				}

				cur := img.RGBAAt(x, y)
				dst := [4]uint8{cur.R, cur.G, cur.B, cur.A}
				blended := blendSourceOver(pd.Color, alpha, dst)
				img.SetRGBA(x, y, color.RGBA{
					R: blended[0],
					G: blended[1],
					B: blended[2],
					A: blended[3],
				})
			}
		}
	}

	return img
}

// RasterizeScenePTCL renders multiple paths using the full Vello compute pipeline:
// scene encoding -> pathtag reduce/scan -> draw reduce/scan -> flatten -> coarse -> fine PTCL.
// This matches Vello's actual GPU pipeline architecture where all paths are processed
// together through shared tile command lists, enabling correct multi-path compositing
// in a single fine rasterization pass.
//
// The result should be pixel-identical (within rounding tolerance) to RasterizeScene,
// which processes paths individually and composites in a separate step.
func (r *Rasterizer) RasterizeScenePTCL(bgColor [4]uint8, paths []PathDef) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))

	// Convert bgColor (straight uint8) to premultiplied float32.
	bgA := float32(bgColor[3]) / 255.0
	bgFloat := [4]float32{
		float32(bgColor[0]) / 255.0 * bgA,
		float32(bgColor[1]) / 255.0 * bgA,
		float32(bgColor[2]) / 255.0 * bgA,
		bgA,
	}

	if len(paths) == 0 {
		// Fill with background and return.
		bg := color.RGBA{R: bgColor[0], G: bgColor[1], B: bgColor[2], A: bgColor[3]}
		for y := 0; y < r.height; y++ {
			for x := 0; x < r.width; x++ {
				img.SetRGBA(x, y, bg)
			}
		}
		return img
	}

	// Step 1: Encode scene.
	enc := EncodeScene(paths)

	// Step 2: Pack scene.
	scene := PackScene(enc)

	// Step 3: Path tag reduce/scan.
	reduced := pathtagReduce(scene)
	_ = pathtagScan(scene, reduced) // Scan result not used in CPU path yet, but called for correctness.

	// Step 4: Draw reduce/scan.
	drawReduced := drawReduce(scene)
	drawMonoids, info, clipInps := drawLeafScan(scene, drawReduced)

	// Step 4b: Clip leaf — fix up EndClip draw monoids.
	if enc.NumClips > 0 && len(clipInps) > 0 {
		clipLeafScan(clipInps, drawMonoids)
	}

	// Step 5: Build allLines with correct PathIx for each path.
	var allLines []LineSoup
	for pathIx, pd := range paths {
		for _, line := range pd.Lines {
			allLines = append(allLines, LineSoup{
				PathIx: uint32(pathIx),
				P0:     line.P0,
				P1:     line.P1,
			})
		}
	}

	// Step 6: Coarse rasterize.
	coarseOut := CoarseRasterize(scene, drawMonoids, info, allLines, r.width, r.height)

	// Step 7: Fine rasterize each tile.
	widthInTiles := coarseOut.WidthInTiles
	heightInTiles := coarseOut.HeightInTiles

	for ty := 0; ty < heightInTiles; ty++ {
		for tx := 0; tx < widthInTiles; tx++ {
			tileIdx := ty*widthInTiles + tx
			tilePixels := fineRasterizeTile(coarseOut.TilePTCLs[tileIdx], coarseOut.Segments, bgFloat)

			// Write tile pixels to output image.
			globalTileX := tx * TileWidth
			globalTileY := ty * TileHeight
			for ly := 0; ly < TileHeight; ly++ {
				py := globalTileY + ly
				if py >= r.height {
					break
				}
				for lx := 0; lx < TileWidth; lx++ {
					px := globalTileX + lx
					if px >= r.width {
						break
					}
					pm := tilePixels[ly*TileWidth+lx]
					straight := premulToStraightU8(pm)
					img.SetRGBA(px, py, color.RGBA{
						R: straight[0],
						G: straight[1],
						B: straight[2],
						A: straight[3],
					})
				}
			}
		}
	}

	return img
}

// RasterizeSceneDefPTCL renders a scene defined by SceneElements (with clip support)
// using the full Vello compute pipeline. This is the clip-aware version of RasterizeScenePTCL.
//
// SceneElements can include Draw, BeginClip, and EndClip operations. BeginClip/EndClip
// pairs define clip regions: only content drawn between them that falls inside the
// clip path is visible.
func (r *Rasterizer) RasterizeSceneDefPTCL(bgColor [4]uint8, elements []SceneElement) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, r.width, r.height))

	// Convert bgColor (straight uint8) to premultiplied float32.
	bgA := float32(bgColor[3]) / 255.0
	bgFloat := [4]float32{
		float32(bgColor[0]) / 255.0 * bgA,
		float32(bgColor[1]) / 255.0 * bgA,
		float32(bgColor[2]) / 255.0 * bgA,
		bgA,
	}

	if len(elements) == 0 {
		bg := color.RGBA{R: bgColor[0], G: bgColor[1], B: bgColor[2], A: bgColor[3]}
		for y := 0; y < r.height; y++ {
			for x := 0; x < r.width; x++ {
				img.SetRGBA(x, y, bg)
			}
		}
		return img
	}

	// Step 1: Encode scene with clip support.
	enc := EncodeSceneDef(elements)

	// Step 2: Pack scene.
	scene := PackScene(enc)

	// Step 3: Path tag reduce/scan.
	reduced := pathtagReduce(scene)
	_ = pathtagScan(scene, reduced)

	// Step 4: Draw reduce/scan.
	drawReduced := drawReduce(scene)
	drawMonoids, info, clipInps := drawLeafScan(scene, drawReduced)

	// Step 4b: Clip leaf — fix up EndClip draw monoids.
	if enc.NumClips > 0 && len(clipInps) > 0 {
		clipLeafScan(clipInps, drawMonoids)
	}

	// Step 5: Build allLines with correct PathIx for each path.
	// SceneElements may have different path indices than their element order;
	// path_ix is assigned by the encoder (one per Draw/BeginClip, also for EndClip dummy).
	var allLines []LineSoup
	pathIdx := uint32(0)
	for _, el := range elements {
		switch el.Type {
		case ElementDraw, ElementBeginClip:
			for _, line := range el.Lines {
				allLines = append(allLines, LineSoup{
					PathIx: pathIdx,
					P0:     line.P0,
					P1:     line.P1,
				})
			}
			pathIdx++
		case ElementEndClip:
			// EndClip has a dummy path (no lines), but path index is still incremented.
			pathIdx++
		}
	}

	// Step 6: Coarse rasterize.
	coarseOut := CoarseRasterize(scene, drawMonoids, info, allLines, r.width, r.height)

	// Step 7: Fine rasterize each tile.
	widthInTiles := coarseOut.WidthInTiles
	heightInTiles := coarseOut.HeightInTiles

	for ty := 0; ty < heightInTiles; ty++ {
		for tx := 0; tx < widthInTiles; tx++ {
			tileIdx := ty*widthInTiles + tx
			tilePixels := fineRasterizeTile(coarseOut.TilePTCLs[tileIdx], coarseOut.Segments, bgFloat)

			globalTileX := tx * TileWidth
			globalTileY := ty * TileHeight
			for ly := 0; ly < TileHeight; ly++ {
				py := globalTileY + ly
				if py >= r.height {
					break
				}
				for lx := 0; lx < TileWidth; lx++ {
					px := globalTileX + lx
					if px >= r.width {
						break
					}
					pm := tilePixels[ly*TileWidth+lx]
					straight := premulToStraightU8(pm)
					img.SetRGBA(px, py, color.RGBA{
						R: straight[0],
						G: straight[1],
						B: straight[2],
						A: straight[3],
					})
				}
			}
		}
	}

	return img
}

// computePath computes the Path struct and tile grid dimensions from lines.
func (r *Rasterizer) computePath(lines []LineSoup) (Path, int, int) {
	// Compute pixel bounding box from all lines
	minX := float32(math.MaxFloat32)
	minY := float32(math.MaxFloat32)
	maxX := float32(-math.MaxFloat32)
	maxY := float32(-math.MaxFloat32)

	for _, line := range lines {
		for _, p := range [][2]float32{line.P0, line.P1} {
			if p[0] < minX {
				minX = p[0]
			}
			if p[0] > maxX {
				maxX = p[0]
			}
			if p[1] < minY {
				minY = p[1]
			}
			if p[1] > maxY {
				maxY = p[1]
			}
		}
	}

	// Clamp to canvas
	if minX < 0 {
		minX = 0
	}
	if minY < 0 {
		minY = 0
	}
	if maxX > float32(r.width) {
		maxX = float32(r.width)
	}
	if maxY > float32(r.height) {
		maxY = float32(r.height)
	}

	// Convert to tile coordinates
	bboxX0 := uint32(math.Floor(float64(minX / float32(TileWidth))))
	bboxY0 := uint32(math.Floor(float64(minY / float32(TileHeight))))
	bboxX1 := uint32(math.Ceil(float64(maxX / float32(TileWidth))))
	bboxY1 := uint32(math.Ceil(float64(maxY / float32(TileHeight))))

	// Clamp to grid limits
	gridTilesX := uint32(math.Ceil(float64(r.width) / float64(TileWidth)))
	gridTilesY := uint32(math.Ceil(float64(r.height) / float64(TileHeight)))
	if bboxX1 > gridTilesX {
		bboxX1 = gridTilesX
	}
	if bboxY1 > gridTilesY {
		bboxY1 = gridTilesY
	}

	tilesX := int(bboxX1 - bboxX0)
	tilesY := int(bboxY1 - bboxY0)

	path := Path{
		BBox:  [4]uint32{bboxX0, bboxY0, bboxX1, bboxY1},
		Tiles: 0,
	}

	return path, tilesX, tilesY
}

// LineSoupFromVelloLine converts a pre-sorted VelloLine (P0.Y <= P1.Y, IsDown flag)
// back to original LineSoup direction (unsorted, as Vello's flattener would emit).
func LineSoupFromVelloLine(p0, p1 [2]float32, isDown bool) LineSoup {
	if isDown {
		// Original direction was downward (p0.y < p1.y) — already in correct order
		return LineSoup{PathIx: 0, P0: p0, P1: p1}
	}
	// Original direction was upward (p0 was below p1) — swap back
	return LineSoup{PathIx: 0, P0: p1, P1: p0}
}
