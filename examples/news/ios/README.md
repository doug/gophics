# gophics news — iOS host

The same M9 embedding model as Android: `shell/mobile.Bridge` on the Go
side, a thin Swift host owning the layer, display link, touch, keyboard,
and URL opening.

## Prerequisite

Full Xcode (not just Command Line Tools) — `gomobile bind -target=ios`
refuses without it. Install from the App Store, then:

```sh
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
```

## Build & run (simulator)

The `gophics` CLI drives bind → xcodegen → xcodebuild → install → launch in one
command (needs `xcodegen`: `brew install xcodegen`). From the repo root:

```sh
gophics run -p ios ./examples/news/mobile
gophics run -p ios -tags gophics_verify ./examples/news/mobile  # GPU bring-up scene
gophics build -p ios ./examples/news/mobile                       # just the xcframework
```

It finds this host project (the sibling `ios/` of the `mobile` package),
regenerates the Xcode project, picks a booted or available iPhone simulator, and
builds + installs + launches. For a device, build with `-sdk iphoneos` and a
signing team, or open `GophicsNews.xcodeproj` in Xcode and run.

Verified 2026-07-26 on iPhone 17 (iOS 26) simulator: live HN front page
over the network, bold titles, safe-area insets under the status bar.
Touch, keyboard, and navigation share the `shell/mobile.Bridge` proven
live on Android and by `shell/mobile`'s headless tests.

## Notes

- Presentation is GPU-first: the Go side renders straight to the
  `CAMetalLayer` via wgpu. When the GPU surface is unavailable (the
  Simulator), the host blits `Snapshot()` RGBA into a `CALayer` — the
  same CPU fallback as Android. See `design/mobile-gpu-bringup.md`.
- Keyboard input uses `UIKeyInput` (committed text); full IME preedit on
  iOS needs `UITextInput` adoption — tracked with M9 polish.
- The bridge API is identical to Android's, so behavior verified by
  `shell/mobile`'s headless tests carries over.

## What this host does that the others do not

**Hands Go a data directory.** `AppDelegate` calls `NewsmobileSetDataDir` with
an Application Support path inside the sandbox before `NewsmobileStart`. It has
to come from here: the sandbox path changes between installs. Application
Support rather than Caches, because the system may delete Caches under storage
pressure and that would throw away everything the ranking has learned.

**Presents the sign-in web view.** Publishers that gate their articles need the
subscriber's own session cookie, and gophics cannot supply it — `shell.WebView`
is implemented for the web shell only and exposes no cookie access. The Go side
raises a request, the frame callback polls it, and `LoginSheet.swift` shows a
`WKWebView`:

```swift
let domain = NewsmobilePendingLoginDomain()
if !domain.isEmpty {
    NewsmobileClearPendingLogin()
    LoginSheet.present(from: self, domain: domain, url: NewsmobilePendingLoginURL())
}
```

On "Done" it reads the site's cookies from `WKHTTPCookieStore` and passes them
to `NewsmobileSetCookies`. Every cookie for the site is taken, never a
hand-picked one: which cookie carries the session is undocumented and changes.
