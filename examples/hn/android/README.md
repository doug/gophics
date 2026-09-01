# gophics HN — Android host

The M9 embedding model: the Go side (`examples/hn/mobile`, over
`shell/mobile.Bridge`) owns the entire UI; this thin Kotlin host owns the
surface, vsync, touch, and intents. Rendering runs on the GPU: the host hands
the bridge the `ANativeWindow` behind its `SurfaceView` (via the
`libgophics_surface` JNI shim) and the bridge presents each frame with wgpu.

## Build & run

The `gophics` CLI drives the whole loop — bind the Go side, compile the APK
(including the native surface shim), install + launch on a connected
device/emulator — in one command from the repo root:

```sh
gophics run -p android ./examples/hn/mobile
gophics run -p android -host . ./examples/gpucheck/mobile  # GPU bring-up scene
gophics build -p android ./examples/hn/mobile                       # just the .aar
```

It finds this host project (the sibling `android/` of the `mobile` package),
ensures the SDK bits (NDK, CMake), builds with the pinned Gradle wrapper
(`./gradlew`, Gradle 8.9) under a JDK 17–21 (Android Studio's JBR is found
automatically), and installs + launches. Prereqs it does *not* install: the
Android SDK (set `ANDROID_HOME` if not at `~/Library/Android/sdk`) and `gomobile`
(`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`). Run
`gophics doctor` to check.

The GPU present path needs a **real device** — the emulator can't back a wgpu
surface (it falls back to CPU).

## Current state / known gaps

- Single-pointer touch; fling works (velocity is tracked in the shell),
  pinch/multi-touch pending.
- On-screen keyboard is summoned on field focus; lifecycle pause/resume and
  safe-area insets are wired. Surface loss on rotation/background is the main
  unverified path (needs a device — see the bring-up checklist).
