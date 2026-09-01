# gophics HN — iOS host

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
gophics run -p ios ./examples/hn/mobile
gophics run -p ios -host . ./examples/gpucheck/mobile      # GPU bring-up scene
gophics build -p ios ./examples/hn/mobile                       # just the xcframework
```

It finds this host project (the sibling `ios/` of the `mobile` package),
regenerates the Xcode project, picks a booted or available iPhone simulator, and
builds + installs + launches. For a device, build with `-sdk iphoneos` and a
signing team, or open `GophicsHN.xcodeproj` in Xcode and run.

Verified 2026-07-26 on iPhone 17 (iOS 26) simulator: live HN front page
over the network, bold titles, safe-area insets under the status bar.
Touch, keyboard, and navigation share the `shell/mobile.Bridge` proven
live on Android and by `shell/mobile`'s headless tests.

## Notes

- Presentation is GPU-first: the Go side renders straight to the
  `CAMetalLayer` via wgpu. When the GPU surface is unavailable (the
  Simulator), the host blits `Snapshot()` RGBA into a `CALayer` — the
  same CPU fallback as Android.
- Keyboard input uses `UIKeyInput` (committed text); full IME preedit on
  iOS needs `UITextInput` adoption — tracked with M9 polish.
- The bridge API is identical to Android's, so behavior verified by
  `shell/mobile`'s headless tests carries over.
