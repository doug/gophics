# gossamer HN — iOS host

The same M9 embedding model as Android: `shell/mobile.Bridge` on the Go
side, a thin Swift host owning the layer, display link, touch, keyboard,
and URL opening.

## Prerequisite

Full Xcode (not just Command Line Tools) — `gomobile bind -target=ios`
refuses without it. Install from the App Store, then:

```sh
sudo xcode-select -s /Applications/Xcode.app/Contents/Developer
```

## Build

```sh
# From the repo root: builds Hnmobile.xcframework (device + simulator).
gomobile bind -target=ios,iossimulator \
  -o examples/hn/ios/Hnmobile.xcframework ./examples/hn/mobile
```

Then create an iOS App project in Xcode (or via xcodegen), add
`GossamerHN/GossamerApp.swift` and drag in `Hnmobile.xcframework`
(Embed & Sign). Run on a simulator or device.

## Notes

- Pixel presentation is `CGContext` → `layer.contents` (the same CPU
  path as Android/web); a Metal-layer path can replace it inside the
  host without touching the Go side.
- Keyboard input uses `UIKeyInput` (committed text); full IME preedit on
  iOS needs `UITextInput` adoption — tracked with M9 polish.
- The bridge API is identical to Android's, so behavior verified by
  `shell/mobile`'s headless tests carries over.
