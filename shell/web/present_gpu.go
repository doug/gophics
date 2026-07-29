//go:build js && wasm && gossamer_gpu

// The GPU web build rasterizes each frame on the GPU via gg's accelerator and
// presents to the canvas's WebGPU surface directly — no CPU readback, no
// putImageData. This lifts the CPU-rasterizer + CopyBytesToJS ceiling that
// makes the 2D-blit path (present_cpu.go) unbearably slow at retina.
//
// WebGPU device acquisition is asynchronous (navigator.gpu returns Promises,
// which the portable wgpu browser backend awaits by parking a goroutine). So
// setup runs on its own goroutine; until it completes, frames are skipped
// (the target's RenderGPU no-ops) rather than falling back to the CPU.
package web

import (
	"log"
	"syscall/js"
	"unsafe"

	"github.com/gogpu/gg"
	_ "github.com/gogpu/gg/gpu" // registers gg's GPU accelerator (SDF/tiled raster)
	"github.com/gogpu/gg/integration/ggcanvas"
	"github.com/gogpu/gpucontext"
	"github.com/gogpu/gputypes"
	"github.com/gogpu/wgpu"

	"github.com/doug/gossamer/shell"
)

// presenter owns the WebGPU device, the canvas surface, and the gg GPU canvas.
type presenter struct {
	w *window

	device  *wgpu.Device
	surface *wgpu.Surface
	ggc     *ggcanvas.Canvas
	format  gputypes.TextureFormat

	ready  bool // set once setup completes on the GPU goroutine
	pw, ph int  // configured physical pixel size
}

func newPresenter(w *window) *presenter {
	p := &presenter{w: w}
	// Device acquisition awaits JS Promises, which deadlocks on the main
	// goroutine; run it on its own goroutine and Invalidate when ready.
	go p.setup()
	return p
}

func (p *presenter) setup() {
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsPrimary})
	if err != nil {
		log.Printf("gossamer/web: gpu instance: %v", err)
		return
	}
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference: gputypes.PowerPreferenceHighPerformance,
	})
	if err != nil {
		log.Printf("gossamer/web: gpu adapter: %v", err)
		return
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		log.Printf("gossamer/web: gpu device: %v", err)
		return
	}
	surface, err := inst.CreateSurfaceFromCanvas(p.w.canvas)
	if err != nil {
		log.Printf("gossamer/web: gpu surface: %v", err)
		return
	}

	p.device = device
	p.surface = surface
	p.format = preferredCanvasFormat()
	p.configure()

	provider := &webProvider{
		device:  device,
		queue:   device.Queue(),
		adapter: adapter,
		format:  p.format,
	}
	// gg renders in logical points; the canvas is physical-sized with a
	// device scale so 1 point maps to dpr device pixels.
	c, err := ggcanvas.NewWithScale(provider, int(p.w.logical.W), int(p.w.logical.H), p.w.dpr)
	if err != nil {
		log.Printf("gossamer/web: ggcanvas: %v", err)
		return
	}
	p.ggc = c
	p.ready = true
	p.w.Invalidate() // draw the first GPU frame now that we can present
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
	if !p.ready {
		return
	}
	p.configure()
	if err := p.ggc.Resize(int(p.w.logical.W), int(p.w.logical.H)); err != nil {
		log.Printf("gossamer/web: ggcanvas resize: %v", err)
	}
	p.ggc.SetDeviceScale(p.w.dpr)
}

// target always returns a gpuTarget so the app layer takes the GPU present
// path (app.gpuCanvasTarget); RenderGPU skips frames until setup completes.
func (p *presenter) target() shell.Target { return gpuTarget{p: p} }

// gpuTarget implements app.gpuCanvasTarget (RenderGPU).
type gpuTarget struct{ p *presenter }

// RenderGPU replays the scene onto the GPU canvas and presents the result to
// the canvas surface's current texture view directly.
func (t gpuTarget) RenderGPU(replay func(*gg.Context)) {
	p := t.p
	if !p.ready {
		return // GPU not initialized yet; skip this frame
	}
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

func (p *webProvider) Device() gpucontext.Device {
	return gpucontext.NewDevice(unsafe.Pointer(p.device))
}

func (p *webProvider) Queue() gpucontext.Queue {
	return gpucontext.NewQueue(unsafe.Pointer(p.queue))
}

func (p *webProvider) SurfaceFormat() gputypes.TextureFormat { return p.format }

func (p *webProvider) Adapter() gpucontext.Adapter {
	return gpucontext.NewAdapter(unsafe.Pointer(p.adapter))
}

func (p *webProvider) AdapterInfo() gpucontext.AdapterInfo {
	info := p.adapter.Info()
	// A real WebGPU adapter — report a hardware type so gg keeps the GPU
	// path (software adapters make it prefer the CPU rasterizer).
	t := gpucontext.AdapterTypeIntegrated
	switch info.DeviceType {
	case gputypes.DeviceTypeDiscreteGPU:
		t = gpucontext.AdapterTypeDiscrete
	case gputypes.DeviceTypeCPU:
		t = gpucontext.AdapterTypeSoftware
	}
	return gpucontext.AdapterInfo{Name: info.Name, Type: t}
}
