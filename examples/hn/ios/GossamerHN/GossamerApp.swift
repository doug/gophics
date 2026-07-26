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

    override class var layerClass: AnyClass { CALayer.self }

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
        HnmobileResize(Int(bounds.width * scale), Int(bounds.height * scale), Double(scale))
        let i = safeAreaInsets
        HnmobileSetInsets(Double(i.top * scale), Double(i.right * scale),
                          Double(i.bottom * scale), Double(i.left * scale))
    }

    @objc private func frame(_ link: CADisplayLink) {
        let dt = lastTime == 0 ? 1.0 / 60 : link.timestamp - lastTime
        lastTime = link.timestamp
        guard HnmobileNeedsFrame() else { syncKeyboard(); return }
        guard let pixels = HnmobileRenderFrame(dt) else { return }
        let w = HnmobileFrameWidth(), h = HnmobileFrameHeight()
        guard w > 0, h > 0 else { return }

        pixels.withUnsafeBytes { (buf: UnsafeRawBufferPointer) in
            let ctx = CGContext(data: UnsafeMutableRawPointer(mutating: buf.baseAddress),
                                width: w, height: h, bitsPerComponent: 8, bytesPerRow: w * 4,
                                space: CGColorSpaceCreateDeviceRGB(),
                                bitmapInfo: CGImageAlphaInfo.premultipliedLast.rawValue)
            if let img = ctx?.makeImage() {
                layer.contents = img
            }
        }
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
}
