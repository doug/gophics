// The macOS twin: an NSScrollView over the same scene tools/uitrace replays
// through gophics, recording one real trackpad flick into the trace contract
// (tools/uitrace/README.md).
//
// It records the finger phase — every scrollWheel event with a gesture phase,
// with its own timestamp — and the scroll view's offset once per display
// frame from the first touch until momentum has ended and the view has been
// still for a moment. Then it writes trace.json and quits: one run, one
// gesture, one file. That is deliberate. A reference recording should be a
// thing you can point at, not a session you have to describe.
//
// Build:  ./build.sh            (swiftc, Cocoa; no Xcode project)
// Run:    ./twin [out.json]     then flick upward on the trackpad, once.
import Cocoa
import CoreVideo

struct Sample: Codable { let t: Double; let v: Double }
struct Trace: Codable {
    var source = "macos-appkit"
    var hz: Double = 0
    var notes = ""
    var input: [Sample] = []
    var offset: [Sample] = []
    var release_t: Double = 0
}

final class RowsView: NSView {
    static let rowH: CGFloat = 44, rows = 300
    override var isFlipped: Bool { true }   // y down, like every other UI toolkit
    override func draw(_ dirty: NSRect) {
        let bounds = self.bounds
        for i in 0..<RowsView.rows {
            let r = NSRect(x: 0, y: CGFloat(i) * RowsView.rowH, width: bounds.width, height: RowsView.rowH)
            if !r.intersects(dirty) { continue }
            (i % 2 == 1 ? NSColor(calibratedRed: 0.90, green: 0.91, blue: 0.94, alpha: 1)
                        : NSColor(calibratedRed: 0.96, green: 0.96, blue: 0.97, alpha: 1)).setFill()
            r.fill()
            let s = "Row \(i)" as NSString
            s.draw(at: NSPoint(x: 16, y: r.minY + 12), withAttributes: [
                .font: NSFont.systemFont(ofSize: 16),
                .foregroundColor: NSColor(calibratedRed: 0.1, green: 0.1, blue: 0.12, alpha: 1)])
        }
    }
}

final class TwinScrollView: NSScrollView {
    var onWheel: ((NSEvent) -> Void)?
    override func scrollWheel(with event: NSEvent) {
        onWheel?(event)           // observe first, so the timestamp is the event's
        super.scrollWheel(with: event)
    }
}

final class Recorder {
    let scroll: TwinScrollView
    let out: String
    var trace = Trace()
    var t0: Double?               // gesture start, in the event/uptime clock
    var released = false
    var momentumEnded = false
    var quiet = 0
    var lastY: Double = .nan
    var link: CVDisplayLink?
    var frameTimes: [Double] = []

    init(scroll: TwinScrollView, out: String) {
        self.scroll = scroll; self.out = out
        scroll.onWheel = { [unowned self] e in self.wheel(e) }
        let v = ProcessInfo.processInfo.operatingSystemVersion
        trace.notes = "macOS \(v.majorVersion).\(v.minorVersion).\(v.patchVersion), trackpad, one flick; offset sampled per display frame"
    }

    func wheel(_ e: NSEvent) {
        let ph = e.phase, mph = e.momentumPhase
        if t0 == nil {
            // Only a real gesture starts a recording; a lone wheel tick is not one.
            guard ph.contains(.began) else { return }
            t0 = e.timestamp
            startFrames()
            return
        }
        let t = e.timestamp - t0!
        if ph.contains(.began) || ph.contains(.changed) {
            trace.input.append(Sample(t: t, v: e.scrollingDeltaY))
        }
        if ph.contains(.ended) || ph.contains(.cancelled) {
            released = true
            trace.release_t = t
        }
        if mph.contains(.ended) || mph.contains(.cancelled) {
            momentumEnded = true
        }
    }

    func startFrames() {
        sample() // t = 0
        CVDisplayLinkCreateWithActiveCGDisplays(&link)
        CVDisplayLinkSetOutputHandler(link!) { [unowned self] _, _, _, _, _ in
            DispatchQueue.main.async { self.sample() }
            return kCVReturnSuccess
        }
        CVDisplayLinkStart(link!)
    }

    func sample() {
        guard let t0 = t0 else { return }
        let t = ProcessInfo.processInfo.systemUptime - t0
        let y = Double(scroll.contentView.bounds.origin.y)
        trace.offset.append(Sample(t: t, v: y))
        frameTimes.append(t)
        if released {
            quiet = (y == lastY) ? quiet + 1 : 0
            // Momentum reported ended, or nothing has moved for a while — a
            // flick with no momentum never reports .ended.
            if (momentumEnded && quiet >= 6) || quiet >= 60 { finish() }
        }
        lastY = y
    }

    func finish() {
        if let l = link { CVDisplayLinkStop(l); link = nil }
        if frameTimes.count > 2 {
            trace.hz = (Double(frameTimes.count - 1) / (frameTimes.last! - frameTimes.first!)).rounded()
        }
        // The contract: finger up is negative input, and offset rises as
        // content moves up — so a gesture's input sum and offset travel have
        // opposite signs. macOS reports scrollingDeltaY in whichever direction
        // the user's "natural scrolling" setting implies; normalize here rather
        // than making every consumer guess.
        let inSum = trace.input.reduce(0) { $0 + $1.v }
        let travel = (trace.offset.last?.v ?? 0) - (trace.offset.first?.v ?? 0)
        if inSum != 0 && travel != 0 && (inSum > 0) == (travel > 0) {
            trace.input = trace.input.map { Sample(t: $0.t, v: -$0.v) }
            trace.notes += "; input sign flipped to the contract's convention"
        }
        let enc = JSONEncoder(); enc.outputFormatting = [.prettyPrinted, .sortedKeys]
        do {
            try enc.encode(trace).write(to: URL(fileURLWithPath: out))
            print("wrote \(out): \(trace.input.count) input events, \(trace.offset.count) frames at \(Int(trace.hz))Hz, release at \(String(format: "%.3f", trace.release_t))s, travel \(Int(travel))px")
        } catch { print("write failed: \(error)") }
        NSApp.terminate(nil)
    }
}

let out = CommandLine.arguments.count > 1 ? CommandLine.arguments[1] : "twin-trace.json"
let app = NSApplication.shared
app.setActivationPolicy(.regular)

let frame = NSRect(x: 0, y: 0, width: 390, height: 844)
let win = NSWindow(contentRect: frame, styleMask: [.titled, .closable], backing: .buffered, defer: false)
win.title = "gophics twin — flick upward once"
let scroll = TwinScrollView(frame: frame)
scroll.hasVerticalScroller = true
scroll.scrollerStyle = .overlay
let doc = RowsView(frame: NSRect(x: 0, y: 0, width: 390, height: CGFloat(RowsView.rows) * RowsView.rowH))
scroll.documentView = doc
win.contentView = scroll
win.center()
win.makeKeyAndOrderFront(nil)
app.activate(ignoringOtherApps: true)

let rec = Recorder(scroll: scroll, out: out)
app.run()
