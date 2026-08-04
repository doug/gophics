# Mobile GPU bring-up check

The mobile GPU present path (`shell/mobile` `SetSurface` + `mobileGPUTarget`,
Android `ANativeWindow` / iOS `CAMetalLayer`) was **built but never verified on a
real device** — the #1 risk in `docs/games-plan.md`. This is the checklist to
close that gap. It uses `examples/gpucheck`, a diagnostic scene designed so every
part of the GPU path is readable at a glance.

## Already verified (no device needed)

- **Compiles for both targets:** `GOOS=android GOARCH=arm64` and `GOOS=ios
  GOARCH=arm64` build `./shell/mobile` and `./examples/hn/mobile` clean.
- **The GPU pipeline composites the scene correctly on real hardware.** Rendering
  `gpucheck` through the Metal rasterizer on macOS (`go test -tags gossamer_gpu
  -run TestGPUReference ./examples/gpucheck`) produces the full scene — colors,
  gradient, crisp text, sprites (plain/tint/rotate), filled paths — with **no
  wipe**. Since the mobile surface uses the same `ggcanvas.RenderDirect` path, the
  *rendering* is very likely correct; what's unproven is the mobile-specific
  **surface handoff + lifecycle**.

## The backends were never linked (fixed 2026-08-03)

Everything below about "the simulator/emulator can't create a GPU surface" was a
**misdiagnosis**. `wgpu: no HAL instance available for surface creation` does not
mean the host lacks a GPU — it is what wgpu says when *no HAL backend is
registered at all*, and `core.Instance` reaches that state silently because it
`continue`s past every failed/absent backend without logging why.

wgpu only links the backends you import (each backend's `init` calls
`hal.RegisterBackend`). The desktop shell gets them transitively through gogpu's
renderer, which the mobile build never pulls in — so **the iOS and Android
binaries contained zero backends** and every device was guaranteed to fall back
to the CPU blit. `shell/mobile/backends.go` now imports `wgpu/hal/allbackends`.

Verified on a **Pixel 10 Pro** (Android 17, Tensor G5 / Imagination PowerVR
D-Series DXT-48-1536, 1080x2238 @2.625x): Vulkan comes up and the scene renders
on the GPU at **~4-5 ms/frame**, against ~117 ms for the CPU blit it was
silently using before.

Two further device-only fixes fell out of that run:

- **Surface format.** `gpu.go` hardcoded `BGRA8Unorm`; PowerVR's swapchain only
  offers `RGBA8Unorm`, so `Configure` failed and every frame died on "surface is
  not configured" — GPU reported ready, screen stayed blank. `negotiateSurface`
  now picks from `GetSurfaceCapabilities` (format, alpha mode, present mode), and
  a failed configure aborts to the CPU path instead of presenting nothing.
- **16 KB pages.** Pixel 10 reports `ro.product.cpu.pagesize.max=16384`; the
  4 KB-aligned `libgojni.so`/`libgossamer_surface.so` triggered Android's
  PageSizeMismatch "app isn't compatible" dialog. Both are now linked with
  `-Wl,-z,max-page-size=16384`. Note this is *ELF segment* alignment — APK
  `zipalign -P 16` passed the whole time, so that check proves nothing here.

Caveat: `allbackends` registers Vulkan on **android/arm64 only** (upstream's
preview contract), so the x86_64 emulator still has no backend and stays on the
CPU blit by design.

## Simulator / emulator results (2026-08-02, superseded)

The simulator/emulator **can't create a GPU surface** — as the games-plan
predicted — but they now render the app via a **CPU-blit present fallback**, so
they're first-class for day-to-day development. `Bridge.GPUActive()` reports
whether the GPU surface is live; when it's false the host presents each frame
with `Bridge.Snapshot()` (the same parity-tested rasterizer, `app/gpu_equiv_test.go`)
and blits the pixels. GPU on device, CPU everywhere else.

- **iOS Simulator** (iPhone 17 Pro, Apple Silicon): `gossamer run -p ios -tags
  gossamer_verify ./examples/hn/mobile` binds, builds, installs, and launches;
  `GPUActive()` is false
  (`wgpu: no HAL instance available for surface creation` — the Simulator's Metal
  lacks the HAL wgpu needs), so the host blits `Snapshot()` into a `CALayer`.
  **Verified:** the gpucheck scene renders fully — title, animating frame
  counter, correct swatches, gradient, three text sizes, sprite trio
  (plain/tint/rotate), triangle + spinning square. Not black.
- **Android emulator** (arm64 `google_apis`, `-gpu host` → host M1 Ultra Metal):
  `gossamer run -p android -tags gossamer_verify ./examples/hn/mobile` packages the JNI surface shim
  (`libgossamer_surface.so`) — the earlier `UnsatisfiedLinkError` crash was a
  missing `externalNativeBuild`/CMake wiring, now fixed — so the app launches.
  `GPUActive()` is false (even with `-gpu host`, wgpu can't back a Vulkan
  surface: `no HAL instance available`), so the host blits via `lockCanvas`.
  **Verified:** the gpucheck scene renders fully and animates, and taps register
  (marker + tap counter). It's slow (~90 ms/frame — full-screen CPU raster +
  blit under emulation), fine for layout/logic/touch work.

**Net:** every dev can run the app in the simulator/emulator with one command
and see a faithful, interactive UI. What's still **device-only** is validating
the *GPU present path itself* — the emulator's SwiftShader/Vulkan isn't a
trustworthy GPU signal, and the iOS Simulator can't create a GPU surface at all.

## What only a device can confirm

Surface creation from a native handle, **rotation** (Android destroys the surface
on every rotation), **background/foreground** (surface loss), the **Vulkan-Android
"preview"** backend holding up, and that content fills the screen at the **right
scale** (the headless GPU render placed content at ~1× in a 2× buffer — watch for
this on device).

## How to run it

The gpucheck scene is selected by the `gossamer_verify` build tag (see
`examples/hn/mobile/scene.go`); the `gossamer` CLI drives bind → build → install
→ launch in one command:

- **Android:** `gossamer run -p android -tags gossamer_verify ./examples/hn/mobile`
- **iOS:** `gossamer run -p ios -tags gossamer_verify ./examples/hn/mobile`
  (Simulator; for a device, build with `-sdk iphoneos` + a signing team, or open
  the project in Xcode)

The CLI finds the sibling `ios/`/`android/` host project, binds into it, ensures
the SDK bits (Android NDK/CMake), picks a simulator/booted device, and launches.
On the Simulator/emulator the scene renders via the CPU fallback; on a real
device it renders on the GPU — the path the checklist below validates.

## The checklist

On a real device, **first confirm you're on the GPU path, not the CPU fallback**
— check the host log for `GPU present` (Android) / a `GPU ready` line
(`gossamer/mobile`); if it says GPU is unavailable you're validating the CPU
blit, not the surface. Then compare the device screen against the desktop render
of the same scene
(`GPUCHECK_SHOT=out.png go test -run TestGPUCheckRenders ./examples/gpucheck`).

| # | Check | Pass = | A failure implies |
|---|---|---|---|
| 1 | Surface renders | you see the scene, not black | surface handoff / `SetSurface` |
| 2 | Text is crisp and present | "GPU CHECK" + fox lines legible | the LoadOpClear wipe (text composited before clear) |
| 3 | Colors correct | red/green/blue/white swatches + cyan→pink gradient | color space / format mismatch |
| 4 | Sprites render | three squares: plain, tinted, rotated | `DrawSprite` on the direct surface |
| 5 | Animation runs | frame counter rising, square spinning | per-frame present / `RenderFrame` loop |
| 6 | Fills the screen at correct scale | content spans the full width, sharp | DPR/scale handling in `SetSurface` |
| 7 | Touch works | tap → yellow marker + tap count bumps | touch delivery (`Bridge.Touch` → `SourceTouch`) |
| 8 | **Rotation** | rotate device → scene still renders correctly | surface recreation (Android destroys on rotate) |
| 9 | **Background/foreground** | home, then reopen → renders, no crash/black | surface `Outdated`/`Lost` reconfigure-and-retry |
| 10 | Stable | ~1 min of tapping/rotating, no crash | Vulkan-Android preview robustness |

Checks 8–10 are the real unknowns. If 1–7 pass but 8/9 fail, the fix is
bind/unbind on the surface-lifecycle callbacks; if the surface is corrupt from the
start on Android, the Vulkan preview backend needs upstream work (re-scope the
mobile action tier, per the games-plan risk register).

## Device results — Pixel 10 Pro, 2026-08-03

First real-hardware run of the GPU path. Compared against
`GPUCHECK_SHOT=... go test -run TestGPUCheckRenders ./examples/gpucheck`.

| # | Check | Result |
|---|---|---|
| 1 | Surface renders | **pass** — scene visible, not black |
| 2 | Text crisp | **FAIL** — every string renders as blurred solid blocks, fully illegible |
| 3 | Colors correct | **pass** — swatches and cyan→pink gradient match the reference |
| 4 | Sprites render | **FAIL** — plain + tint draw; the **rotated sprite is missing** |
| 5 | Animation runs | **pass** — spinning path animates, frame timings advance |
| 6 | Fills screen at right scale | **pass** — layout matches the reference proportionally at 2.625x |
| 7 | Touch works | **pass** — tap marker appears at the touch point |

Checks 8–10 (rotation, background/foreground, stability) are **not yet run on the
GPU path** — the earlier lifecycle exercise happened while the app was on the CPU
fallback, so it validated the fallback, not the surface handoff.

**Open GPU-backend bugs this surfaced** (both in gg's GPU rendering, not in the
mobile surface plumbing — the surface, colors, gradients, paths, scale and touch
are all correct):

1. **Text renders as blocks.** Glyph/MSDF output is wrong on the Vulkan backend.
   This is the blocker for calling the mobile GPU path usable.
2. **Rotated sprites drop out.** A rotated *path* draws fine, so it is specific
   to `DrawSprite` under rotation.

Neither reproduces through the Metal reference render, so they need debugging
against Vulkan specifically.
