// Reference iOS PlatformViewHost for gophics's shell/mobile platform-view
// bridge (shell/mobile/platformview.go).
//
// Wire once at startup, naming the kinds this host can build:
//
//     let pv = GophicsPlatformViews(bridge: bridge, container: gophicsView)
//     bridge.setPlatformViewHost(pv, kinds: "webview")
//
// Until that call, ctx.PlatformViews() is nil in Go and widget.PlatformView
// paints its placeholder. Registering is the whole opt-in: an app that embeds
// nothing changes nothing.
//
// iOS needs no restructuring to host one of these. GophicsView is a UIView
// whose layerClass is CAMetalLayer, and a UIKit subview composites above its
// superview's own layer content — so adding a subview to the gophics view is
// all there is to it. This is the same mechanism Flutter uses on iOS.
//
// Two limits are structural and not worked around here or anywhere: a native
// view composites above the whole gophics surface, so nothing gophics draws
// can appear over it; and it is invisible to bridge.snapshot(), so a golden
// image of this screen shows the placeholder where the view is. See the doc
// comment on shell.PlatformViews.
//
// The kinds below are a reference. Add your own by extending the switch in
// createView — a kind gophics has never heard of is fine, which is the point
// of a registry: pass its name to setPlatformViewHost and the Go side offers
// it through Kinds().

import Foundation
import Mobile
import UIKit
import WebKit

final class GophicsPlatformViews: NSObject {
    private let bridge: MobileBridge
    // The view the subviews are added to — the gophics view itself, or any
    // ancestor of it. gophics never sees this and never requires it to be
    // anything in particular: placement is entirely the host's decision.
    private weak var container: UIView?

    private var views: [Int: UIView] = [:]
    // Which ids are still loading, so a WKNavigationDelegate callback that
    // arrives after destroyView does not report readiness for a dead id.
    private var pending: Set<Int> = []

    init(bridge: MobileBridge, container: UIView) {
        self.bridge = bridge
        self.container = container
    }

    // Bounds cross from Go in physical pixels (the same units as setSurface),
    // and UIKit frames are in points, so every rect divides by the screen
    // scale. Skipping this is invisible on a 1x simulator and three times too
    // large on a real phone.
    private var scale: CGFloat { container?.window?.screen.scale ?? UIScreen.main.scale }
}

extension GophicsPlatformViews: MobilePlatformViewHostProtocol {
    func createView(_ id: Int, kind: String?, params: String?) {
        guard let container = container else {
            bridge.failView(id, msg: "no container view")
            return
        }
        let params = params ?? ""
        switch kind ?? "" {
        case "webview":
            let cfg = WKWebViewConfiguration()
            // A message handler is how a cooperating page talks back:
            // window.webkit.messageHandlers.gophics.postMessage("…") becomes
            // deliverViewMessage on the Go side.
            cfg.userContentController.add(self, name: "gophics")
            let web = WKWebView(frame: .zero, configuration: cfg)
            web.navigationDelegate = self
            // Start hidden. Go sends the first setViewBounds immediately after
            // this call returns, and a view that appears at .zero first
            // flickers in the top-left corner for one frame.
            web.isHidden = true
            web.tag = id
            views[id] = web
            pending.insert(id)
            container.addSubview(web)
            if let url = URL(string: params) {
                web.load(URLRequest(url: url))
            } else {
                finish(id, error: "not a URL: \(params)")
            }
        default:
            // Answer, always. A host that ignores an unknown kind leaves the
            // widget waiting on a view that will never arrive, which shows as
            // a loading state that never ends.
            bridge.failView(id, msg: "unsupported platform view kind: \(kind ?? "")")
        }
    }

    func setViewBounds(_ id: Int, x: Int, y: Int, w: Int, h: Int) {
        guard let v = views[id] else { return }
        if w <= 0 || h <= 0 {
            // Hidden, not destroyed: the view keeps its state — a scroll
            // position, a half-filled form, a playing video — for when it
            // scrolls back into the layout.
            v.isHidden = true
            return
        }
        let s = scale
        v.frame = CGRect(x: CGFloat(x) / s, y: CGFloat(y) / s,
                         width: CGFloat(w) / s, height: CGFloat(h) / s)
        v.isHidden = pending.contains(id)
    }

    func send(toView id: Int, msg: String?) {
        guard let msg = msg, !msg.isEmpty else { return }
        if let web = views[id] as? WKWebView, let url = URL(string: msg) {
            web.load(URLRequest(url: url))
        }
    }

    func destroyView(_ id: Int) {
        pending.remove(id)
        guard let v = views.removeValue(forKey: id) else { return }
        if let web = v as? WKWebView {
            web.navigationDelegate = nil
            web.configuration.userContentController
                .removeScriptMessageHandler(forName: "gophics")
            web.stopLoading()
        }
        v.removeFromSuperview()
    }

    // finish reports the one-and-only outcome for an id and unhides the view
    // on success. Called from the navigation delegate, which may fire more
    // than once per view (a redirect, a second load after send); pending is
    // what makes the report happen exactly once.
    fileprivate func finish(_ id: Int, error: String?) {
        guard pending.remove(id) != nil else { return }
        if let error = error {
            bridge.failView(id, msg: error)
            return
        }
        views[id]?.isHidden = false
        bridge.deliverViewReady(id)
    }
}

extension GophicsPlatformViews: WKNavigationDelegate {
    func webView(_ webView: WKWebView, didFinish navigation: WKNavigation!) {
        finish(webView.tag, error: nil)
        bridge.deliverViewMessage(webView.tag, msg: "loaded")
    }

    func webView(_ webView: WKWebView, didFail navigation: WKNavigation!,
                 withError error: Error) {
        finish(webView.tag, error: error.localizedDescription)
    }

    func webView(_ webView: WKWebView, didFailProvisionalNavigation navigation: WKNavigation!,
                 withError error: Error) {
        finish(webView.tag, error: error.localizedDescription)
    }
}

extension GophicsPlatformViews: WKScriptMessageHandler {
    func userContentController(_ controller: WKUserContentController,
                               didReceive message: WKScriptMessage) {
        guard let web = message.webView else { return }
        bridge.deliverViewMessage(web.tag, msg: String(describing: message.body))
    }
}
