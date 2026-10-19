// Reference Android PlatformViewHost for gophics's shell/mobile platform-view
// bridge (shell/mobile/platformview.go).
//
// Wire once at startup, naming the kinds this host can build:
//
//     val root = FrameLayout(this)
//     val v = GophicsView(this)
//     root.addView(v)
//     setContentView(root)
//     bridge.setPlatformViewHost(GophicsPlatformViews(this, bridge, root), "webview")
//
// Until that call, ctx.PlatformViews() is nil in Go and widget.PlatformView
// paints its placeholder — so an activity that keeps `setContentView(v)` and
// registers nothing compiles and runs exactly as before. That is deliberate:
// the FrameLayout is the host's own arrangement, gophics never sees it and
// never requires one to exist, and adding this feature to a shipped app
// changes nothing until the app opts in.
//
// Why a wrapper is needed here and not on iOS: GophicsView is a SurfaceView
// installed as the whole content view, and a SurfaceView has no parent
// ViewGroup to put a sibling in. The z-order works out for free once there is
// one — a SurfaceView's surface is composited below the window's view surface,
// so ordinary sibling views already draw on top.
//
// Two limits are structural and not worked around here or anywhere: a native
// view composites above everything gophics draws, so no widget can appear over
// it; and it is invisible to bridge.snapshot(), so a golden image of this
// screen shows the placeholder where the view is. See the doc comment on
// shell.PlatformViews.
//
// The kinds below are a reference. Add your own by extending the `when` in
// createView — a kind gophics has never heard of is fine, which is the point
// of a registry: pass its name to setPlatformViewHost and the Go side offers
// it through Kinds().

package com.example.gophics

import android.app.Activity
import android.os.Handler
import android.os.Looper
import android.view.View
import android.webkit.WebResourceError
import android.webkit.WebResourceRequest
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.FrameLayout
import mobile.Bridge
import mobile.PlatformViewHost

class GophicsPlatformViews(
    private val activity: Activity,
    private val bridge: Bridge,
    // The container the subviews go into — a FrameLayout the host made and
    // the gophics SurfaceView is already in. gophics never sees it.
    private val container: FrameLayout,
) : PlatformViewHost {

    private val ui = Handler(Looper.getMainLooper())
    private val views = HashMap<Int, View>()

    // Ids still loading, so a WebViewClient callback arriving after
    // destroyView does not report readiness for a view that is gone.
    private val pending = HashSet<Int>()

    override fun createView(id: Int, kind: String, params: String) {
        ui.post {
            when (kind) {
                "webview" -> {
                    val web = WebView(activity)
                    web.settings.javaScriptEnabled = true
                    web.webViewClient = client(id)
                    // Start hidden: Go sends the first setViewBounds right
                    // after this returns, and a view that appears at 0,0
                    // first flickers in the corner for a frame.
                    web.visibility = View.GONE
                    views[id] = web
                    pending.add(id)
                    // Zero layout params; setViewBounds is what sizes it.
                    container.addView(web, FrameLayout.LayoutParams(0, 0))
                    web.loadUrl(params)
                }
                else ->
                    // Answer, always. A host that ignores an unknown kind
                    // leaves the widget waiting on a view that will never
                    // arrive, which shows as a loading state with no end.
                    bridge.failView(id, "unsupported platform view kind: $kind")
            }
        }
    }

    // Bounds arrive in physical pixels, which is already what Android view
    // geometry is in — no density conversion here, unlike iOS. Converting
    // anyway is the mistake to avoid: it would be invisible at 1x and wrong
    // on every real device.
    override fun setViewBounds(id: Int, x: Int, y: Int, w: Int, h: Int) {
        ui.post {
            val v = views[id] ?: return@post
            if (w <= 0 || h <= 0) {
                // Hidden, not destroyed: the view keeps its state — a scroll
                // position, a half-filled form, a playing video — for when it
                // scrolls back into the layout.
                v.visibility = View.GONE
                return@post
            }
            v.layoutParams = FrameLayout.LayoutParams(w, h).also {
                it.leftMargin = x
                it.topMargin = y
            }
            v.visibility = if (pending.contains(id)) View.GONE else View.VISIBLE
            v.requestLayout()
        }
    }

    override fun sendToView(id: Int, msg: String) {
        if (msg.isEmpty()) return
        ui.post { (views[id] as? WebView)?.loadUrl(msg) }
    }

    override fun destroyView(id: Int) {
        ui.post {
            pending.remove(id)
            val v = views.remove(id) ?: return@post
            (v as? WebView)?.let {
                it.webViewClient = WebViewClient()
                it.stopLoading()
                it.destroy()
            }
            container.removeView(v)
        }
    }

    // finish reports the one-and-only outcome for an id and reveals the view
    // on success. A WebViewClient fires more than once per view (a redirect, a
    // second load after sendToView); `pending` is what makes the report happen
    // exactly once, which is the contract Go relies on.
    private fun finish(id: Int, error: String?) {
        if (!pending.remove(id)) return
        if (error != null) {
            bridge.failView(id, error)
            return
        }
        views[id]?.visibility = View.VISIBLE
        bridge.deliverViewReady(id)
    }

    private fun client(id: Int) = object : WebViewClient() {
        override fun onPageFinished(view: WebView?, url: String?) {
            finish(id, null)
            bridge.deliverViewMessage(id, "loaded")
        }

        override fun onReceivedError(
            view: WebView?,
            request: WebResourceRequest?,
            error: WebResourceError?,
        ) {
            // Subresource failures report here too; only the main document's
            // failure is this view's failure.
            if (request?.isForMainFrame != true) return
            finish(id, error?.description?.toString() ?: "page load failed")
        }
    }
}
