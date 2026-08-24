# Mirror on iOS

The only example that wires the capture backends, and therefore the only one
that compiles `GophicsMonitor.swift` and `GophicsPreview.swift` — the reference
hosts in `shell/mobile/native`. Everything else in this tree checks them by
typechecking against a `gomobile bind`; this builds them into an app.

    gomobile bind -target=ios,iossimulator -o ios/Mirrormobile.xcframework \
        ./examples/mirror/mobile ./shell/mobile
    cd examples/mirror/ios && xcodegen && \
        xcodebuild -project GophicsMirror.xcodeproj -scheme GophicsMirror \
          -sdk iphonesimulator -destination 'platform=iOS Simulator,name=iPhone 17 Pro' build

**The Simulator has no camera.** Apple does not emulate one, so `StartPreview`
finds no device there and mirror reports the failure — which is worth running
for its own sake, since it is the path an app takes on a machine without a
camera. The microphone is real: the Simulator passes the Mac's through, so the
monitor half runs for real.

A device exercises both.
