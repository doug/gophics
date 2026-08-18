//go:build !nogpu

// Package wgpu provides GPU-accelerated rendering using WebGPU.
package gpu

import (
	_ "embed"
	"fmt"
	"math"
	"sort"
	"sync"

	"github.com/doug/gophics/internal/gfx/gg/scene"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

//go:embed shaders/fine.wgsl
var fineShaderWGSL string

// GPUSegment is the GPU-compatible layout of LineSegment.
// Must match the Segment struct in fine.wgsl.
type GPUSegment struct {
	X0      float32 // Start X coordinate
	Y0      float32 // Start Y coordinate
	X1      float32 // End X coordinate
	Y1      float32 // End Y coordinate
	Winding int32   // Winding direction: +1 or -1
	TileY0  int32   // Starting tile Y (precomputed)
	TileY1  int32   // Ending tile Y (precomputed)
	Padding int32   // Padding for alignment
}

// GPUTileSegmentRef maps a segment to a tile.
// Must match TileSegmentRef in fine.wgsl.
type GPUTileSegmentRef struct {
	TileX       uint32 // Tile X coordinate
	TileY       uint32 // Tile Y coordinate
	SegmentIdx  uint32 // Index into segments array
	WindingFlag uint32 // Whether this contributes winding (0 or 1)
}

// GPUTileInfo contains tile processing information.
// Must match TileInfo in fine.wgsl.
type GPUTileInfo struct {
	TileX    uint32 // Tile X coordinate
	TileY    uint32 // Tile Y coordinate
	StartIdx uint32 // Start index in tile_segments
	Count    uint32 // Number of segments for this tile
	Backdrop int32  // Accumulated winding from left
	Padding1 uint32 // Padding for alignment
	Padding2 uint32 // Padding for alignment
	Padding3 uint32 // Padding for alignment
}

// GPUFineConfig contains GPU fine rasterization configuration.
// Must match Config in fine.wgsl.
type GPUFineConfig struct {
	ViewportWidth  uint32 // Viewport width in pixels
	ViewportHeight uint32 // Viewport height in pixels
	TileColumns    uint32 // Number of tile columns
	TileRows       uint32 // Number of tile rows
	TileCount      uint32 // Number of tiles to process
	FillRule       uint32 // 0 = NonZero, 1 = EvenOdd
	Padding1       uint32 // Padding for alignment
	Padding2       uint32 // Padding for alignment
}

// FillRuleToGPU converts scene.FillStyle to GPU constant.
func FillRuleToGPU(rule scene.FillStyle) uint32 {
	switch rule {
	case scene.FillEvenOdd:
		return 1
	default:
		return 0 // NonZero
	}
}

// GPUFineRasterizer performs fine rasterization on the GPU.
// It creates compute pipelines and manages GPU buffers for coverage calculation.
//
// Note: This is Phase 6.1 implementation. Full GPU buffer binding requires
// HAL API extensions to expose buffer handles. Currently this serves as
// infrastructure and data flow verification.
type GPUFineRasterizer struct {
	mu sync.Mutex

	device *wgpu.Device
	queue  *wgpu.Queue

	// Compute pipelines
	finePipeline      *wgpu.ComputePipeline
	fineSolidPipeline *wgpu.ComputePipeline
	clearPipeline     *wgpu.ComputePipeline

	// Shader module (cached)
	shaderModule *wgpu.ShaderModule

	// Pipeline layout and bind group layouts
	pipelineLayout   *wgpu.PipelineLayout
	inputBindLayout  *wgpu.BindGroupLayout
	outputBindLayout *wgpu.BindGroupLayout

	// Compiled SPIR-V (cached for verification)
	spirvCode []uint32

	// Viewport dimensions
	width  uint16
	height uint16

	// State
	initialized bool
	shaderReady bool
}

// NewGPUFineRasterizer creates a new GPU fine rasterizer.
// Returns an error if GPU compute is not supported.
func NewGPUFineRasterizer(device *wgpu.Device, queue *wgpu.Queue, width, height uint16) (*GPUFineRasterizer, error) {
	if device == nil || queue == nil {
		return nil, fmt.Errorf("gpu_fine: device and queue are required")
	}

	r := &GPUFineRasterizer{
		device: device,
		queue:  queue,
		width:  width,
		height: height,
	}

	if err := r.init(); err != nil {
		r.Destroy()
		return nil, err
	}

	return r, nil
}

// init initializes GPU resources (pipelines, layouts).
func (r *GPUFineRasterizer) init() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile WGSL to SPIR-V using shared helper
	spirvCode, err := CompileShaderToSPIRV(fineShaderWGSL)
	if err != nil {
		return fmt.Errorf("gpu_fine: %w", err)
	}
	r.spirvCode = spirvCode
	r.shaderReady = true

	// Created from WGSL, not from the SPIR-V above: SPIR-V is a Vulkan-only
	// input, and Metal silently accepts a module it cannot use. The SPIR-V is
	// still compiled, since it is what SPIRVCode() exposes for verification.
	shaderModule, err := CreateShaderModuleWGSL(r.device, "fine_shader", fineShaderWGSL)
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create shader module: %w", err)
	}
	r.shaderModule = shaderModule

	// Create bind group layouts
	if err := r.createBindGroupLayouts(); err != nil {
		return err
	}

	// Create pipeline layout
	if err := r.createPipelineLayout(); err != nil {
		return err
	}

	// Create compute pipelines
	if err := r.createPipelines(); err != nil {
		return err
	}

	r.initialized = true
	return nil
}

// createBindGroupLayouts creates the bind group layouts for the pipeline.
func (r *GPUFineRasterizer) createBindGroupLayouts() error {
	// Input bind group layout (group 0)
	inputLayout, err := r.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "fine_input_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type:           gputypes.BufferBindingTypeUniform,
					MinBindingSize: 32, // sizeof(Config)
				},
			},
			{
				Binding:    1,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    2,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeReadOnlyStorage,
				},
			},
			{
				Binding:    3,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeReadOnlyStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create input bind group layout: %w", err)
	}
	r.inputBindLayout = inputLayout

	// Output bind group layout (group 1)
	outputLayout, err := r.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "fine_output_layout",
		Entries: []gputypes.BindGroupLayoutEntry{
			{
				Binding:    0,
				Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{
					Type: gputypes.BufferBindingTypeStorage,
				},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create output bind group layout: %w", err)
	}
	r.outputBindLayout = outputLayout

	return nil
}

// createPipelineLayout creates the pipeline layout.
func (r *GPUFineRasterizer) createPipelineLayout() error {
	layout, err := r.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "fine_pipeline_layout",
		BindGroupLayouts: []*wgpu.BindGroupLayout{r.inputBindLayout, r.outputBindLayout},
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create pipeline layout: %w", err)
	}
	r.pipelineLayout = layout
	return nil
}

// createPipelines creates the compute pipelines.
func (r *GPUFineRasterizer) createPipelines() error {
	// Main fine rasterization pipeline
	finePipeline, err := r.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "fine_pipeline",
		Layout:     r.pipelineLayout,
		Module:     r.shaderModule,
		EntryPoint: "cs_fine",
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create fine pipeline: %w", err)
	}
	r.finePipeline = finePipeline

	// Solid tile pipeline
	solidPipeline, err := r.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "fine_solid_pipeline",
		Layout:     r.pipelineLayout,
		Module:     r.shaderModule,
		EntryPoint: "cs_fine_solid",
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create solid pipeline: %w", err)
	}
	r.fineSolidPipeline = solidPipeline

	// Clear coverage pipeline
	clearPipeline, err := r.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "clear_coverage_pipeline",
		Layout:     r.pipelineLayout,
		Module:     r.shaderModule,
		EntryPoint: "cs_clear_coverage",
	})
	if err != nil {
		return fmt.Errorf("gpu_fine: failed to create clear pipeline: %w", err)
	}
	r.clearPipeline = clearPipeline

	return nil
}

// Rasterize performs fine rasterization, preferring the GPU and falling back
// to the CPU reference if the dispatch fails. A frame in flight wants pixels
// more than it wants an error, so the fallback is deliberate — but it is
// logged, because a GPU that quietly stopped being used is exactly the failure
// this package spent its first life in. Tests call RasterizeGPU instead.
func (r *GPUFineRasterizer) Rasterize(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
	fillRule scene.FillStyle,
) ([]uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	segs, refs, tiles, err := r.prepareLocked(coarse, segments, backdrop)
	if err != nil || len(tiles) == 0 {
		return nil, err
	}

	coverage, gpuErr := r.rasterizeGPULocked(segs, refs, tiles, fillRule)
	if gpuErr == nil {
		return coverage, nil
	}
	slogger().Warn("gpu_fine: GPU dispatch failed, using CPU reference", "err", gpuErr)
	return r.computeCoverageCPU(segs, refs, tiles, fillRule), nil
}

// RasterizeGPU runs the fine shader on the GPU and reports any failure instead
// of falling back. This is the entry point the equivalence test uses: a silent
// fallback would turn "the GPU drew nothing" into a passing test.
func (r *GPUFineRasterizer) RasterizeGPU(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
	fillRule scene.FillStyle,
) ([]uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	segs, refs, tiles, err := r.prepareLocked(coarse, segments, backdrop)
	if err != nil || len(tiles) == 0 {
		return nil, err
	}
	return r.rasterizeGPULocked(segs, refs, tiles, fillRule)
}

// RasterizeCPU computes coverage with the Go transcription of the shader. It is
// the reference the GPU is diffed against, and the fallback Rasterize uses.
func (r *GPUFineRasterizer) RasterizeCPU(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
	fillRule scene.FillStyle,
) ([]uint8, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	segs, refs, tiles, err := r.prepareLocked(coarse, segments, backdrop)
	if err != nil || len(tiles) == 0 {
		return nil, err
	}
	return r.computeCoverageCPU(segs, refs, tiles, fillRule), nil
}

// prepareLocked turns coarse output into the three shader-facing arrays. It
// returns no tiles when there is nothing to rasterize, which every caller
// treats as an empty result rather than an error. The caller holds r.mu.
func (r *GPUFineRasterizer) prepareLocked(
	coarse *CoarseRasterizer,
	segments *SegmentList,
	backdrop []int32,
) ([]GPUSegment, []GPUTileSegmentRef, []GPUTileInfo, error) {
	if !r.initialized {
		return nil, nil, nil, fmt.Errorf("gpu_fine: rasterizer not initialized")
	}
	if coarse == nil || segments == nil || len(coarse.Entries()) == 0 {
		return nil, nil, nil, nil
	}

	tiles, tileSegRefs := r.buildTileData(coarse, segments, backdrop)
	if len(tiles) == 0 {
		return nil, nil, nil, nil
	}

	return r.convertSegments(segments), r.convertTileSegmentRefs(tileSegRefs), r.convertTileInfos(tiles), nil
}

// computeCoverageCPU computes coverage using CPU (mirrors GPU shader algorithm).
// This serves as reference implementation and fallback.
func (r *GPUFineRasterizer) computeCoverageCPU(
	segments []GPUSegment,
	tileRefs []GPUTileSegmentRef,
	tiles []GPUTileInfo,
	fillRule scene.FillStyle,
) []uint8 {
	coverageSize := len(tiles) * TileSize * TileSize
	coverage := make([]uint8, coverageSize)

	for tileIdx, tile := range tiles {
		// Process each pixel in the tile
		for py := uint32(0); py < TileSize; py++ {
			for px := uint32(0); px < TileSize; px++ {
				winding := float32(tile.Backdrop)

				// Process all segments for this tile
				for i := tile.StartIdx; i < tile.StartIdx+tile.Count; i++ {
					if int(i) >= len(tileRefs) {
						break
					}
					ref := tileRefs[i]
					if int(ref.SegmentIdx) >= len(segments) {
						continue
					}
					seg := segments[ref.SegmentIdx]

					// Compute segment contribution to this pixel
					area := r.computePixelArea(
						seg,
						float32(tile.TileX*TileSize),
						float32(tile.TileY*TileSize),
						px, py,
					)
					winding += area
				}

				// Convert winding to coverage
				cov := r.windingToCoverage(winding, fillRule)

				// Store coverage
				pixelIdx := tileIdx*TileSize*TileSize + int(py)*TileSize + int(px)
				if pixelIdx < len(coverage) {
					coverage[pixelIdx] = uint8(cov*255 + 0.5)
				}
			}
		}
	}

	return coverage
}

// computePixelArea computes segment's area contribution to a pixel.
func (r *GPUFineRasterizer) computePixelArea(
	seg GPUSegment,
	tileLeftX, tileTopY float32,
	pxX, pxY uint32,
) float32 {
	// Convert to tile-relative coordinates
	p0x := seg.X0 - tileLeftX
	p0y := seg.Y0 - tileTopY
	p1x := seg.X1 - tileLeftX
	p1y := seg.Y1 - tileTopY

	// Skip horizontal segments
	if p0y == p1y {
		return 0
	}

	sign := float32(seg.Winding)

	// Line is monotonic (Y0 <= Y1)
	lineTopY := p0y
	lineTopX := p0x
	lineBottomY := p1y
	// lineBottomX := p1x

	// Calculate slopes
	dy := lineBottomY - lineTopY
	dx := p1x - p0x

	var ySlope float32
	if dx == 0 {
		if lineBottomY > lineTopY {
			ySlope = 1e10
		} else {
			ySlope = -1e10
		}
	} else {
		ySlope = dy / dx
	}
	xSlope := 1.0 / ySlope

	// Pixel row bounds
	pxTopY := float32(pxY)
	pxBottomY := pxTopY + 1.0
	pxLeftX := float32(pxX)
	pxRightX := pxLeftX + 1.0

	// Clamp line Y range to this pixel row
	yMin := maxf32(lineTopY, pxTopY)
	yMax := minf32(lineBottomY, pxBottomY)

	// Check if line crosses this row
	if yMin >= yMax {
		return 0
	}

	// Calculate Y coordinates where line intersects pixel left and right edges
	linePxLeftY := lineTopY + (pxLeftX-lineTopX)*ySlope
	linePxRightY := lineTopY + (pxRightX-lineTopX)*ySlope

	// Clamp to pixel row bounds and line Y bounds
	linePxLeftY = clampf32(linePxLeftY, yMin, yMax)
	linePxRightY = clampf32(linePxRightY, yMin, yMax)

	// Calculate X coordinates at the clamped Y values
	linePxLeftYX := lineTopX + (linePxLeftY-lineTopY)*xSlope
	linePxRightYX := lineTopX + (linePxRightY-lineTopY)*xSlope

	// Height of line segment within this pixel
	pixelH := absf32(linePxRightY - linePxLeftY)

	// Trapezoidal area: area between line and pixel right edge
	area := 0.5 * pixelH * (2.0*pxRightX - linePxRightYX - linePxLeftYX)

	return area * sign
}

// windingToCoverage converts winding number to coverage based on fill rule.
func (r *GPUFineRasterizer) windingToCoverage(winding float32, fillRule scene.FillStyle) float32 {
	var cov float32

	if fillRule == scene.FillNonZero {
		// NonZero: coverage = |winding|, clamped to [0, 1]
		cov = absf32(winding)
		if cov > 1.0 {
			cov = 1.0
		}
	} else {
		// EvenOdd: coverage based on fractional part
		absWinding := absf32(winding)
		im1 := float32(int32(absWinding*0.5 + 0.5))
		cov = absf32(absWinding - 2.0*im1)
		if cov > 1.0 {
			cov = 1.0
		}
	}

	return cov
}

// buildTileData builds tile info and segment references from coarse entries.
func (r *GPUFineRasterizer) buildTileData(
	coarse *CoarseRasterizer,
	_ *SegmentList, // segments not used directly, entries contain indices
	backdrop []int32,
) ([]GPUTileInfo, []GPUTileSegmentRef) {
	entries := coarse.Entries()
	if len(entries) == 0 {
		return nil, nil
	}

	coarse.SortEntries()

	// Group entries by tile
	type tileKey struct {
		x, y uint16
	}
	tileMap := make(map[tileKey][]int) // tile -> entry indices

	for i, e := range entries {
		key := tileKey{e.X, e.Y}
		tileMap[key] = append(tileMap[key], i)
	}

	// Build tile info and segment refs
	tiles := make([]GPUTileInfo, 0, len(tileMap))
	refs := make([]GPUTileSegmentRef, 0, len(entries))

	tileColumns := int(coarse.TileColumns())

	// Iterate the tiles in a stable order. Ranging the map directly made the
	// output depend on Go's randomised map iteration: the same scene produced
	// the same pixels in a different buffer order on every call, so nothing
	// downstream could cache a tile, diff two runs, or compare this rasterizer
	// against another one. Scanline order (top to bottom, left to right) is
	// also the order the coarse rasterizer's entries are already sorted into.
	keys := make([]tileKey, 0, len(tileMap))
	for key := range tileMap {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].y != keys[j].y {
			return keys[i].y < keys[j].y
		}
		return keys[i].x < keys[j].x
	})

	for _, key := range keys {
		indices := tileMap[key]
		//nolint:gosec // len(refs) is bounded by number of coarse entries
		startIdx := uint32(len(refs))

		// Get backdrop for this tile
		var bd int32
		if backdrop != nil {
			idx := int(key.y)*tileColumns + int(key.x)
			if idx >= 0 && idx < len(backdrop) {
				bd = backdrop[idx]
			}
		}

		// Add segment refs for this tile
		for _, entryIdx := range indices {
			e := entries[entryIdx]
			refs = append(refs, GPUTileSegmentRef{
				TileX:       uint32(e.X),
				TileY:       uint32(e.Y),
				SegmentIdx:  e.LineIdx,
				WindingFlag: boolToUint32(e.Winding),
			})
		}

		tiles = append(tiles, GPUTileInfo{
			TileX:    uint32(key.x),
			TileY:    uint32(key.y),
			StartIdx: startIdx,
			//nolint:gosec // len(indices) is bounded by segment count
			Count:    uint32(len(indices)),
			Backdrop: bd,
		})
	}

	return tiles, refs
}

// convertSegments converts CPU segments to GPU format.
func (r *GPUFineRasterizer) convertSegments(segments *SegmentList) []GPUSegment {
	lines := segments.Segments()
	result := make([]GPUSegment, len(lines))

	for i, seg := range lines {
		result[i] = GPUSegment{
			X0:      seg.X0,
			Y0:      seg.Y0,
			X1:      seg.X1,
			Y1:      seg.Y1,
			Winding: int32(seg.Winding),
			TileY0:  seg.TileY0,
			TileY1:  seg.TileY1,
		}
	}

	return result
}

// convertTileSegmentRefs is a no-op since GPUTileSegmentRef is already the right type.
func (r *GPUFineRasterizer) convertTileSegmentRefs(refs []GPUTileSegmentRef) []GPUTileSegmentRef {
	return refs
}

// convertTileInfos is a no-op since GPUTileInfo is already the right type.
func (r *GPUFineRasterizer) convertTileInfos(tiles []GPUTileInfo) []GPUTileInfo {
	return tiles
}

// IsInitialized returns whether the rasterizer is initialized.
func (r *GPUFineRasterizer) IsInitialized() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.initialized
}

// IsShaderReady returns whether the shader compiled successfully.
func (r *GPUFineRasterizer) IsShaderReady() bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.shaderReady
}

// SPIRVCode returns the compiled SPIR-V code (for debugging/verification).
func (r *GPUFineRasterizer) SPIRVCode() []uint32 {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.spirvCode
}

// Destroy releases all GPU resources.
func (r *GPUFineRasterizer) Destroy() {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.device == nil {
		return
	}

	// Destroy pipelines
	if r.finePipeline != nil {
		r.finePipeline.Release()
		r.finePipeline = nil
	}
	if r.fineSolidPipeline != nil {
		r.fineSolidPipeline.Release()
		r.fineSolidPipeline = nil
	}
	if r.clearPipeline != nil {
		r.clearPipeline.Release()
		r.clearPipeline = nil
	}

	// Destroy pipeline layout
	if r.pipelineLayout != nil {
		r.pipelineLayout.Release()
		r.pipelineLayout = nil
	}

	// Destroy bind group layouts
	if r.inputBindLayout != nil {
		r.inputBindLayout.Release()
		r.inputBindLayout = nil
	}
	if r.outputBindLayout != nil {
		r.outputBindLayout.Release()
		r.outputBindLayout = nil
	}

	// Destroy shader module
	if r.shaderModule != nil {
		r.shaderModule.Release()
		r.shaderModule = nil
	}

	r.initialized = false
}

// Helper functions

func boolToUint32(b bool) uint32 {
	if b {
		return 1
	}
	return 0
}

// Note: absf32 and clampf32 are defined in fine.go/flatten.go

// Byte serialization helpers (for future GPU buffer upload)

func writeUint32(buf []byte, offset int, val uint32) {
	buf[offset] = byte(val)
	buf[offset+1] = byte(val >> 8)
	buf[offset+2] = byte(val >> 16)
	buf[offset+3] = byte(val >> 24)
}

func writeInt32(buf []byte, offset int, val int32) {
	//nolint:gosec // Intentional bit-cast for GPU buffer serialization
	writeUint32(buf, offset, uint32(val))
}

func writeFloat32(buf []byte, offset int, val float32) {
	bits := math.Float32bits(val)
	writeUint32(buf, offset, bits)
}

func segmentsToBytes(segments []GPUSegment) []byte {
	buf := make([]byte, len(segments)*32)
	for i, seg := range segments {
		off := i * 32
		writeFloat32(buf, off+0, seg.X0)
		writeFloat32(buf, off+4, seg.Y0)
		writeFloat32(buf, off+8, seg.X1)
		writeFloat32(buf, off+12, seg.Y1)
		writeInt32(buf, off+16, seg.Winding)
		writeInt32(buf, off+20, seg.TileY0)
		writeInt32(buf, off+24, seg.TileY1)
		writeInt32(buf, off+28, seg.Padding)
	}
	return buf
}

func tileRefsToBytes(refs []GPUTileSegmentRef) []byte {
	buf := make([]byte, len(refs)*16)
	for i, ref := range refs {
		off := i * 16
		writeUint32(buf, off+0, ref.TileX)
		writeUint32(buf, off+4, ref.TileY)
		writeUint32(buf, off+8, ref.SegmentIdx)
		writeUint32(buf, off+12, ref.WindingFlag)
	}
	return buf
}

func tilesToBytes(tiles []GPUTileInfo) []byte {
	buf := make([]byte, len(tiles)*32)
	for i, tile := range tiles {
		off := i * 32
		writeUint32(buf, off+0, tile.TileX)
		writeUint32(buf, off+4, tile.TileY)
		writeUint32(buf, off+8, tile.StartIdx)
		writeUint32(buf, off+12, tile.Count)
		writeInt32(buf, off+16, tile.Backdrop)
		writeUint32(buf, off+20, tile.Padding1)
		writeUint32(buf, off+24, tile.Padding2)
		writeUint32(buf, off+28, tile.Padding3)
	}
	return buf
}
