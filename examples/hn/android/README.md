# gossamer HN — Android host

The M9 embedding model: the Go side (`examples/hn/mobile`, over
`shell/mobile.Bridge`) owns the entire UI; this thin Kotlin host owns the
surface, vsync, touch, and intents. Rendering runs on the GPU: the host hands
the bridge the `ANativeWindow` behind its `SurfaceView` (via the
`libgossamer_surface` JNI shim) and the bridge presents each frame with wgpu.

## Build & run

One command builds the Go side, compiles the APK (including the native surface
shim), and installs + launches on a connected device/emulator:

```sh
./run.sh            # HN app
./run.sh --verify   # GPU bring-up scene (docs/mobile-gpu-bringup.md)
./run.sh --build    # build only, don't install/launch
```

It ensures the SDK bits it needs (NDK, CMake), builds with the pinned Gradle
wrapper (`./gradlew`, Gradle 8.9) under a JDK 17–21, and reports the native
libraries packaged into the APK. Prereqs it does *not* install: the Android SDK
(set `ANDROID_HOME` if it isn't at `~/Library/Android/sdk`), a JDK 17–21
(Android Studio's bundled JBR is picked up automatically), and `gomobile`
(`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`).

The GPU present path needs a **real device** — the emulator's SwiftShader/Vulkan
isn't a reliable signal. See `docs/mobile-gpu-bringup.md`.

## Current state / known gaps

- Single-pointer touch; fling works (velocity is tracked in the shell),
  pinch/multi-touch pending.
- On-screen keyboard is summoned on field focus; lifecycle pause/resume and
  safe-area insets are wired. Surface loss on rotation/background is the main
  unverified path (needs a device — see the bring-up checklist).
