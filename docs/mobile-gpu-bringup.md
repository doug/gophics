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

## Simulator / emulator results (2026-08-02)

The simulator/emulator **can't create a GPU surface** — as the games-plan
predicted — but they now render the app via a **CPU-blit present fallback**, so
they're first-class for day-to-day development. `Bridge.GPUActive()` reports
whether the GPU surface is live; when it's false the host presents each frame
with `Bridge.Snapshot()` (the same parity-tested rasterizer, `app/gpu_equiv_test.go`)
and blits the pixels. GPU on device, CPU everywhere else.

- **iOS Simulator** (iPhone 17 Pro, Apple Silicon): `./examples/hn/ios/run.sh
  --verify` binds, builds, installs, and launches; `GPUActive()` is false
  (`wgpu: no HAL instance available for surface creation` — the Simulator's Metal
  lacks the HAL wgpu needs), so the host blits `Snapshot()` into a `CALayer`.
  **Verified:** the gpucheck scene renders fully — title, animating frame
  counter, correct swatches, gradient, three text sizes, sprite trio
  (plain/tint/rotate), triangle + spinning square. Not black.
- **Android emulator** (arm64 `google_apis`, `-gpu host` → host M1 Ultra Metal):
  `./examples/hn/android/run.sh --verify` packages the JNI surface shim
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

`StartVerify()` is wired into `examples/hn/mobile` (it builds the gpucheck scene
instead of HN), exposed via each host's `--verify` flag. One command each:

- **Android:** `examples/hn/android/run.sh --verify` (device or emulator)
- **iOS:** `examples/hn/ios/run.sh --verify` (Simulator; for a device, build
  with `-sdk iphoneos` + a signing team, or open the project in Xcode)

Both bind the Go side, build, install, and launch. On the Simulator/emulator the
scene renders via the CPU fallback; on a real device it renders on the GPU —
which is the path the checklist below validates.

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
