# gossamer HN — Android host

The M9 embedding model: the Go side (`examples/hn/mobile`, over
`shell/mobile.Bridge`) owns the entire UI; this thin Kotlin host owns the
surface, vsync, touch, and intents, blitting the bridge's RGBA frames into
a `SurfaceView`.

## Build

```sh
# Prereqs: Android SDK + NDK (sdk/ndk/<version>), JDK 17, gradle.
go install golang.org/x/mobile/cmd/gomobile@latest
gomobile init

# From the repo root — builds the Go side into an .aar:
ANDROID_HOME=~/Library/Android/sdk gomobile bind -target=android -androidapi 24 \
  -o examples/hn/android/app/libs/hnmobile.aar ./examples/hn/mobile

# Then the app:
cd examples/hn/android
gradle :app:assembleDebug        # → app/build/outputs/apk/debug/app-debug.apk
adb install app/build/outputs/apk/debug/app-debug.apk
```

## Current state / known gaps

- Presentation is CPU pixels → `Bitmap` blit (the shell/web model). A
  GPU/ANativeWindow path can replace the blit inside the bridge later.
- Single-pointer touch; fling works (velocity is tracked in the shell),
  pinch/multi-touch pending.
- On-screen keyboard: not yet summoned on field focus (needs
  `InputMethodManager` + a focus signal from the bridge) — next M9 step,
  along with lifecycle pause/resume and safe-area insets.
