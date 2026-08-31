# gophics Health — Android host

The Go side (`examples/health/mobile`, over `shell/mobile.Bridge`) owns the
entire dashboard UI; this thin Kotlin host owns the surface, vsync, and touch,
and feeds the UI **real on-device data read from Health Connect**. Rendering runs
on the GPU: the host hands the bridge the `ANativeWindow` behind its
`SurfaceView` (via the `libgophics_surface` JNI shim) and the bridge presents
each frame with wgpu.

`MainActivity` reads steps / heart rate / weight / sleep via the Health Connect
`connect-client` (after the user grants the permission sheet) and pushes each
series into the shared Go provider with the scalar `Healthmobile.pushSample`
(gomobile can't bind a `[]float64`, so there is no batch push). Metric codes:
0 heart rate, 1 steps, 2 weight, 3 sleep.

## Build & run

The `gophics` CLI drives the whole loop — bind the Go side, compile the APK
(including the native surface shim), install + launch on a connected device —
in one command from the repo root:

```sh
gophics run -p android ./examples/health/mobile
gophics build -p android ./examples/health/mobile   # just the .aar
```

It finds this host project (the sibling `android/` of the `mobile` package),
ensures the SDK bits (NDK, CMake), builds with the pinned Gradle wrapper under a
JDK 17–21, and installs + launches. Prereqs it does *not* install: the Android
SDK (set `ANDROID_HOME` if not at `~/Library/Android/sdk`) and `gomobile`
(`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`).

At first launch the app requests the Health Connect read permissions; grant them
in the system sheet and the dashboard fills with real data. A metric with no
source (no watch/scale/sleep-tracker feeding Health Connect) simply shows "no
data" — that is not an error. The GPU present path needs a **real device**; the
emulator falls back to CPU and lacks health data.
