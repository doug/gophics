//go:build !nogpu

package gpu

import (
	"sync"

	"github.com/doug/gophics/internal/gfx/gg/scene"
)

// TileCoverageRasterizer turns a path into per-pixel coverage over 4×4 tiles:
// flatten Beziers to monotonic line segments, bin each segment into the tiles
// it crosses, then compute analytic anti-aliased coverage per pixel. That is
// what RasterizePath does and the only path TileCoverageFiller takes.
//
// The strips field below, and RasterizeToStrips and RenderStripsToBuffer
// beside it, are a second and unreachable pipeline: Vello-style sparse
// strips, which nothing outside this file's own tests calls. They are what
// the type's former name, SparseStripsRasterizer, actually described — the
// rename left them here rather than deleting code and renaming it in one
// step. Deleting them is the next item in this phase.
type TileCoverageRasterizer struct {
	// Pipeline components
	flattenCtx *FlattenContext
	coarse     *CoarseRasterizer
	fine       *FineRasterizer
	strips     *StripRenderer

	// Configuration
	width    uint16
	height   uint16
	fillRule scene.FillStyle

	// Computed data
	backdrop []int32

	// Pool for reuse
	pool *TileCoveragePool
}

// TileCoverageConfig configures a TileCoverageRasterizer.
type TileCoverageConfig struct {
	Width     uint16
	Height    uint16
	FillRule  scene.FillStyle
	Tolerance float32 // Flattening tolerance (0 uses default)
}

// DefaultConfig returns a default configuration.
func DefaultConfig(width, height uint16) TileCoverageConfig {
	return TileCoverageConfig{
		Width:     width,
		Height:    height,
		FillRule:  scene.FillNonZero,
		Tolerance: FlattenTolerance,
	}
}

// NewTileCoverageRasterizer returns a rasterizer sized by config.
func NewTileCoverageRasterizer(config TileCoverageConfig) *TileCoverageRasterizer {
	return &TileCoverageRasterizer{
		flattenCtx: NewFlattenContext(),
		coarse:     NewCoarseRasterizer(config.Width, config.Height),
		fine:       NewFineRasterizer(config.Width, config.Height),
		strips:     NewStripRenderer(),
		width:      config.Width,
		height:     config.Height,
		fillRule:   config.FillRule,
		pool:       globalTileCoveragePool,
	}
}

// Reset clears the rasterizer state for reuse with new geometry.
func (tcr *TileCoverageRasterizer) Reset() {
	tcr.flattenCtx.Reset()
	tcr.coarse.Reset()
	tcr.fine.Reset()
	tcr.strips.Reset()
	tcr.backdrop = nil
}

// SetFillRule sets the fill rule for rendering.
func (tcr *TileCoverageRasterizer) SetFillRule(rule scene.FillStyle) {
	tcr.fillRule = rule
	tcr.fine.SetFillRule(rule)
	tcr.strips.SetFillRule(rule)
}

// raster.FillRule returns the current fill rule.
func (tcr *TileCoverageRasterizer) FillRule() scene.FillStyle {
	return tcr.fillRule
}

// SetSize changes the rasterizer viewport dimensions.
func (tcr *TileCoverageRasterizer) SetSize(width, height uint16) {
	if tcr.width != width || tcr.height != height {
		tcr.width = width
		tcr.height = height
		tcr.coarse = NewCoarseRasterizer(width, height)
		tcr.fine = NewFineRasterizer(width, height)
	}
}

// Width returns the viewport width.
func (tcr *TileCoverageRasterizer) Width() uint16 {
	return tcr.width
}

// Height returns the viewport height.
func (tcr *TileCoverageRasterizer) Height() uint16 {
	return tcr.height
}

// RasterizePath rasterizes a single path to the tile grid.
func (tcr *TileCoverageRasterizer) RasterizePath(
	path *scene.Path,
	transform scene.Affine,
	tolerance float32,
) {
	if path == nil || path.IsEmpty() {
		return
	}

	if tolerance <= 0 {
		tolerance = FlattenTolerance
	}

	// Phase 1: Flatten path to monotonic line segments
	tcr.flattenCtx.Reset()
	tcr.flattenCtx.FlattenPathTo(path, transform, tolerance)
	segments := tcr.flattenCtx.Segments()

	if segments.Len() == 0 {
		return
	}

	// Phase 2: Coarse rasterization - bin segments into tiles
	tcr.coarse.Rasterize(segments)
	tcr.coarse.SortEntries()

	// Phase 3: Calculate backdrop winding
	tcr.backdrop = tcr.coarse.CalculateBackdrop()

	// Phase 4: Fine rasterization - analytic anti-aliasing
	tcr.fine.SetFillRule(tcr.fillRule)
	tcr.fine.Rasterize(tcr.coarse, segments, tcr.backdrop)
}

// RasterizeToStrips rasterizes a path and generates sparse strips.
func (tcr *TileCoverageRasterizer) RasterizeToStrips(
	path *scene.Path,
	transform scene.Affine,
	tolerance float32,
) {
	if path == nil || path.IsEmpty() {
		return
	}

	if tolerance <= 0 {
		tolerance = FlattenTolerance
	}

	// Phase 1: Flatten
	tcr.flattenCtx.Reset()
	tcr.flattenCtx.FlattenPathTo(path, transform, tolerance)
	segments := tcr.flattenCtx.Segments()

	if segments.Len() == 0 {
		return
	}

	// Phase 2: Coarse rasterization
	tcr.coarse.Rasterize(segments)
	tcr.coarse.SortEntries()

	// Phase 3: Calculate backdrop
	tcr.backdrop = tcr.coarse.CalculateBackdrop()

	// Phase 4: Generate strips
	tcr.strips.SetFillRule(tcr.fillRule)
	tcr.strips.RenderTiles(tcr.coarse, segments, tcr.backdrop)
}

// Grid returns the tile grid with computed coverage.
func (tcr *TileCoverageRasterizer) Grid() *TileGrid {
	return tcr.fine.Grid()
}

// Strips returns the strip renderer with generated strips.
func (tcr *TileCoverageRasterizer) Strips() *StripRenderer {
	return tcr.strips
}

// Segments returns the flattened segments.
func (tcr *TileCoverageRasterizer) Segments() *SegmentList {
	return tcr.flattenCtx.Segments()
}

// RenderToBuffer renders the rasterized coverage to a pixel buffer.
func (tcr *TileCoverageRasterizer) RenderToBuffer(
	buffer []uint8,
	stride int,
	color [4]uint8,
) {
	tcr.fine.RenderToBuffer(buffer, int(tcr.width), int(tcr.height), stride, color)
}

// RenderStripsToBuffer renders strips to a pixel buffer.
func (tcr *TileCoverageRasterizer) RenderStripsToBuffer(
	buffer []uint8,
	stride int,
	color [4]uint8,
) {
	strips := tcr.strips.Strips()
	alphas := tcr.strips.Alphas()

	if len(strips) == 0 {
		return
	}

	for i := 0; i < len(strips)-1; i++ {
		strip := strips[i]
		nextStrip := strips[i+1]

		if strip.X == 0xFFFF {
			continue // Sentinel, skip
		}

		// Calculate strip width in pixels
		endX := calculateStripEndX(strip, nextStrip)

		// Fill gap if needed
		if strip.FillGap && i > 0 {
			prevStrip := strips[i-1]
			if prevStrip.Y == strip.Y && prevStrip.X != 0xFFFF {
				prevEndX := strip.X
				//nolint:gosec // Integer overflow is bounded by viewport dimensions
				gapStartX := prevStrip.X + uint16((strip.AlphaIdx-prevStrip.AlphaIdx)/TileHeight)
				fillGap(buffer, stride, int(gapStartX), int(prevEndX), int(strip.Y), TileHeight, color)
			}
		}

		// Render strip alphas
		renderStripAlphas(buffer, stride, int(strip.X), int(strip.Y), int(endX-strip.X), alphas[strip.AlphaIdx:nextStrip.AlphaIdx], color)
	}
}

// calculateStripEndX calculates the end X coordinate of a strip.
//
//nolint:gosec // Integer overflow is bounded by viewport dimensions
func calculateStripEndX(strip, nextStrip SparseStrip) uint16 {
	switch {
	case nextStrip.X == 0xFFFF:
		// Use alpha buffer to determine width
		alphaCount := nextStrip.AlphaIdx - strip.AlphaIdx
		return strip.X + uint16(alphaCount/TileHeight)
	case nextStrip.Y == strip.Y:
		return nextStrip.X
	default:
		alphaCount := nextStrip.AlphaIdx - strip.AlphaIdx
		return strip.X + uint16(alphaCount/TileHeight)
	}
}

// fillGap fills a solid gap between strips.
func fillGap(buffer []uint8, stride, startX, endX, y, height int, color [4]uint8) {
	for py := 0; py < height; py++ {
		pixelY := y + py
		rowOffset := pixelY * stride
		for px := startX; px < endX; px++ {
			idx := rowOffset + px*4
			if idx+3 < len(buffer) {
				buffer[idx+0] = color[0]
				buffer[idx+1] = color[1]
				buffer[idx+2] = color[2]
				buffer[idx+3] = color[3]
			}
		}
	}
}

// renderStripAlphas renders alpha values for a strip.
func renderStripAlphas(buffer []uint8, stride, x, y, width int, alphas []uint8, color [4]uint8) {
	alphaIdx := 0
	for px := 0; px < width && alphaIdx < len(alphas); px++ {
		pixelX := x + px
		for py := 0; py < TileHeight && alphaIdx < len(alphas); py++ {
			pixelY := y + py
			alpha := alphas[alphaIdx]
			alphaIdx++

			if alpha == 0 {
				continue
			}

			idx := pixelY*stride + pixelX*4
			if idx+3 >= len(buffer) {
				continue
			}

			// Premultiplied alpha blending
			srcA := uint16(alpha) * uint16(color[3]) / 255
			invA := 255 - srcA

			//nolint:gosec // Result is bounded 0-255
			buffer[idx+0] = uint8((uint16(color[0])*srcA + uint16(buffer[idx+0])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+1] = uint8((uint16(color[1])*srcA + uint16(buffer[idx+1])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+2] = uint8((uint16(color[2])*srcA + uint16(buffer[idx+2])*invA) / 255)
			//nolint:gosec // Result is bounded 0-255
			buffer[idx+3] = uint8(srcA + uint16(buffer[idx+3])*invA/255)
		}
	}
}

// TileCoveragePool manages pooled rasterizers for reuse.
type TileCoveragePool struct {
	mu   sync.Mutex
	pool []*TileCoverageRasterizer
}

// globalTileCoveragePool is the default pool.
var globalTileCoveragePool = NewTileCoveragePool()

// NewTileCoveragePool creates a new pool.
func NewTileCoveragePool() *TileCoveragePool {
	return &TileCoveragePool{
		pool: make([]*TileCoverageRasterizer, 0, 4),
	}
}

// Get retrieves a rasterizer from the pool or creates a new one.
func (p *TileCoveragePool) Get(config TileCoverageConfig) *TileCoverageRasterizer {
	p.mu.Lock()
	if len(p.pool) > 0 {
		tcr := p.pool[len(p.pool)-1]
		p.pool = p.pool[:len(p.pool)-1]
		p.mu.Unlock()

		tcr.Reset()
		tcr.SetSize(config.Width, config.Height)
		tcr.SetFillRule(config.FillRule)
		return tcr
	}
	p.mu.Unlock()

	return NewTileCoverageRasterizer(config)
}

// Put returns a rasterizer to the pool.
func (p *TileCoveragePool) Put(tcr *TileCoverageRasterizer) {
	if tcr == nil {
		return
	}
	tcr.Reset()

	p.mu.Lock()
	if len(p.pool) < 16 {
		p.pool = append(p.pool, tcr)
	}
	p.mu.Unlock()
}

// RasterizePath is a convenience function that rasterizes a path using a pooled rasterizer.
func RasterizePath(
	path *scene.Path,
	transform scene.Affine,
	width, height uint16,
	fillRule scene.FillStyle,
) *TileGrid {
	config := DefaultConfig(width, height)
	config.FillRule = fillRule

	tcr := globalTileCoveragePool.Get(config)
	defer globalTileCoveragePool.Put(tcr)

	tcr.RasterizePath(path, transform, FlattenTolerance)
	return tcr.Grid()
}

// Stats contains statistics about the rasterization process.
type Stats struct {
	SegmentCount    int
	TileEntryCount  int
	ActiveTileCount int
	StripCount      int
	AlphaByteCount  int
}

// GetStats returns statistics about the current rasterization state.
func (tcr *TileCoverageRasterizer) GetStats() Stats {
	return Stats{
		SegmentCount:    tcr.flattenCtx.Segments().Len(),
		TileEntryCount:  len(tcr.coarse.Entries()),
		ActiveTileCount: tcr.fine.Grid().TileCount(),
		StripCount:      len(tcr.strips.Strips()),
		AlphaByteCount:  len(tcr.strips.Alphas()),
	}
}
