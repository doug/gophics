# gophics news — Android host

The M9 embedding model: the Go side (`examples/news/mobile`, over
`shell/mobile.Bridge`) owns the entire UI; this thin Kotlin host owns the
surface, vsync, touch, and intents. Rendering runs on the GPU: the host hands
the bridge the `ANativeWindow` behind its `SurfaceView` (via the
`libgophics_surface` JNI shim) and the bridge presents each frame with wgpu.

## Build & run

The `gophics` CLI drives the whole loop — bind the Go side, compile the APK
(including the native surface shim), install + launch on a connected
device/emulator — in one command from the repo root:

```sh
gophics run -p android ./examples/news/mobile
gophics run -p android -tags gophics_verify ./examples/news/mobile  # GPU bring-up scene
gophics build -p android ./examples/news/mobile                       # just the .aar
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

## What this host does that the others do not

**Hands Go a data directory.** `MainActivity` calls `Newsmobile.setDataDir(filesDir.absolutePath)`
before `start()`. The reader keeps articles, subscriptions, the ranking model
and its picture cache on disk, and only the Java side knows where an app may
write — `os.UserConfigDir` on Android does not point anywhere usable.

**Presents the sign-in web view.** Publishers that gate their articles need the
subscriber's own session cookie. gophics cannot supply that: `shell.WebView` is
implemented for the web shell only and exposes no cookie access. So the Go side
raises a request, this host polls it on the frame loop it already runs, shows a
real `WebView` (`LoginSheet.kt`), and hands the session back:

```kotlin
val domain = Newsmobile.pendingLoginDomain()
if (domain.isNotEmpty()) {
    Newsmobile.clearPendingLogin()
    LoginSheet.show(activity, domain, Newsmobile.pendingLoginURL())
}
```

`LoginSheet` reads every cookie the browser holds for that site out of
`CookieManager` and passes them to `Newsmobile.setCookies`. Which cookie carries
the session is undocumented and changes, so none are hand-picked; cookies for
other domains are rejected on the Go side by domain mismatch.
