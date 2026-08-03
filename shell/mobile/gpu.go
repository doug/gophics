package mobile

import (
	"log"
	"unsafe"

	"github.com/doug/gg"
	"github.com/doug/gg/integration/ggcanvas"
	"github.com/doug/gpucontext"
	"github.com/doug/gputypes"
	"github.com/doug/wgpu"
)

// GPU presentation for the mobile Bridge. The host hands over a native render
// surface via SetSurface — an iOS CAMetalLayer or an Android ANativeWindow — and
// the Bridge rasterizes each frame on the GPU straight to it (gg's accelerator →
// ggcanvas.RenderDirect), the same model as the web and desktop GPU present.
// This is the only live-rendering path (Snapshot renders offscreen on the CPU
// for tests/screenshots).

// mobileGPU owns the wgpu device, the host surface, and the gg GPU canvas.
type mobileGPU struct {
	device  *wgpu.Device
	surface *wgpu.Surface
	ggc     *ggcanvas.Canvas
	format  gputypes.TextureFormat
	pw, ph  int
	scale   float64
}

// SetSurface gives the Bridge the native render surface to present to.
// displayHandle/windowHandle are the platform's raw handles as int64 (iOS: 0,
// CAMetalLayer*; Android: 0, ANativeWindow*); widthPx/heightPx are the surface's
// physical size and scale its density. Safe to call again after a
// rotation/resize (it rebuilds). On failure the surface is left unset and
// RenderFrame no-ops until a surface is provided — there is no CPU host
// fallback for live rendering (Snapshot stays available for offscreen).
func (b *Bridge) SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float32) {
	b.ClearSurface()
	if scale <= 0 {
		scale = 1
	}
	g, err := newMobileGPU(uintptr(displayHandle), uintptr(windowHandle), widthPx, heightPx, float64(scale))
	if err != nil {
		// No CPU-blit fallback exists for live rendering, so the surface stays
		// blank until a valid one is provided — RenderFrame no-ops. (Seen on the
		// iOS Simulator, whose Metal doesn't expose the HAL wgpu needs.)
		log.Printf("gossamer/mobile: GPU surface creation failed; live rendering disabled until a valid surface is provided: %v", err)
		return
	}
	b.gpu = g
	b.dirty.Store(true)
}

// ClearSurface tears down the GPU surface (call when the host surface is
// destroyed — backgrounding, rotation — before handing over a new one).
// RenderFrame no-ops until SetSurface is called again.
func (b *Bridge) ClearSurface() {
	if b.gpu != nil {
		b.gpu.release()
		b.gpu = nil
		b.dirty.Store(true)
	}
}

func newMobileGPU(display, window uintptr, wPx, hPx int, scale float64) (*mobileGPU, error) {
	inst, err := wgpu.CreateInstance(&wgpu.InstanceDescriptor{Backends: gputypes.BackendsPrimary})
	if err != nil {
		return nil, err
	}
	surface, err := inst.CreateSurface(display, window)
	if err != nil {
		return nil, err
	}
	adapter, err := inst.RequestAdapter(&wgpu.RequestAdapterOptions{
		PowerPreference:   gputypes.PowerPreferenceHighPerformance,
		CompatibleSurface: surface,
	})
	if err != nil {
		return nil, err
	}
	device, err := adapter.RequestDevice(nil)
	if err != nil {
		return nil, err
	}
	g := &mobileGPU{
		device:  device,
		surface: surface,
		format:  gputypes.TextureFormatBGRA8Unorm,
		pw:      wPx,
		ph:      hPx,
		scale:   scale,
	}
	g.configure()

	provider := &mobileProvider{device: device, queue: device.Queue(), adapter: adapter, format: g.format}
	// gg renders in logical points; the surface is physical-sized with a device
	// scale so 1 point maps to `scale` device pixels.
	lw, lh := logicalDim(wPx, scale), logicalDim(hPx, scale)
	c, err := ggcanvas.NewWithScale(provider, lw, lh, scale)
	if err != nil {
		return nil, err
	}
	g.ggc = c
	// Force the render-pass pipeline (the compute rasterizer reads back to a CPU
	// pixmap and never reaches the surface — the same fix as the web GPU path).
	if pma, ok := gg.Accelerator().(gg.PipelineModeAware); ok {
		pma.SetPipelineMode(gg.PipelineModeRenderPass)
	}
	log.Printf("gossamer/mobile: GPU ready (%s, %dx%d @%gx)", adapter.Info().Name, wPx, hPx, scale)
	return g, nil
}

func logicalDim(px int, scale float64) int {
	if scale <= 0 {
		return px
	}
	return int(float64(px) / scale)
}

func (g *mobileGPU) configure() {
	if err := g.surface.Configure(g.device, &wgpu.SurfaceConfiguration{
		Width:       uint32(g.pw),
		Height:      uint32(g.ph),
		Format:      g.format,
		Usage:       gputypes.TextureUsageRenderAttachment,
		AlphaMode:   gputypes.CompositeAlphaModeOpaque,
		PresentMode: gputypes.PresentModeFifo,
	}); err != nil {
		log.Printf("gossamer/mobile: surface configure: %v", err)
	}
}

func (g *mobileGPU) resize(wPx, hPx int, scale float64) {
	if scale <= 0 {
		scale = g.scale
	}
	g.pw, g.ph, g.scale = wPx, hPx, scale
	g.configure()
	if g.ggc != nil {
		_ = g.ggc.Resize(logicalDim(wPx, scale), logicalDim(hPx, scale))
		g.ggc.SetDeviceScale(scale)
	}
}

func (g *mobileGPU) release() {
	if g.surface != nil {
		g.surface.Release()
	}
}

// mobileGPUTarget implements app.gpuCanvasTarget (RenderGPU): replay the scene
// onto the GPU canvas and present to the surface's current texture.
type mobileGPUTarget struct{ g *mobileGPU }

func (t mobileGPUTarget) RenderGPU(replay func(*gg.Context)) {
	g := t.g
	if err := g.ggc.Draw(func(cc *gg.Context) { replay(cc) }); err != nil {
		log.Printf("gossamer/mobile: gpu draw: %v", err)
		return
	}
	st, _, err := g.surface.GetCurrentTexture()
	if err != nil {
		log.Printf("gossamer/mobile: get current texture: %v", err)
		return
	}
	view, err := st.CreateView(nil)
	if err != nil {
		log.Printf("gossamer/mobile: surface view: %v", err)
		return
	}
	if err := g.ggc.RenderDirect(gpucontext.NewTextureView(unsafe.Pointer(view)), uint32(g.pw), uint32(g.ph)); err != nil {
		log.Printf("gossamer/mobile: render direct: %v", err)
	}
	_ = g.surface.Present(st)
}

// mobileProvider adapts the wgpu device/queue to gpucontext.DeviceProvider so
// gg's accelerator and ggcanvas share this surface's device.
type mobileProvider struct {
	device  *wgpu.Device
	queue   *wgpu.Queue
	adapter *wgpu.Adapter
	format  gputypes.TextureFormat
}

func (p *mobileProvider) Device() gpucontext.Device {
	return gpucontext.NewDevice(unsafe.Pointer(p.device))
}
func (p *mobileProvider) Queue() gpucontext.Queue {
	return gpucontext.NewQueue(unsafe.Pointer(p.queue))
}
func (p *mobileProvider) SurfaceFormat() gputypes.TextureFormat { return p.format }
func (p *mobileProvider) Adapter() gpucontext.Adapter {
	return gpucontext.NewAdapter(unsafe.Pointer(p.adapter))
}

func (p *mobileProvider) AdapterInfo() gpucontext.AdapterInfo {
	info := p.adapter.Info()
	t := gpucontext.AdapterTypeIntegrated
	switch info.DeviceType {
	case gputypes.DeviceTypeDiscreteGPU:
		t = gpucontext.AdapterTypeDiscrete
	case gputypes.DeviceTypeCPU:
		t = gpucontext.AdapterTypeSoftware
	}
	return gpucontext.AdapterInfo{Name: info.Name, Type: t}
}
