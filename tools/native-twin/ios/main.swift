// The iOS twin: a UIScrollView over the same scene tools/uitrace replays through
// gophics, recording one real flick in the Simulator into the trace contract.
//
// This is the reference that binds. gophics's own fling runs where there is no
// OS momentum — native touch and web-touch — and its constants implement
// UIKit's documented deceleration (0.998 per millisecond, τ ≈ 0.5s). Whether
// UIKit actually does that is what this records.
//
// The finger phase comes from the scroll view's own pan recognizer — the same
// events UIKit derives its momentum from — as translation deltas with
// timestamps. The offset is sampled per display frame by CADisplayLink. When
// deceleration ends and the view is still, the trace is printed to stdout
// between markers (simctl launch --console carries it out of the sandbox) and
// the app exits.
import UIKit
import QuartzCore

struct Sample: Codable { let t: Double; let v: Double }
struct Trace: Codable {
    var source = "ios-uikit"
    var hz: Double = 0
    var notes = ""
    var input: [Sample] = []
    var offset: [Sample] = []
    var release_t: Double = 0
}

final class RowsView: UIView {
    static let rowH: CGFloat = 44, rows = 300
    override func draw(_ dirty: CGRect) {
        let ctx = UIGraphicsGetCurrentContext()!
        let attrs: [NSAttributedString.Key: Any] = [
            .font: UIFont.systemFont(ofSize: 16),
            .foregroundColor: UIColor(red: 0.1, green: 0.1, blue: 0.12, alpha: 1)]
        for i in 0..<RowsView.rows {
            let r = CGRect(x: 0, y: CGFloat(i) * RowsView.rowH, width: bounds.width, height: RowsView.rowH)
            if !r.intersects(dirty) { continue }
            ctx.setFillColor((i % 2 == 1 ? UIColor(red: 0.90, green: 0.91, blue: 0.94, alpha: 1)
                                         : UIColor(red: 0.96, green: 0.96, blue: 0.97, alpha: 1)).cgColor)
            ctx.fill(r)
            ("Row \(i)" as NSString).draw(at: CGPoint(x: 16, y: r.minY + 12), withAttributes: attrs)
        }
    }
}

final class TwinVC: UIViewController, UIScrollViewDelegate {
    let scroll = UIScrollView()
    var trace = Trace()
    var t0: Double?
    var lastTranslation: CGFloat = 0
    var released = false, momentumEnded = false, finished = false
    var quiet = 0
    var lastY: Double = .nan
    var link: CADisplayLink?
    var frameTimes: [Double] = []

    override func viewDidLoad() {
        super.viewDidLoad()
        view.backgroundColor = .white
        scroll.frame = view.bounds
        scroll.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        scroll.contentInsetAdjustmentBehavior = .never
        scroll.delegate = self
        let doc = RowsView(frame: CGRect(x: 0, y: 0, width: view.bounds.width,
                                         height: CGFloat(RowsView.rows) * RowsView.rowH))
        doc.backgroundColor = .white
        scroll.addSubview(doc)
        scroll.contentSize = doc.bounds.size
        view.addSubview(scroll)
        scroll.panGestureRecognizer.addTarget(self, action: #selector(pan(_:)))
        let v = UIDevice.current
        trace.notes = "iOS \(v.systemVersion) Simulator (\(v.model)), UIScrollView decelerationRate normal, one flick; offset per display frame"
        print("twin ready — flick upward once in the Simulator window"); fflush(stdout)
    }

    @objc func pan(_ g: UIPanGestureRecognizer) {
        if released { return } // one gesture per file
        let t = CACurrentMediaTime()
        let ty = g.translation(in: view).y
        switch g.state {
        case .began:
            t0 = t; lastTranslation = ty
            startFrames()
        case .changed:
            guard let t0 = t0 else { return }
            trace.input.append(Sample(t: t - t0, v: Double(ty - lastTranslation)))
            lastTranslation = ty
        case .ended, .cancelled:
            guard let t0 = t0 else { return }
            let d = ty - lastTranslation
            if d != 0 { trace.input.append(Sample(t: t - t0, v: Double(d))) }
            released = true
            trace.release_t = t - t0
        default: break
        }
    }

    func scrollViewDidEndDecelerating(_ sv: UIScrollView) { momentumEnded = true }
    func scrollViewDidEndDragging(_ sv: UIScrollView, willDecelerate d: Bool) { if !d { momentumEnded = true } }

    func startFrames() {
        sample()
        link = CADisplayLink(target: self, selector: #selector(tick))
        link?.add(to: .main, forMode: .common)
    }
    @objc func tick() { sample() }

    func sample() {
        guard let t0 = t0, !finished else { return }
        let t = CACurrentMediaTime() - t0
        let y = Double(scroll.contentOffset.y)
        trace.offset.append(Sample(t: t, v: y))
        frameTimes.append(t)
        if released {
            quiet = (y == lastY) ? quiet + 1 : 0
            if (momentumEnded && quiet >= 3) || quiet >= 60 { finish() }
        }
        lastY = y
    }

    func finish() {
        finished = true
        link?.invalidate()
        if frameTimes.count > 2 {
            trace.hz = (Double(frameTimes.count - 1) / (frameTimes.last! - frameTimes.first!)).rounded()
        }
        // Contract: finger up is negative input and offset rises. A pan
        // translation already has that sign; normalize anyway, so a change in
        // how the deltas are derived cannot silently flip a reference.
        let inSum = trace.input.reduce(0) { $0 + $1.v }
        let travel = (trace.offset.last?.v ?? 0) - (trace.offset.first?.v ?? 0)
        if inSum != 0 && travel != 0 && (inSum > 0) == (travel > 0) {
            trace.input = trace.input.map { Sample(t: $0.t, v: -$0.v) }
            trace.notes += "; input sign flipped to the contract's convention"
        }
        let enc = JSONEncoder(); enc.outputFormatting = [.sortedKeys]
        let json = String(data: try! enc.encode(trace), encoding: .utf8)!
        print("TRACE-BEGIN"); print(json); print("TRACE-END")
        print("recorded \(trace.input.count) input events, \(trace.offset.count) frames at \(Int(trace.hz))Hz, release at \(String(format: "%.3f", trace.release_t))s, travel \(Int(travel))px")
        fflush(stdout)
        DispatchQueue.main.asyncAfter(deadline: .now() + 0.2) { exit(0) }
    }
}

final class AppDelegate: UIResponder, UIApplicationDelegate {
    var window: UIWindow?
    func application(_ app: UIApplication, didFinishLaunchingWithOptions o: [UIApplication.LaunchOptionsKey: Any]?) -> Bool {
        window = UIWindow(frame: UIScreen.main.bounds)
        window?.rootViewController = TwinVC()
        window?.makeKeyAndVisible()
        return true
    }
}

UIApplicationMain(CommandLine.argc, CommandLine.unsafeArgv, nil, NSStringFromClass(AppDelegate.self))
