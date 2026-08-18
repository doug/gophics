//go:build !nogpu

// Real compute dispatch for the fine rasterizer.
//
// GPUFineRasterizer compiled its shader, built its layouts and created three
// compute pipelines from the day it was written — and then computed coverage on
// the CPU, because "buffer binding needs HAL extensions". That stopped being
// true: wgpu grew CreateBuffer, CreateBindGroup and a native compute pass with
// SetPipeline/SetBindGroup/Dispatch, and vello_compute.go in this same package
// already dispatches through them. The comment outlived the limitation.
//
// The shape that leaves behind is the dangerous one. A type named
// GPUFineRasterizer, holding real pipelines, returning correct pixels from a
// CPU loop, passes every test while never touching the GPU. So the split here
// is deliberate:
//
//   - RasterizeGPU dispatches and fails loudly. Tests call it, so a GPU that
//     silently draws nothing is a red test rather than a green one.
//   - Rasterize prefers the GPU and falls back to the CPU with a warning, which
//     is what a frame in flight wants.
//
// Both paths compute the same thing, which is what makes the equivalence test
// possible: computeCoverageCPU is the shader's algorithm written out in Go, so
// TestGPUFineMatchesCPU diffs the GPU against a reference that lives beside it.

package gpu

import (
	"context"
	"encoding/binary"
	"fmt"
	"math"

	"github.com/doug/gophics/internal/gfx/gg/scene"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// Byte sizes of the shader-facing structs. WGSL fixes these layouts, so they
// are written out explicitly rather than taken from unsafe.Sizeof: a field
// reordered on the Go side should break the encoder, not silently shift every
// element the shader reads.
const (
	gpuSegmentSize     = 32 // 4 × f32 + 4 × i32
	gpuTileSegRefSize  = 16 // 4 × u32
	gpuTileInfoSize    = 32 // 5 × u32/i32 + 3 × u32 padding
	gpuFineConfigSize  = 32 // 8 × u32
	gpuCoveragePerTile = TileSize * TileSize
)

// maxFineWorkgroups is WebGPU's floor for maxComputeWorkgroupsPerDimension.
// One workgroup handles one tile, so a scene past this many tiles cannot be
// dispatched in a single 1D pass; the caller falls back rather than silently
// rasterizing a prefix of the geometry.
const maxFineWorkgroups = 65535

// encodeSegments packs segments into the layout of Segment in fine.wgsl.
func encodeSegments(segs []GPUSegment) []byte {
	buf := make([]byte, len(segs)*gpuSegmentSize)
	le := binary.LittleEndian
	for i, s := range segs {
		o := i * gpuSegmentSize
		le.PutUint32(buf[o+0:], math.Float32bits(s.X0))
		le.PutUint32(buf[o+4:], math.Float32bits(s.Y0))
		le.PutUint32(buf[o+8:], math.Float32bits(s.X1))
		le.PutUint32(buf[o+12:], math.Float32bits(s.Y1))
		le.PutUint32(buf[o+16:], uint32(s.Winding))
		le.PutUint32(buf[o+20:], uint32(s.TileY0))
		le.PutUint32(buf[o+24:], uint32(s.TileY1))
		le.PutUint32(buf[o+28:], uint32(s.Padding))
	}
	return buf
}

// encodeTileSegRefs packs refs into the layout of TileSegmentRef in fine.wgsl.
func encodeTileSegRefs(refs []GPUTileSegmentRef) []byte {
	buf := make([]byte, len(refs)*gpuTileSegRefSize)
	le := binary.LittleEndian
	for i, r := range refs {
		o := i * gpuTileSegRefSize
		le.PutUint32(buf[o+0:], r.TileX)
		le.PutUint32(buf[o+4:], r.TileY)
		le.PutUint32(buf[o+8:], r.SegmentIdx)
		le.PutUint32(buf[o+12:], r.WindingFlag)
	}
	return buf
}

// encodeTileInfos packs tiles into the layout of TileInfo in fine.wgsl.
func encodeTileInfos(tiles []GPUTileInfo) []byte {
	buf := make([]byte, len(tiles)*gpuTileInfoSize)
	le := binary.LittleEndian
	for i, t := range tiles {
		o := i * gpuTileInfoSize
		le.PutUint32(buf[o+0:], t.TileX)
		le.PutUint32(buf[o+4:], t.TileY)
		le.PutUint32(buf[o+8:], t.StartIdx)
		le.PutUint32(buf[o+12:], t.Count)
		le.PutUint32(buf[o+16:], uint32(t.Backdrop))
		// padding1..3 stay zero
	}
	return buf
}

// encodeFineConfig packs the Config uniform from fine.wgsl.
func encodeFineConfig(width, height uint16, tileCount int, fillRule scene.FillStyle) []byte {
	buf := make([]byte, gpuFineConfigSize)
	le := binary.LittleEndian
	cols := (uint32(width) + TileSize - 1) / TileSize
	rows := (uint32(height) + TileSize - 1) / TileSize
	le.PutUint32(buf[0:], uint32(width))
	le.PutUint32(buf[4:], uint32(height))
	le.PutUint32(buf[8:], cols)
	le.PutUint32(buf[12:], rows)
	le.PutUint32(buf[16:], uint32(tileCount))
	le.PutUint32(buf[20:], fineFillRuleCode(fillRule))
	return buf
}

// fineFillRuleCode maps the fill rule to the shader's encoding (0 nonzero,
// 1 even-odd), matching windingToCoverage on the CPU side.
func fineFillRuleCode(fillRule scene.FillStyle) uint32 {
	if fillRule == scene.FillEvenOdd {
		return 1
	}
	return 0
}

// storageBuffer uploads data as a read-only storage buffer. An empty slice
// still gets a one-element allocation: WebGPU rejects a zero-sized binding, and
// the shader never indexes an array it was told is empty.
func (r *GPUFineRasterizer) storageBuffer(label string, data []byte, stride int) (*wgpu.Buffer, error) {
	size := len(data)
	if size == 0 {
		size = stride
	}
	buf, err := r.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: label,
		Size:  uint64(size),
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create %s: %w", label, err)
	}
	if len(data) > 0 {
		if err := r.queue.WriteBuffer(buf, 0, data); err != nil {
			buf.Release()
			return nil, fmt.Errorf("gpu_fine: upload %s: %w", label, err)
		}
	}
	return buf, nil
}

// rasterizeGPULocked runs the fine shader over the prepared tile data and
// returns unpacked 8-bit coverage, one byte per pixel per tile. The caller
// holds r.mu.
func (r *GPUFineRasterizer) rasterizeGPULocked(
	segs []GPUSegment,
	refs []GPUTileSegmentRef,
	tiles []GPUTileInfo,
	fillRule scene.FillStyle,
) ([]uint8, error) {
	if len(tiles) > maxFineWorkgroups {
		return nil, fmt.Errorf("gpu_fine: %d tiles exceeds the %d workgroup dispatch limit", len(tiles), maxFineWorkgroups)
	}

	configBuf, err := r.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "fine_config",
		Size:  gpuFineConfigSize,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create config buffer: %w", err)
	}
	defer configBuf.Release()
	if err := r.queue.WriteBuffer(configBuf, 0, encodeFineConfig(r.width, r.height, len(tiles), fillRule)); err != nil {
		return nil, fmt.Errorf("gpu_fine: upload config: %w", err)
	}

	segBuf, err := r.storageBuffer("fine_segments", encodeSegments(segs), gpuSegmentSize)
	if err != nil {
		return nil, err
	}
	defer segBuf.Release()

	refBuf, err := r.storageBuffer("fine_tile_segments", encodeTileSegRefs(refs), gpuTileSegRefSize)
	if err != nil {
		return nil, err
	}
	defer refBuf.Release()

	tileBuf, err := r.storageBuffer("fine_tiles", encodeTileInfos(tiles), gpuTileInfoSize)
	if err != nil {
		return nil, err
	}
	defer tileBuf.Release()

	covSize := uint64(len(tiles) * gpuCoveragePerTile)
	covBuf, err := r.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "fine_coverage",
		Size:  covSize,
		Usage: gputypes.BufferUsageStorage | gputypes.BufferUsageCopySrc,
	})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create coverage buffer: %w", err)
	}
	defer covBuf.Release()

	inputBG, err := r.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "fine_input",
		Layout: r.inputBindLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: configBuf, Size: gpuFineConfigSize},
			{Binding: 1, Buffer: segBuf},
			{Binding: 2, Buffer: refBuf},
			{Binding: 3, Buffer: tileBuf},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create input bind group: %w", err)
	}
	defer inputBG.Release()

	outputBG, err := r.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:   "fine_output",
		Layout:  r.outputBindLayout,
		Entries: []wgpu.BindGroupEntry{{Binding: 0, Buffer: covBuf}},
	})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create output bind group: %w", err)
	}
	defer outputBG.Release()

	encoder, err := r.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "fine_dispatch"})
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: create encoder: %w", err)
	}

	pass, err := encoder.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "fine"})
	if err != nil {
		encoder.DiscardEncoding()
		return nil, fmt.Errorf("gpu_fine: begin compute pass: %w", err)
	}
	pass.SetPipeline(r.finePipeline)
	pass.SetBindGroup(0, inputBG, nil)
	pass.SetBindGroup(1, outputBG, nil)
	// One workgroup per tile; the shader's 16 threads cover the 4×4 pixels.
	pass.Dispatch(uint32(len(tiles)), 1, 1)
	if err := pass.End(); err != nil {
		encoder.DiscardEncoding()
		return nil, fmt.Errorf("gpu_fine: end compute pass: %w", err)
	}

	staging, err := r.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "fine_coverage_readback",
		Size:  covSize,
		Usage: gputypes.BufferUsageMapRead | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		encoder.DiscardEncoding()
		return nil, fmt.Errorf("gpu_fine: create readback buffer: %w", err)
	}
	defer staging.Release()

	encoder.CopyBufferToBuffer(covBuf, 0, staging, 0, covSize)

	cmdBuf, err := encoder.Finish()
	if err != nil {
		return nil, fmt.Errorf("gpu_fine: finish encoding: %w", err)
	}
	if _, err := r.queue.Submit(cmdBuf); err != nil {
		return nil, fmt.Errorf("gpu_fine: submit: %w", err)
	}

	if err := staging.Map(context.Background(), wgpu.MapModeRead, 0, covSize); err != nil {
		return nil, fmt.Errorf("gpu_fine: map readback: %w", err)
	}
	rng, err := staging.MappedRange(0, covSize)
	if err != nil {
		if uerr := staging.Unmap(); uerr != nil {
			slogger().Warn("gpu_fine: unmap failed", "err", uerr)
		}
		return nil, fmt.Errorf("gpu_fine: mapped range: %w", err)
	}
	coverage := make([]uint8, covSize)
	copy(coverage, rng.Bytes())
	if err := staging.Unmap(); err != nil {
		slogger().Warn("gpu_fine: unmap failed", "err", err)
	}

	return coverage, nil
}
