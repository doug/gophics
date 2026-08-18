//go:build !nogpu

// GPU presentation for the compute pipeline.
//
// The compute path used to finish a frame by reading its entire output buffer
// back to host memory and compositing it in a CPU loop over every pixel. That
// dominated the cost of a compute frame, and for a GPU-direct target — a
// texture view with no CPU buffer behind it — it did not work at all: the loop
// had nothing to write into, skipped every pixel, and reported success.
//
// This keeps the result on the GPU. The packed output is copied into a storage
// texture by a small compute pass, and that texture is composited onto the
// target view by a full-screen triangle with premultiplied source-over
// blending, matching what the CPU loop used to compute.

package gpu

import (
	_ "embed"
	"encoding/binary"
	"fmt"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

//go:embed shaders/vello_present.wgsl
var velloPresentShaderWGSL string

// velloPresenter composites a compute frame onto a texture view.
//
// Resources are cached across frames and rebuilt only when the target size or
// format changes: a presenter that allocated a texture and two pipelines per
// frame would replace one per-frame cost with another.
type velloPresenter struct {
	device *wgpu.Device
	queue  *wgpu.Queue

	// Buffer-to-texture copy.
	copyPipeline *wgpu.ComputePipeline
	copyLayout   *wgpu.BindGroupLayout
	cfgBuf       *wgpu.Buffer

	// Composite onto the target view.
	blitPipeline *wgpu.RenderPipeline
	blitLayout   *wgpu.BindGroupLayout
	sampler      *wgpu.Sampler
	blitFormat   gputypes.TextureFormat

	// Staging texture holding one frame, sized to the target.
	staging     *wgpu.Texture
	stagingView *wgpu.TextureView
	stagingW    uint32
	stagingH    uint32
}

// destroy releases every cached resource.
func (p *velloPresenter) destroy() {
	if p == nil {
		return
	}
	p.releaseStaging()
	for _, r := range []interface{ Release() }{
		p.copyPipeline, p.copyLayout, p.cfgBuf,
		p.blitPipeline, p.blitLayout, p.sampler,
	} {
		if r != nil {
			r.Release()
		}
	}
	p.copyPipeline, p.copyLayout, p.cfgBuf = nil, nil, nil
	p.blitPipeline, p.blitLayout, p.sampler = nil, nil, nil
}

func (p *velloPresenter) releaseStaging() {
	if p.stagingView != nil {
		p.stagingView.Release()
		p.stagingView = nil
	}
	if p.staging != nil {
		p.staging.Release()
		p.staging = nil
	}
	p.stagingW, p.stagingH = 0, 0
}

// ensureCopy builds the buffer-to-texture compute pipeline.
func (p *velloPresenter) ensureCopy() error {
	if p.copyPipeline != nil {
		return nil
	}

	layout, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
		Label: "vello_present_copy_bgl",
		Entries: []gputypes.BindGroupLayoutEntry{
			{Binding: 0, Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeUniform, MinBindingSize: 8}},
			{Binding: 1, Visibility: gputypes.ShaderStageCompute,
				Buffer: &gputypes.BufferBindingLayout{Type: gputypes.BufferBindingTypeReadOnlyStorage}},
			{Binding: 2, Visibility: gputypes.ShaderStageCompute,
				StorageTexture: &gputypes.StorageTextureBindingLayout{
					Access:        gputypes.StorageTextureAccessWriteOnly,
					Format:        gputypes.TextureFormatRGBA8Unorm,
					ViewDimension: gputypes.TextureViewDimension2D,
				}},
		},
	})
	if err != nil {
		return fmt.Errorf("vello present: copy bind group layout: %w", err)
	}
	p.copyLayout = layout

	pl, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "vello_present_copy_pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{layout},
	})
	if err != nil {
		return fmt.Errorf("vello present: copy pipeline layout: %w", err)
	}
	defer pl.Release()

	mod, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "vello_present_copy",
		WGSL:  velloPresentShaderWGSL,
	})
	if err != nil {
		return fmt.Errorf("vello present: copy shader: %w", err)
	}
	defer mod.Release()

	pipe, err := p.device.CreateComputePipeline(&wgpu.ComputePipelineDescriptor{
		Label:      "vello_present_copy",
		Layout:     pl,
		Module:     mod,
		EntryPoint: "cs_present",
	})
	if err != nil {
		return fmt.Errorf("vello present: copy pipeline: %w", err)
	}
	p.copyPipeline = pipe

	cfg, err := p.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "vello_present_cfg",
		Size:  8,
		Usage: gputypes.BufferUsageUniform | gputypes.BufferUsageCopyDst,
	})
	if err != nil {
		return fmt.Errorf("vello present: config buffer: %w", err)
	}
	p.cfgBuf = cfg
	return nil
}

// ensureBlit builds the compositing pipeline for a target format. The format is
// part of the pipeline, so a target with a different one rebuilds it.
func (p *velloPresenter) ensureBlit(format gputypes.TextureFormat) error {
	if p.blitPipeline != nil && p.blitFormat == format {
		return nil
	}
	if p.blitPipeline != nil {
		p.blitPipeline.Release()
		p.blitPipeline = nil
	}

	if p.blitLayout == nil {
		layout, err := p.device.CreateBindGroupLayout(&wgpu.BindGroupLayoutDescriptor{
			Label: "vello_present_blit_bgl",
			Entries: []gputypes.BindGroupLayoutEntry{
				{Binding: 0, Visibility: gputypes.ShaderStageFragment,
					Texture: &gputypes.TextureBindingLayout{
						SampleType:    gputypes.TextureSampleTypeFloat,
						ViewDimension: gputypes.TextureViewDimension2D,
					}},
				{Binding: 1, Visibility: gputypes.ShaderStageFragment,
					Sampler: &gputypes.SamplerBindingLayout{Type: gputypes.SamplerBindingTypeFiltering}},
			},
		})
		if err != nil {
			return fmt.Errorf("vello present: blit bind group layout: %w", err)
		}
		p.blitLayout = layout
	}

	if p.sampler == nil {
		s, err := p.device.CreateSampler(&wgpu.SamplerDescriptor{Label: "vello_present_sampler"})
		if err != nil {
			return fmt.Errorf("vello present: sampler: %w", err)
		}
		p.sampler = s
	}

	pl, err := p.device.CreatePipelineLayout(&wgpu.PipelineLayoutDescriptor{
		Label:            "vello_present_blit_pl",
		BindGroupLayouts: []*wgpu.BindGroupLayout{p.blitLayout},
	})
	if err != nil {
		return fmt.Errorf("vello present: blit pipeline layout: %w", err)
	}
	defer pl.Release()

	mod, err := p.device.CreateShaderModule(&wgpu.ShaderModuleDescriptor{
		Label: "vello_present_blit",
		WGSL:  blitShaderSource,
	})
	if err != nil {
		return fmt.Errorf("vello present: blit shader: %w", err)
	}
	defer mod.Release()

	// Premultiplied source-over, the same operation the CPU loop performed:
	// dst' = src + dst * (1 - src.a).
	blend := gputypes.BlendStatePremultiplied()
	pipe, err := p.device.CreateRenderPipeline(&wgpu.RenderPipelineDescriptor{
		Label:  "vello_present_blit",
		Layout: pl,
		Vertex: wgpu.VertexState{Module: mod, EntryPoint: "vs_main"},
		Fragment: &wgpu.FragmentState{
			Module:     mod,
			EntryPoint: "fs_main",
			Targets: []gputypes.ColorTargetState{{
				Format:    format,
				Blend:     &blend,
				WriteMask: gputypes.ColorWriteMaskAll,
			}},
		},
		Primitive:   gputypes.PrimitiveState{Topology: gputypes.PrimitiveTopologyTriangleList},
		Multisample: gputypes.MultisampleState{Count: 1, Mask: 0xFFFFFFFF},
	})
	if err != nil {
		return fmt.Errorf("vello present: blit pipeline: %w", err)
	}
	p.blitPipeline = pipe
	p.blitFormat = format
	return nil
}

// ensureStaging allocates the intermediate texture, reusing it while the size
// holds.
func (p *velloPresenter) ensureStaging(w, h uint32) error {
	if p.staging != nil && p.stagingW == w && p.stagingH == h {
		return nil
	}
	p.releaseStaging()

	tex, err := p.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "vello_present_staging",
		Size:          wgpu.Extent3D{Width: w, Height: h, DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     gputypes.TextureDimension2D,
		Format:        gputypes.TextureFormatRGBA8Unorm,
		Usage:         gputypes.TextureUsageStorageBinding | gputypes.TextureUsageTextureBinding,
	})
	if err != nil {
		return fmt.Errorf("vello present: staging texture: %w", err)
	}
	view, err := p.device.CreateTextureView(tex, nil)
	if err != nil {
		tex.Release()
		return fmt.Errorf("vello present: staging view: %w", err)
	}
	p.staging, p.stagingView, p.stagingW, p.stagingH = tex, view, w, h
	return nil
}

// present composites the packed output buffer onto the target view.
func (p *velloPresenter) present(out *wgpu.Buffer, target gg.GPURenderTarget, w, h uint32) error {
	if w == 0 || h == 0 {
		return nil
	}
	if err := p.ensureCopy(); err != nil {
		return err
	}
	format := target.ViewFormat
	if format == 0 {
		format = gputypes.TextureFormatBGRA8Unorm
	}
	if err := p.ensureBlit(format); err != nil {
		return err
	}
	if err := p.ensureStaging(w, h); err != nil {
		return err
	}

	cfg := make([]byte, 8)
	binary.LittleEndian.PutUint32(cfg[0:4], w)
	binary.LittleEndian.PutUint32(cfg[4:8], h)
	if err := p.queue.WriteBuffer(p.cfgBuf, 0, cfg); err != nil {
		return fmt.Errorf("vello present: write config: %w", err)
	}

	copyBG, err := p.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "vello_present_copy_bg",
		Layout: p.copyLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, Buffer: p.cfgBuf, Size: 8},
			{Binding: 1, Buffer: out},
			{Binding: 2, TextureView: p.stagingView},
		},
	})
	if err != nil {
		return fmt.Errorf("vello present: copy bind group: %w", err)
	}
	defer copyBG.Release()

	blitBG, err := p.device.CreateBindGroup(&wgpu.BindGroupDescriptor{
		Label:  "vello_present_blit_bg",
		Layout: p.blitLayout,
		Entries: []wgpu.BindGroupEntry{
			{Binding: 0, TextureView: p.stagingView},
			{Binding: 1, Sampler: p.sampler},
		},
	})
	if err != nil {
		return fmt.Errorf("vello present: blit bind group: %w", err)
	}
	defer blitBG.Release()

	targetView := (*wgpu.TextureView)(target.View.Pointer())
	if targetView == nil {
		return fmt.Errorf("vello present: target view is nil")
	}

	enc, err := p.device.CreateCommandEncoder(&wgpu.CommandEncoderDescriptor{Label: "vello_present"})
	if err != nil {
		return fmt.Errorf("vello present: encoder: %w", err)
	}

	pass, err := enc.BeginComputePass(&wgpu.ComputePassDescriptor{Label: "vello_present_copy"})
	if err != nil {
		enc.DiscardEncoding()
		return fmt.Errorf("vello present: copy pass: %w", err)
	}
	pass.SetPipeline(p.copyPipeline)
	pass.SetBindGroup(0, copyBG, nil)
	pass.Dispatch((w+7)/8, (h+7)/8, 1)
	if err := pass.End(); err != nil {
		enc.DiscardEncoding()
		return fmt.Errorf("vello present: end copy pass: %w", err)
	}

	// LoadOpLoad, because the compute result composites *over* whatever the
	// target already holds. Clearing here would discard earlier drawing, which
	// is exactly what the CPU source-over loop was careful not to do.
	rp, err := enc.BeginRenderPass(&wgpu.RenderPassDescriptor{
		Label: "vello_present_blit",
		ColorAttachments: []wgpu.RenderPassColorAttachment{{
			View:    targetView,
			LoadOp:  gputypes.LoadOpLoad,
			StoreOp: gputypes.StoreOpStore,
		}},
	})
	if err != nil {
		enc.DiscardEncoding()
		return fmt.Errorf("vello present: blit pass: %w", err)
	}
	rp.SetPipeline(p.blitPipeline)
	rp.SetBindGroup(0, blitBG, nil)
	rp.Draw(3, 1, 0, 0)
	if err := rp.End(); err != nil {
		enc.DiscardEncoding()
		return fmt.Errorf("vello present: end blit pass: %w", err)
	}

	cmd, err := enc.Finish()
	if err != nil {
		return fmt.Errorf("vello present: finish: %w", err)
	}
	if _, err := p.queue.Submit(cmd); err != nil {
		return fmt.Errorf("vello present: submit: %w", err)
	}
	return nil
}
