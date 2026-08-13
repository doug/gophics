// Thin iOS host for Tally (M9 embedding model): the Go side
// (Tallymobile.xcframework, built by gomobile bind) owns the UI; this host
// owns the layer, display link, touch, keyboard, and URL opening —
// mirroring the Android host.
import UIKit
import Tallymobile

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        let err = TallymobileStart()
        if !err.isEmpty { fatalError("gophics start: \(err)") }
        let w = UIWindow(frame: UIScreen.main.bounds)
        w.rootViewController = GophicsViewController()
        w.makeKeyAndVisible()
        window = w
        return true
    }

    func applicationWillResignActive(_ application: UIApplication) { TallymobileFocused(false) }
    func applicationDidBecomeActive(_ application: UIApplication) { TallymobileFocused(true) }
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
        TallymobileSetDarkMode(traitCollection.userInterfaceStyle == .dark)
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
            TallymobileSetSurface(0, ptr, wPx, hPx, Double(scale))
            surfaceSet = true
        }
        TallymobileResize(wPx, hPx, Double(scale))
        let i = safeAreaInsets
        TallymobileSetInsets(Double(i.top * scale), Double(i.right * scale),
                          Double(i.bottom * scale), Double(i.left * scale))
    }

    @objc private func frame(_ link: CADisplayLink) {
        let dt = lastTime == 0 ? 1.0 / 60 : link.timestamp - lastTime
        lastTime = link.timestamp
        guard TallymobileNeedsFrame() else { syncKeyboard(); return }
        if TallymobileGpuActive() {
            TallymobileRenderFrame(dt) // renders on the GPU straight to the CAMetalLayer
        } else {
            presentCPU(dt) // Simulator: rasterize on the CPU and blit
        }
        while true {
            let url = TallymobileTakeOpenedURL()
            if url.isEmpty { break }
            if let u = URL(string: url) { UIApplication.shared.open(u) }
        }
        while true {
            let h = TallymobileTakeHaptic()
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

    // presentCPU renders one frame on the CPU (Tallymobile.Snapshot → RGBA8888)
    // and shows it in cpuLayer. Used only when GPU rendering is unavailable.
    private func presentCPU(_ dt: CFTimeInterval) {
        guard let data = TallymobileSnapshot(dt), !data.isEmpty else { return }
        let w = TallymobileFrameWidth(), h = TallymobileFrameHeight()
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
        let want = TallymobileTextInputActive()
        guard want != keyboardVisible else { return }
        keyboardVisible = want
        if want { becomeFirstResponder() } else { resignFirstResponder() }
    }

    // --- Touch ---

    private func send(_ phase: Int, _ t: UITouch) {
        let scale = window?.screen.scale ?? 2
        let p = t.location(in: self)
        TallymobileTouch(phase, Double(p.x * scale), Double(p.y * scale))
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
        if text == "\n" { TallymobileKey(1, true) } else { TallymobileText(text) }
    }
    func deleteBackward() { TallymobileKey(2, true) }

    // --- VoiceOver: expose gophics's semantics tree as a flat list of
    // virtual accessibility elements (the Go side owns the pixels, so there
    // are no real subviews). Mirrors the Android AccessibilityNodeProvider,
    // consuming the same Tallymobile.A11y* surface. ---

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
        let count = TallymobileA11yRefresh()
        var out: [Any] = []
        for i in 0..<count {
            let label = TallymobileA11yLabel(i)
            let tappable = TallymobileA11yTappable(i)
            // Skip pure structural containers with nothing to announce.
            if label.isEmpty && !tappable { continue }
            let el = GophicsA11yElement(accessibilityContainer: self)
            el.nodeID = TallymobileA11yID(i)
            let value = TallymobileA11yValue(i)
            el.accessibilityLabel = value.isEmpty ? label : "\(label), \(value)"
            let hint = TallymobileA11yHint(i)
            if !hint.isEmpty { el.accessibilityHint = hint }
            el.accessibilityTraits = tappable ? .button : .staticText
            let r = CGRect(x: Double(TallymobileA11yX(i)) / scale, y: Double(TallymobileA11yY(i)) / scale,
                           width: Double(TallymobileA11yW(i)) / scale, height: Double(TallymobileA11yH(i)) / scale)
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
        TallymobileA11yActivate(nodeID)
        return true
    }
}
