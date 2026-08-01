//go:build js && wasm

// Presentation for the web shell, chosen at runtime from Config.Renderer.
//
// A browser <canvas> can hold only ONE context type for its lifetime (2d XOR
// webgpu), so the choice is committed once, before any context is bound:
//   - CPU renderer, or no navigator.gpu: bind a 2d context immediately and blit
//     CPU-rasterized frames with putImageData.
//   - GPU renderer (default/Auto with WebGPU present): acquire the device on a
//     goroutine (navigator.gpu is async), bind the canvas's WebGPU surface, and
//     present GPU-rasterized frames directly — no CPU readback. Until setup
//     completes, frames are skipped (never CPU-blitted, which would taint the
//     canvas); if setup fails before the canvas is bound, it falls back to CPU.
package web

import (
	"image"
	"log"
	"syscall/js"
	"unsafe"

	"github.com/gogpu/gg"
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	"github.com/doug/gossamer/shell"
)

// presenter owns whichever presentation path this run committed to.
type presenter struct {
	w *window

	// CPU path (2d canvas + putImageData).
	ctx2d      js.Value
	buf        js.Value // Uint8ClampedArray cache
	imageData  js.Value
	bufW, bufH int
	cpuActive  bool // a 2d context is bound and CPU frames present

	// GPU path (WebGPU surface).
	device   *wgpu.Device
	surface  *wgpu.Surface
	ggc      *ggcanvas.Canvas
	format   gputypes.TextureFormat
	gpuReady bool
	pw, ph   int // configured physical pixel size
}

// newPresenter commits to a presentation path based on the resolved renderer
// and whether the browser exposes WebGPU.
func newPresenter(w *window) *presenter {
	p := &presenter{w: w}
	if w.renderer != shell.RendererCPU && gpuSupported() {
		go p.setupGPU() // async device acquisition; skips frames until ready
		return p
	}
	if w.renderer == shell.RendererGPU && !gpuSupported() {
		log.Printf("gossamer/web: GPU requested but navigator.gpu is unavailable; using CPU")
	}
	p.initCPU()
	return p
}

func gpuSupported() bool {
	gpu := js.Global().Get("navigator").Get("gpu")
	return !gpu.IsUndefined() && !gpu.IsNull()
}

// initCPU binds the canvas's 2d context (irreversible for this canvas).
func (p *presenter) initCPU() {
	p.ctx2d = p.w.canvas.Call("getContext", "2d")
	p.cpuActive = true
}

func (p *presenter) setupGPU() {
	// Acquire adapter/device BEFORE touching the canvas, so a failure here
	// leaves the canvas clean and CPU fallback is still possible.
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsPrimary})
	if err != nil {
		p.gpuSetupFailed("instance", err)
		return
	}
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		p.gpuSetupFailed("adapter", err)
		return
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		p.gpuSetupFailed("device", err)
		return
	}
	surface, err := inst.CreateSurfaceFromCanvas(p.w.canvas)
	if err != nil {
		// The canvas may now be bound to WebGPU; CPU fallback is unsafe, so
		// skip frames rather than taint it. This is rare (device already OK).
		log.Printf("gossamer/web: gpu surface: %v", err)
		return
	}

	p.device = device
	p.surface = surface
	p.format = preferredCanvasFormat()
	p.configure()

	provider := &webProvider{device: device, queue: device.Queue(), adapter: adapter, format: p.format}
	// gg renders in logical points; the canvas is physical-sized with a device
	// scale so 1 point maps to dpr device pixels.
	c, err := ggcanvas.NewWithScale(provider, int(p.w.logical.W), int(p.w.logical.H), p.w.dpr)
	if err != nil {
		log.Printf("gossamer/web: ggcanvas: %v", err)
		return
	}
	p.ggc = c

	// Force the render-pass pipeline. Auto-selection routes complex scenes to
	// the Vello compute rasterizer, which reads its result back to a CPU pixmap
	// and ignores the surface view — so nothing reaches the canvas. The
	// render-pass path composites directly to the view.
	if pma, ok := gg.Accelerator().(gg.PipelineModeAware); ok {
		pma.SetPipelineMode(gg.PipelineModeRenderPass)
	}

	p.gpuReady = true
	log.Printf("gossamer/web: GPU ready (%s, %s, %dx%d @%gx)",
		adapter.Info().Name, p.format, p.pw, p.ph, p.w.dpr)
	p.w.Invalidate() // draw the first GPU frame now that we can present
}

// gpuSetupFailed logs and falls back to the CPU path — safe because the canvas
// has not been bound to a WebGPU surface yet at these failure points.
func (p *presenter) gpuSetupFailed(stage string, err error) {
	log.Printf("gossamer/web: gpu %s: %v; falling back to CPU", stage, err)
	p.initCPU()
	p.w.Invalidate()
}

// configure sizes and configures the surface to the current physical extent.
func (p *presenter) configure() {
	p.pw = int(float64(p.w.logical.W) * p.w.dpr)
	p.ph = int(float64(p.w.logical.H) * p.w.dpr)
	if err := p.surface.Configure(p.device, &wgpu.SurfaceConfiguration{
		Width:     uint32(p.pw),
		Height:    uint32(p.ph),
		Format:    p.format,
		Usage:     gputypes.TextureUsageRenderAttachment,
		AlphaMode: gputypes.CompositeAlphaModeOpaque,
	}); err != nil {
		log.Printf("gossamer/web: surface configure: %v", err)
	}
}

func (p *presenter) onResize() {
	if !p.gpuReady {
		return // CPU reallocates its ImageData lazily; GPU not up yet
	}
	p.configure()
	if err := p.ggc.Resize(int(p.w.logical.W), int(p.w.logical.H)); err != nil {
		log.Printf("gossamer/web: ggcanvas resize: %v", err)
	}
	p.ggc.SetDeviceScale(p.w.dpr)
}

// target returns this frame's presentation target: the GPU surface once ready,
// the CPU blit when that path is active, or a pending sentinel (skip) while GPU
// setup is still in flight.
func (p *presenter) target() shell.Target {
	switch {
	case p.gpuReady:
		return gpuTarget{p: p}
	case p.cpuActive:
		return shell.PixelTarget{Put: p.putCPU}
	default:
		return pendingTarget{} // GPU initializing; present nothing this frame
	}
}

// pendingTarget is neither a GPU nor pixel target, so paint.End presents
// nothing for it — the frames before the GPU device is ready.
type pendingTarget struct{}

// putCPU blits a CPU-rasterized frame via 2d putImageData.
func (p *presenter) putCPU(img *image.RGBA) {
	pw, ph := img.Rect.Dx(), img.Rect.Dy()
	if p.buf.IsUndefined() || p.bufW != pw || p.bufH != ph {
		p.buf = js.Global().Get("Uint8ClampedArray").New(len(img.Pix))
		p.imageData = js.Global().Get("ImageData").New(p.buf, pw, ph)
		p.bufW, p.bufH = pw, ph
	}
	js.CopyBytesToJS(p.buf, img.Pix)
	p.ctx2d.Call("putImageData", p.imageData, 0, 0)
}

// gpuTarget implements app.gpuCanvasTarget (RenderGPU).
type gpuTarget struct{ p *presenter }

// RenderGPU replays the scene onto the GPU canvas and presents the result to
// the canvas surface's current texture view directly.
func (t gpuTarget) RenderGPU(replay func(*gg.Context)) {
	p := t.p
	if err := p.ggc.Draw(func(cc *gg.Context) { replay(cc) }); err != nil {
		log.Printf("gossamer/web: gpu draw: %v", err)
		return
	}
	st, _, err := p.surface.GetCurrentTexture()
	if err != nil {
		log.Printf("gossamer/web: get current texture: %v", err)
		return
	}
	view, err := st.CreateView(nil)
	if err != nil {
		log.Printf("gossamer/web: surface view: %v", err)
		return
	}
	if err := p.ggc.RenderDirect(gpucontext.NewTextureView(unsafe.Pointer(view)), uint32(p.pw), uint32(p.ph)); err != nil {
		log.Printf("gossamer/web: render direct: %v", err)
	}
	_ = p.surface.Present(st) // no-op on browser (auto-presented)
}

// preferredCanvasFormat asks the browser which format composites without a
// conversion (bgra8unorm on most desktop GPUs).
func preferredCanvasFormat() gputypes.TextureFormat {
	gpu := js.Global().Get("navigator").Get("gpu")
	if gpu.IsUndefined() || gpu.IsNull() {
		return gputypes.TextureFormatBGRA8Unorm
	}
	switch gpu.Call("getPreferredCanvasFormat").String() {
	case "rgba8unorm":
		return gputypes.TextureFormatRGBA8Unorm
	default:
		return gputypes.TextureFormatBGRA8Unorm
	}
}

// webProvider adapts the WebGPU device/queue to gpucontext.DeviceProvider so
// gg's accelerator and ggcanvas share this frame's device.
type webProvider struct {
	device  *wgpu.Device
	queue   *wgpu.Queue
	adapter *wgpu.Adapter
	format  gputypes.TextureFormat
}

func (p *webProvider) Device() gpucontext.Device { return gpucontext.NewDevice(unsafe.Pointer(p.device)) }
func (p *webProvider) Queue() gpucontext.Queue   { return gpucontext.NewQueue(unsafe.Pointer(p.queue)) }
func (p *webProvider) SurfaceFormat() gputypes.TextureFormat { return p.format }
func (p *webProvider) Adapter() gpucontext.Adapter {
	return gpucontext.NewAdapter(unsafe.Pointer(p.adapter))
}

func (p *webProvider) AdapterInfo() gpucontext.AdapterInfo {
	info := p.adapter.Info()
	// A real WebGPU adapter — report a hardware type so gg keeps the GPU path
	// (software adapters make it prefer the CPU rasterizer).
	t := gpucontext.AdapterTypeIntegrated
	switch info.DeviceType {
	case gputypes.DeviceTypeDiscreteGPU:
		t = gpucontext.AdapterTypeDiscrete
	case gputypes.DeviceTypeCPU:
		t = gpucontext.AdapterTypeSoftware
	}
	return gpucontext.AdapterInfo{Name: info.Name, Type: t}
}
