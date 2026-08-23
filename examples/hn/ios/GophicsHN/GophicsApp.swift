// Thin iOS host for the gophics HN app (M9 embedding model): the Go side
// (Hnmobile.xcframework, built by gomobile bind) owns the UI; this host
// owns the layer, display link, touch, keyboard, and URL opening —
// mirroring the Android host.
import UIKit
import Hnmobile

// One bridge per process, at file scope because the app delegate and the view
// controller both drive it. gomobile assumes one anyway: Start builds the app
// once.
private var bridge: MobileBridge!

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        // Start is a package-level Go function, so gomobile emits it as a C
        // function rather than a method — which means Swift does not translate
        // its NSError** into `throws`, and the pointer is passed by hand.
        var err: NSError?
        guard let b = HnmobileStart(&err) else {
            fatalError("gophics start: \(err?.localizedDescription ?? "unknown")")
        }
        bridge = b
        let w = UIWindow(frame: UIScreen.main.bounds)
        w.rootViewController = GophicsViewController()
        w.makeKeyAndVisible()
        window = w
        return true
    }

    func applicationWillResignActive(_ application: UIApplication) { bridge.focused(false) }
    func applicationDidBecomeActive(_ application: UIApplication) { bridge.focused(true) }
}

class GophicsViewController: UIViewController {
    override func loadView() { view = GophicsView() }
    override var prefersStatusBarHidden: Bool { false }
}

class GophicsView: UIView, UIKeyInput {
    private var displayLink: CADisplayLink?
    private var lastTime: CFTimeInterval = 0
    private var keyboardVisible = false
    private var surfaceSet = false

    // CPU present fallback: when the GPU surface can't be created (iOS
    // Simulator — its Metal lacks the HAL wgpu needs), the Go side rasterizes
    // each frame on the CPU and we blit it into this layer instead. Same
    // parity-tested rasterizer; GPU on device, CPU in the Simulator.
    private let cpuLayer = CALayer()
    private let rgbColorSpace = CGColorSpaceCreateDeviceRGB()

    // The Go side renders on the GPU straight to this CAMetalLayer.
    override class var layerClass: AnyClass { CAMetalLayer.self }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        guard window != nil else { displayLink?.invalidate(); return }
        if cpuLayer.superlayer == nil {
            cpuLayer.isHidden = true
            layer.addSublayer(cpuLayer)
        }
        bridge.setDarkMode(traitCollection.userInterfaceStyle == .dark)
        let link = CADisplayLink(target: self, selector: #selector(frame(_:)))
        link.add(to: .main, forMode: .common)
        displayLink = link
    }

    override func layoutSubviews() {
        super.layoutSubviews()
        let scale = window?.screen.scale ?? 2
        let wPx = Int(bounds.width * scale), hPx = Int(bounds.height * scale)
        guard wPx > 0, hPx > 0 else { return }

        // Hand the CAMetalLayer to the Go side once it has a size; the GPU
        // renders directly to it. Resize thereafter reconfigures the surface.
        let metal = layer as! CAMetalLayer
        metal.contentsScale = scale
        metal.drawableSize = CGSize(width: wPx, height: hPx)
        CATransaction.begin(); CATransaction.setDisableActions(true)
        cpuLayer.frame = bounds
        cpuLayer.contentsScale = scale
        CATransaction.commit()
        if !surfaceSet {
            let ptr = Int64(Int(bitPattern: Unmanaged.passUnretained(metal).toOpaque()))
            bridge.setSurface(0, windowHandle: ptr, widthPx: wPx, heightPx: hPx, scale: Float(scale))
            surfaceSet = true
        }
        bridge.resize(wPx, heightPx: hPx, scale: Float(scale))
        let i = safeAreaInsets
        bridge.setInsets(Float(i.top * scale), rightPx: Float(i.right * scale), bottomPx: Float(i.bottom * scale), leftPx: Float(i.left * scale))
    }

    @objc private func frame(_ link: CADisplayLink) {
        let dt = lastTime == 0 ? 1.0 / 60 : link.timestamp - lastTime
        lastTime = link.timestamp
        guard bridge.needsFrame() else { syncKeyboard(); return }
        if bridge.gpuActive() {
            bridge.renderFrame(dt) // renders on the GPU straight to the CAMetalLayer
        } else {
            presentCPU(dt) // Simulator: rasterize on the CPU and blit
        }
        while true {
            let url = bridge.takeOpenedURL()
            if url.isEmpty { break }
            if let u = URL(string: url) { UIApplication.shared.open(u) }
        }
        while true {
            let h = bridge.takeHaptic()
            if h < 0 { break }
            playHaptic(h)
        }
        syncKeyboard()
    }

    // playHaptic maps a gophics shell.HapticKind (drained from the bridge each
    // frame) to the matching UIFeedbackGenerator — the iOS counterpart to the
    // Android host's performHapticFeedback. Generators are cheap to create; the
    // system honours the user's system-haptics setting.
    private func playHaptic(_ kind: Int) {
        switch kind {
        case 0: UISelectionFeedbackGenerator().selectionChanged()
        case 1: UIImpactFeedbackGenerator(style: .light).impactOccurred()
        case 2: UIImpactFeedbackGenerator(style: .medium).impactOccurred()
        case 3: UIImpactFeedbackGenerator(style: .heavy).impactOccurred()
        case 4: UINotificationFeedbackGenerator().notificationOccurred(.success)
        case 5: UINotificationFeedbackGenerator().notificationOccurred(.warning)
        case 6: UINotificationFeedbackGenerator().notificationOccurred(.error)
        default: UIImpactFeedbackGenerator(style: .light).impactOccurred()
        }
    }

    // presentCPU renders one frame on the CPU (Hnmobile.Snapshot → RGBA8888)
    // and shows it in cpuLayer. Used only when GPU rendering is unavailable.
    private func presentCPU(_ dt: CFTimeInterval) {
        guard let data = bridge.snapshot(dt), !data.isEmpty else { return }
        let w = bridge.frameWidth(), h = bridge.frameHeight()
        guard w > 0, h > 0, data.count >= w * h * 4,
              let provider = CGDataProvider(data: data as CFData) else { return }
        let img = CGImage(
            width: w, height: h, bitsPerComponent: 8, bitsPerPixel: 32, bytesPerRow: w * 4,
            space: rgbColorSpace,
            bitmapInfo: CGBitmapInfo(rawValue: CGImageAlphaInfo.premultipliedLast.rawValue),
            provider: provider, decode: nil, shouldInterpolate: false, intent: .defaultIntent)
        CATransaction.begin(); CATransaction.setDisableActions(true)
        cpuLayer.contents = img
        cpuLayer.isHidden = false
        CATransaction.commit()
    }

    private func syncKeyboard() {
        let want = bridge.textInputActive()
        guard want != keyboardVisible else { return }
        keyboardVisible = want
        if want { becomeFirstResponder() } else { resignFirstResponder() }
    }

    // --- Touch ---

    private func send(_ phase: Int, _ t: UITouch) {
        let scale = window?.screen.scale ?? 2
        let p = t.location(in: self)
        bridge.touch(phase, xPx: Float(p.x * scale), yPx: Float(p.y * scale))
    }

    override func touchesBegan(_ touches: Set<UITouch>, with event: UIEvent?) {
        touches.first.map { send(0, $0) }
    }
    override func touchesMoved(_ touches: Set<UITouch>, with event: UIEvent?) {
        touches.first.map { send(1, $0) }
    }
    override func touchesEnded(_ touches: Set<UITouch>, with event: UIEvent?) {
        touches.first.map { send(2, $0) }
    }
    override func touchesCancelled(_ touches: Set<UITouch>, with event: UIEvent?) {
        touches.first.map { send(3, $0) }
    }

    // --- UIKeyInput: the on-screen keyboard commits through here ---

    override var canBecomeFirstResponder: Bool { true }
    var hasText: Bool { true }
    func insertText(_ text: String) {
        if text == "\n" { bridge.key(1, pressed: true) } else { bridge.text(text) }
    }
    func deleteBackward() { bridge.key(2, pressed: true) }

    // --- VoiceOver: expose gophics's semantics tree as a flat list of
    // virtual accessibility elements (the Go side owns the pixels, so there
    // are no real subviews). Mirrors the Android AccessibilityNodeProvider,
    // consuming the same Hnmobile.A11y* surface. ---

    override var isAccessibilityElement: Bool {
        get { false }
        set { }
    }

    override var accessibilityElements: [Any]? {
        get { buildA11yElements() }
        set { }
    }

    private func buildA11yElements() -> [Any] {
        let scale = window?.screen.scale ?? 2
        let count = bridge.a11yRefresh()
        var out: [Any] = []
        for i in 0..<count {
            let label = bridge.a11yLabel(i)
            let tappable = bridge.a11yTappable(i)
            // Skip pure structural containers with nothing to announce.
            if label.isEmpty && !tappable { continue }
            let el = GophicsA11yElement(accessibilityContainer: self)
            el.nodeID = bridge.a11yID(i)
            let value = bridge.a11yValue(i)
            el.accessibilityLabel = value.isEmpty ? label : "\(label), \(value)"
            let hint = bridge.a11yHint(i)
            if !hint.isEmpty { el.accessibilityHint = hint }
            el.accessibilityTraits = tappable ? .button : .staticText
            let r = CGRect(x: Double(bridge.a11yX(i)) / scale, y: Double(bridge.a11yY(i)) / scale,
                           width: Double(bridge.a11yW(i)) / scale, height: Double(bridge.a11yH(i)) / scale)
            el.accessibilityFrame = UIAccessibility.convertToScreenCoordinates(r, in: self)
            out.append(el)
        }
        return out
    }
}

/// A single VoiceOver element backed by a gophics semantics node; activating
/// it (double-tap) fires the widget's OnActivate through the bridge.
final class GophicsA11yElement: UIAccessibilityElement {
    var nodeID: Int = -1
    override func accessibilityActivate() -> Bool {
        bridge.a11yActivate(nodeID)
        return true
    }
}
