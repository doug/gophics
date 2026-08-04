package paint

import "github.com/doug/gossamer/internal/gfx/gg"

// The GPU accelerator (gg's SDF shapes + tiled/adaptive coverage) is linked and
// process-globally registered by accel_link.go's blank import, unless the
// binary is built with -tags nogpu. Registration silently no-ops when the
// machine has no GPU, so GPUAvailable reports the real outcome.
//
// gossamer keeps its offscreen/CPU Painter contexts opted out of the global
// accelerator per-context (see begin()'s SetGPUDisabled), so a registered
// accelerator never perturbs the deterministic CPU raster path — only the
// shell's on-screen GPU present path uses it.

// GPUAvailable reports whether a GPU accelerator is currently registered (i.e.
// the GPU path is usable). It is false under -tags nogpu, or when GPU device
// acquisition failed at startup, or after UseCPU.
func GPUAvailable() bool { return gg.Accelerator() != nil }

// UseCPU drops the registered GPU accelerator so all rasterization stays on the
// CPU. Call it once, before rendering, when the resolved renderer is CPU. It is
// idempotent and safe when no accelerator is registered.
func UseCPU() { gg.CloseAccelerator() }
