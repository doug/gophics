# Tally — Android host

The M9 embedding model: the Go side (`examples/tally/ui`, bound through
`shell/mobile.Bridge`) owns the entire UI; this thin Kotlin host owns the
surface, vsync, touch, IME, and intents. Rendering runs on the GPU: the host
hands the bridge the `ANativeWindow` behind its `SurfaceView` (via the
`libgophics_surface` JNI shim) and the bridge presents each frame with wgpu.

The host is the CLI's stock template (`internal/cli/templates/mobile`) with
two departures kept on purpose — haptics played from the frame loop, and the
keyboard height reported apart from the system bars — plus the capability
hosts Tally actually uses: the file picker, the clipboard and device facts.
When the template changes, diff `MainActivity.kt` against it.

## Build & run

Tally is its own Go module, so run the CLI from `examples/tally`:

```sh
cd examples/tally
gophics run -p android .        # bind, assemble, install + launch
gophics build -p android .      # just the .aar, under build/android
./package/android.sh            # the same as run; --release for a release APK
```

gomobile cannot bind `package main`, so the CLI generates the bind surface
into `build/bind` from `ui.Root` and `ui.Config` and binds it together with
the shell bridge into `android/app/libs/tallymobile.aar`, which
`app/build.gradle` picks up by pattern. It ensures the SDK bits (NDK, CMake),
builds with the pinned Gradle wrapper (`./gradlew`, Gradle 8.9) under a JDK
17–21 (Android Studio's JBR is found automatically), and installs + launches.
Prereqs it does *not* install: the Android SDK (set `ANDROID_HOME` if not at
`~/Library/Android/sdk`) and `gomobile`
(`go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init`). Run
`gophics doctor` to check.

The GPU present path needs a **real device** — the emulator can't back a wgpu
surface (it falls back to CPU).

## Current state / known gaps

- Single-pointer touch; fling works (velocity is tracked in the shell),
  pinch/multi-touch pending.
- On-screen keyboard is summoned on field focus with the layout the field asks
  for (numeric for amounts); lifecycle pause/resume, dark-mode changes and
  safe-area insets are wired. Surface loss on rotation/background is the main
  unverified path (needs a device — see the bring-up checklist).
