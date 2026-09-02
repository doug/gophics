//go:build !(js && wasm)

package mobile

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
)

// GPU frame capture, for diagnosing rendering faults that only appear on a
// device.
//
// A screenshot of the screen is not available here — on iOS 17 and later the
// screenshot service sits behind a tunnel the usual tooling does not speak —
// and it would not be the right picture anyway. What is wanted is the frame
// this code produced, in the pixels it produced, so it can be compared against
// the same frame rendered on a desktop. So the frame is rendered again into an
// offscreen texture and read back.
//
// It renders through the same ggcanvas and the same device as the live path,
// which matters: a fault in the glyph atlas texture is in that device's
// memory, and any frame drawn from it shows the fault.

// captureTarget renders a frame into an offscreen texture instead of the
// surface, and keeps the pixels.
type captureTarget struct {
	g   *mobileGPU
	img *image.RGBA
	err error
}

func (t *captureTarget) RenderGPU(replay func(*gg.Context)) {
	g := t.g
	if err := g.ggc.Draw(replay); err != nil {
		t.err = fmt.Errorf("mobile: capture draw: %w", err)
		return
	}
	t.img, t.err = g.renderOffscreen()
}

// renderOffscreen draws the canvas into a fresh texture and reads it back.
func (g *mobileGPU) renderOffscreen() (*image.RGBA, error) {
	w, h := g.pw, g.ph
	if w <= 0 || h <= 0 {
		return nil, errors.New("mobile: capture surface has no size")
	}

	tex, err := g.device.CreateTexture(&wgpu.TextureDescriptor{
		Label:         "gophics_capture",
		Size:          wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
		MipLevelCount: 1,
		SampleCount:   1,
		Dimension:     wgpu.TextureDimension2D,
		Format:        g.format,
		Usage:         gputypes.TextureUsageRenderAttachment | gputypes.TextureUsageCopySrc,
	})
	if err != nil {
		return nil, fmt.Errorf("mobile: capture create texture: %w", err)
	}
	defer tex.Release()

	view, err := g.device.CreateTextureView(tex, nil)
	if err != nil {
		return nil, fmt.Errorf("mobile: capture create view: %w", err)
	}
	defer view.Release()

	if err := g.ggc.RenderDirect(gpucontext.NewTextureView(unsafe.Pointer(view)),
		uint32(w), uint32(h)); err != nil {
		return nil, fmt.Errorf("mobile: capture render: %w", err)
	}

	// Rows in a copy destination are aligned; the image is packed, so the two
	// strides differ and the copy has to be unpicked row by row below.
	const rowAlign = 256
	rowBytes := uint32(w) * 4
	if rem := rowBytes % rowAlign; rem != 0 {
		rowBytes += rowAlign - rem
	}
	bufSize := uint64(rowBytes) * uint64(h)

	buf, err := g.device.CreateBuffer(&wgpu.BufferDescriptor{
		Label: "gophics_capture_staging",
		Size:  bufSize,
		Usage: gputypes.BufferUsageCopyDst | gputypes.BufferUsageMapRead,
	})
	if err != nil {
		return nil, fmt.Errorf("mobile: capture create staging buffer: %w", err)
	}
	defer buf.Release()

	enc, err := g.device.CreateCommandEncoder(nil)
	if err != nil {
		return nil, fmt.Errorf("mobile: capture encoder: %w", err)
	}
	enc.CopyTextureToBuffer(tex, buf, []wgpu.BufferTextureCopy{{
		TextureBase: wgpu.ImageCopyTexture{Texture: tex, MipLevel: 0, Aspect: gputypes.TextureAspectAll},
		BufferLayout: wgpu.ImageDataLayout{
			Offset: 0, BytesPerRow: rowBytes, RowsPerImage: uint32(h),
		},
		Size: wgpu.Extent3D{Width: uint32(w), Height: uint32(h), DepthOrArrayLayers: 1},
	}})
	cmds, err := enc.Finish()
	if err != nil {
		return nil, fmt.Errorf("mobile: capture finish: %w", err)
	}
	g.device.Queue().Submit(cmds)
	g.device.Poll(wgpu.PollWait)

	pending, err := buf.MapAsync(wgpu.MapModeRead, 0, bufSize)
	if err != nil {
		return nil, fmt.Errorf("mobile: capture map: %w", err)
	}
	g.device.Poll(wgpu.PollWait)
	ready, statusErr := pending.Status()
	if !ready {
		return nil, errors.New("mobile: capture staging buffer not ready after poll")
	}
	if statusErr != nil {
		return nil, fmt.Errorf("mobile: capture map status: %w", statusErr)
	}
	mapped, err := buf.MappedRange(0, bufSize)
	if err != nil {
		return nil, fmt.Errorf("mobile: capture mapped range: %w", err)
	}
	raw := mapped.Bytes()

	bgra := g.format == gputypes.TextureFormatBGRA8Unorm ||
		g.format == gputypes.TextureFormatBGRA8UnormSrgb
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			s := y*int(rowBytes) + x*4
			d := img.PixOffset(x, y)
			if bgra {
				img.Pix[d+0], img.Pix[d+1] = raw[s+2], raw[s+1]
				img.Pix[d+2], img.Pix[d+3] = raw[s+0], raw[s+3]
			} else {
				copy(img.Pix[d:d+4], raw[s:s+4])
			}
		}
	}
	_ = buf.Unmap()
	return img, nil
}

// CaptureGPU renders one frame through the GPU path into an offscreen texture
// and returns it as PNG bytes, or nil if there is no GPU surface.
//
// It is a diagnostic, not a present path: the frame it returns is drawn with
// the same device, canvas and glyph atlas the screen is drawn with, so a fault
// living in that atlas texture appears here too — which a screenshot of the
// screen could show, but which no screenshot service on this platform will
// give us.
func (b *Bridge) CaptureGPU(dtSeconds float64) []byte {
	if b.gpu == nil {
		return nil
	}
	t := &captureTarget{g: b.gpu}
	b.capturing = t
	b.dirty.Store(true) // a capture must render even when the scene is unchanged
	b.handler.Frame(b, &frame{b: b}, dtSeconds)
	b.capturing = nil
	if t.err != nil || t.img == nil {
		return nil
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, t.img); err != nil {
		return nil
	}
	return buf.Bytes()
}
