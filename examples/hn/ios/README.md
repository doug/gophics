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

## Build & run (simulator)

Reproducible, no manual Xcode GUI steps (needs `xcodegen`: `brew install
xcodegen`). From `examples/hn/ios`:

```sh
# 1. Build the Go side into an xcframework (device + simulator slices).
gomobile bind -target=ios,iossimulator -o Hnmobile.xcframework ../mobile

# 2. Generate the Xcode project from project.yml and build for the sim.
xcodegen generate
xcodebuild -project GossamerHN.xcodeproj -scheme GossamerHN \
  -sdk iphonesimulator -configuration Debug \
  -destination 'platform=iOS Simulator,name=iPhone 17' \
  -derivedDataPath build build

# 3. Boot, install, launch.
xcrun simctl boot "iPhone 17"
xcrun simctl install "iPhone 17" \
  build/Build/Products/Debug-iphonesimulator/GossamerHN.app
xcrun simctl launch "iPhone 17" dev.gossamer.hn
```

For a device, build with `-sdk iphoneos` and a signing team, or open
`GossamerHN.xcodeproj` in Xcode and run.

Verified 2026-07-26 on iPhone 17 (iOS 26) simulator: live HN front page
over the network, bold titles, safe-area insets under the status bar.
Touch, keyboard, and navigation share the `shell/mobile.Bridge` proven
live on Android and by `shell/mobile`'s headless tests.

## Notes

- Pixel presentation is `CGContext` → `layer.contents` (the same CPU
  path as Android/web); a Metal-layer path can replace it inside the
  host without touching the Go side.
- Keyboard input uses `UIKeyInput` (committed text); full IME preedit on
  iOS needs `UITextInput` adoption — tracked with M9 polish.
- The bridge API is identical to Android's, so behavior verified by
  `shell/mobile`'s headless tests carries over.
