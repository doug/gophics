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

type TileCoverageRasterizer struct {
	// Pipeline components
	flattenCtx *FlattenContext
	coarse     *CoarseRasterizer
	fine       *FineRasterizer

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
	tcr.backdrop = nil
}

// SetFillRule sets the fill rule for rendering.
func (tcr *TileCoverageRasterizer) SetFillRule(rule scene.FillStyle) {
	tcr.fillRule = rule
	tcr.fine.SetFillRule(rule)
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

// Grid returns the tile grid with computed coverage.
func (tcr *TileCoverageRasterizer) Grid() *TileGrid {
	return tcr.fine.Grid()
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
}

// GetStats returns statistics about the current rasterization state.
func (tcr *TileCoverageRasterizer) GetStats() Stats {
	return Stats{
		SegmentCount:    tcr.flattenCtx.Segments().Len(),
		TileEntryCount:  len(tcr.coarse.Entries()),
		ActiveTileCount: tcr.fine.Grid().TileCount(),
	}
}
