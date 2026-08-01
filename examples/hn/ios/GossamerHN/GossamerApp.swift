// Thin iOS host for the gossamer HN app (M9 embedding model): the Go side
// (Hnmobile.xcframework, built by gomobile bind) owns the UI; this host
// owns the layer, display link, touch, keyboard, and URL opening —
// mirroring the Android host.
import UIKit
import Hnmobile

@main
class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?

    func application(_ application: UIApplication,
                     didFinishLaunchingWithOptions launchOptions: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        let err = HnmobileStart()
        if !err.isEmpty { fatalError("gossamer start: \(err)") }
        let w = UIWindow(frame: UIScreen.main.bounds)
        w.rootViewController = GossamerViewController()
        w.makeKeyAndVisible()
        window = w
        return true
    }

    func applicationWillResignActive(_ application: UIApplication) { HnmobileFocused(false) }
    func applicationDidBecomeActive(_ application: UIApplication) { HnmobileFocused(true) }
}

class GossamerViewController: UIViewController {
    override func loadView() { view = GossamerView() }
    override var prefersStatusBarHidden: Bool { false }
}

class GossamerView: UIView, UIKeyInput {
    private var displayLink: CADisplayLink?
    private var lastTime: CFTimeInterval = 0
    private var keyboardVisible = false
    private var surfaceSet = false

    // The Go side renders on the GPU straight to this CAMetalLayer.
    override class var layerClass: AnyClass { CAMetalLayer.self }

    override func didMoveToWindow() {
        super.didMoveToWindow()
        guard window != nil else { displayLink?.invalidate(); return }
        HnmobileSetDarkMode(traitCollection.userInterfaceStyle == .dark)
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
        if !surfaceSet {
            let ptr = Int64(Int(bitPattern: Unmanaged.passUnretained(metal).toOpaque()))
            HnmobileSetSurface(0, ptr, wPx, hPx, Double(scale))
            surfaceSet = true
        }
        HnmobileResize(wPx, hPx, Double(scale))
        let i = safeAreaInsets
        HnmobileSetInsets(Double(i.top * scale), Double(i.right * scale),
                          Double(i.bottom * scale), Double(i.left * scale))
    }

    @objc private func frame(_ link: CADisplayLink) {
        let dt = lastTime == 0 ? 1.0 / 60 : link.timestamp - lastTime
        lastTime = link.timestamp
        guard HnmobileNeedsFrame() else { syncKeyboard(); return }
        HnmobileRenderFrame(dt) // renders on the GPU straight to the CAMetalLayer
        while true {
            let url = HnmobileTakeOpenedURL()
            if url.isEmpty { break }
            if let u = URL(string: url) { UIApplication.shared.open(u) }
        }
        syncKeyboard()
    }

    private func syncKeyboard() {
        let want = HnmobileTextInputActive()
        guard want != keyboardVisible else { return }
        keyboardVisible = want
        if want { becomeFirstResponder() } else { resignFirstResponder() }
    }

    // --- Touch ---

    private func send(_ phase: Int, _ t: UITouch) {
        let scale = window?.screen.scale ?? 2
        let p = t.location(in: self)
        HnmobileTouch(phase, Double(p.x * scale), Double(p.y * scale))
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
        if text == "\n" { HnmobileKey(1, true) } else { HnmobileText(text) }
    }
    func deleteBackward() { HnmobileKey(2, true) }

    // --- VoiceOver: expose gossamer's semantics tree as a flat list of
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
        let count = HnmobileA11yRefresh()
        var out: [Any] = []
        for i in 0..<count {
            let label = HnmobileA11yLabel(i)
            let tappable = HnmobileA11yTappable(i)
            // Skip pure structural containers with nothing to announce.
            if label.isEmpty && !tappable { continue }
            let el = GossamerA11yElement(accessibilityContainer: self)
            el.nodeID = HnmobileA11yID(i)
            let value = HnmobileA11yValue(i)
            el.accessibilityLabel = value.isEmpty ? label : "\(label), \(value)"
            let hint = HnmobileA11yHint(i)
            if !hint.isEmpty { el.accessibilityHint = hint }
            el.accessibilityTraits = tappable ? .button : .staticText
            let r = CGRect(x: Double(HnmobileA11yX(i)) / scale, y: Double(HnmobileA11yY(i)) / scale,
                           width: Double(HnmobileA11yW(i)) / scale, height: Double(HnmobileA11yH(i)) / scale)
            el.accessibilityFrame = UIAccessibility.convertToScreenCoordinates(r, in: self)
            out.append(el)
        }
        return out
    }
}

/// A single VoiceOver element backed by a gossamer semantics node; activating
/// it (double-tap) fires the widget's OnActivate through the bridge.
final class GossamerA11yElement: UIAccessibilityElement {
    var nodeID: Int = -1
    override func accessibilityActivate() -> Bool {
        HnmobileA11yActivate(nodeID)
        return true
    }
}
