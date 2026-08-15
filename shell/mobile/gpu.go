package mobile

import (
	"log"
	"slices"
	"unsafe"

	"github.com/doug/gophics/internal/gfx/gg"
	"github.com/doug/gophics/internal/gfx/gg/integration/ggcanvas"
	"github.com/doug/gophics/internal/gfx/gpucontext"
	"github.com/doug/gophics/internal/gfx/gputypes"
	"github.com/doug/gophics/internal/gfx/wgpu"
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
	alpha   gputypes.CompositeAlphaMode
	present gputypes.PresentMode
	pw, ph  int
	scale   float64
}

// SetSurface gives the Bridge the native render surface to present to.
// displayHandle/windowHandle are the platform's raw handles as int64 (iOS: 0,
// CAMetalLayer*; Android: 0, ANativeWindow*); widthPx/heightPx are the surface's
// physical size and scale its density. Safe to call again after a
// rotation/resize (it rebuilds). On failure the surface is left unset
// (GPUActive reports false) and RenderFrame no-ops; the host should then
// present with the CPU path (Snapshot + blit — see GPUActive).
func (b *Bridge) SetSurface(displayHandle, windowHandle int64, widthPx, heightPx int, scale float32) {
	b.ClearSurface()
	b.dispHandle, b.winHandle = displayHandle, windowHandle
	if scale <= 0 {
		scale = 1
	}
	g, err := newMobileGPU(uintptr(displayHandle), uintptr(windowHandle), widthPx, heightPx, float64(scale))
	if err != nil {
		// GPU surface unavailable (e.g. the iOS Simulator, whose Metal doesn't
		// expose the HAL wgpu needs): GPUActive stays false so the host falls
		// back to the CPU present path. RenderFrame no-ops in the meantime.
		log.Printf("gophics/mobile: GPU surface unavailable, host should use CPU present (GPUActive=false): %v", err)
		return
	}
	b.gpu = g
	b.dirty.Store(true)
}

// GPUActive reports whether a GPU render surface is live. When false — no
// surface set, or surface creation failed (iOS Simulator, some emulators) —
// the host should present with the CPU path instead: call Snapshot each frame
// and blit the returned pixels. GPU on device, CPU everywhere else, both from
// the same (parity-tested) rasterizer.
func (b *Bridge) GPUActive() bool { return b.gpu != nil }

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

// alignSurface rounds physical surface dimensions down to a multiple of 8.
// Imagination PowerVR's MSAA resolve shears the frame when the surface width is
// not 8-aligned (verified on a Pixel 10 Pro: portrait 1080 — 8-aligned — is
// clean, landscape 2238 — not — tears into a horizontal shear). The Vulkan spec
// imposes no such requirement, so this is a driver quirk; rounding down costs at
// most 7px at the screen edge (imperceptible) and keeps the swapchain, MSAA
// color, resolve target, and viewport all matched at an aligned size.
func alignSurface(px int) int { return px &^ 7 }

func newMobileGPU(display, window uintptr, wPx, hPx int, scale float64) (*mobileGPU, error) {
	wPx, hPx = alignSurface(wPx), alignSurface(hPx)
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
	// Request the adapter's own limits rather than the spec defaults. Several
	// compute passes in the vector renderer bind more storage buffers per stage
	// than the default 8 allows (vello_coarse binds 9), and with default limits
	// those pipelines fail to create — which surfaces as a wall of validation
	// errors and a silently degraded renderer. Asking for exactly what the
	// adapter reports is always valid; asking for more than it reports is not.
	device, err := adapter.RequestDevice(&wgpu.DeviceDescriptor{
		RequiredLimits: adapter.Limits(),
	})
	if err != nil {
		return nil, err
	}
	format, alpha, present := negotiateSurface(adapter, surface)
	g := &mobileGPU{
		device:  device,
		surface: surface,
		format:  format,
		alpha:   alpha,
		present: present,
		pw:      wPx,
		ph:      hPx,
		scale:   scale,
	}
	// A surface that fails to configure never yields a texture, so every frame
	// would die in RenderGPU with "surface is not configured" and the host would
	// show nothing at all. Fail here instead so GPUActive stays false and the
	// host presents the CPU blit — a slow picture beats a blank screen.
	if err := g.configure(); err != nil {
		return nil, err
	}

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
	log.Printf("gophics/mobile: GPU ready (%s, %dx%d @%gx, %v)",
		adapter.Info().Name, wPx, hPx, scale, g.format)
	return g, nil
}

func logicalDim(px int, scale float64) int {
	if scale <= 0 {
		return px
	}
	return int(float64(px) / scale)
}

// negotiateSurface picks a format/alpha/present mode the adapter actually
// reports for this surface. Hardcoding BGRA8Unorm + Opaque works on Metal and
// desktop Vulkan, but not universally: the Pixel 10's PowerVR swapchain offers
// RGBA8Unorm only, and configuring an unsupported format leaves the surface
// unconfigured — the GPU reports ready and then no frame ever presents.
// Preferences come first so we keep matching the desktop path where we can.
func negotiateSurface(adapter *wgpu.Adapter, surface *wgpu.Surface) (
	gputypes.TextureFormat, gputypes.CompositeAlphaMode, gputypes.PresentMode,
) {
	format, alpha := gputypes.TextureFormatBGRA8Unorm, gputypes.CompositeAlphaModeOpaque
	present := gputypes.PresentModeFifo
	caps := adapter.GetSurfaceCapabilities(surface)
	if caps == nil {
		return format, alpha, present
	}
	if len(caps.Formats) > 0 {
		format = preferred(caps.Formats,
			gputypes.TextureFormatBGRA8Unorm, gputypes.TextureFormatRGBA8Unorm)
	}
	if len(caps.AlphaModes) > 0 {
		alpha = preferred(caps.AlphaModes,
			gputypes.CompositeAlphaModeOpaque, gputypes.CompositeAlphaModeInherit)
	}
	if len(caps.PresentModes) > 0 {
		// Fifo is the only mode Vulkan guarantees, so it stays first choice.
		present = preferred(caps.PresentModes, gputypes.PresentModeFifo)
	}
	return format, alpha, present
}

// preferred returns the first pref that avail contains, else avail's first
// entry. avail must be non-empty.
func preferred[T comparable](avail []T, prefs ...T) T {
	for _, p := range prefs {
		if slices.Contains(avail, p) {
			return p
		}
	}
	return avail[0]
}

func (g *mobileGPU) configure() error {
	err := g.surface.Configure(g.device, &wgpu.SurfaceConfiguration{
		Width:       uint32(g.pw),
		Height:      uint32(g.ph),
		Format:      g.format,
		Usage:       gputypes.TextureUsageRenderAttachment,
		AlphaMode:   g.alpha,
		PresentMode: g.present,
	})
	if err != nil {
		log.Printf("gophics/mobile: surface configure: %v", err)
	}
	return err
}

// orientationChanged reports whether the incoming (physical) size flips the
// surface orientation vs the current one — i.e. a device rotation. The Bridge
// full-rebuilds on this rather than resizing in place (see Bridge.Resize).
func (g *mobileGPU) orientationChanged(wPx, hPx int) bool {
	wPx, hPx = alignSurface(wPx), alignSurface(hPx)
	return (wPx > hPx) != (g.pw > g.ph)
}

func (g *mobileGPU) resize(wPx, hPx int, scale float64) {
	if scale <= 0 {
		scale = g.scale
	}
	wPx, hPx = alignSurface(wPx), alignSurface(hPx)
	g.pw, g.ph, g.scale = wPx, hPx, scale
	_ = g.configure()
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
		log.Printf("gophics/mobile: gpu draw: %v", err)
		return
	}
	st, _, err := g.surface.GetCurrentTexture()
	if err != nil {
		log.Printf("gophics/mobile: get current texture: %v", err)
		return
	}
	view, err := st.CreateView(nil)
	if err != nil {
		log.Printf("gophics/mobile: surface view: %v", err)
		return
	}
	if err := g.ggc.RenderDirect(gpucontext.NewTextureView(unsafe.Pointer(view)), uint32(g.pw), uint32(g.ph)); err != nil {
		log.Printf("gophics/mobile: render direct: %v", err)
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
