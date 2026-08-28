package dev.gophics.news

import android.app.Activity
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Rect
import android.net.Uri
import android.os.Bundle
import android.util.Log
import android.view.Choreographer
import android.view.HapticFeedbackConstants
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.accessibility.AccessibilityNodeInfo
import android.view.accessibility.AccessibilityNodeProvider
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import java.nio.ByteBuffer
import newsmobile.Newsmobile
import mobile.Bridge

/**
 * Thin host for the gophics news reader (M9 embedding model): the Go side owns
 * the UI; this activity owns the surface, vsync, input, IME, and intents.
 *
 * Two jobs here are specific to the reader. It hands Go a writable directory
 * before starting, because only the Java side knows where app-private storage
 * lives. And it presents the sign-in WebView for paid sources, because the Go
 * side has no browser and no way to read cookies — see PendingLoginDomain in
 * examples/news/mobile/login.go.
 */
// One bridge per process, at file scope because MainActivity and the surface
// view are separate classes and both drive it. gomobile assumes one anyway:
// Start builds the app once.
private lateinit var bridge: Bridge

class MainActivity : Activity() {
    private var view: GophicsView? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        // Before anything else: where the reader may keep its articles, ranking
        // model, subscriptions and picture cache. Go cannot work this out for
        // itself — os.UserConfigDir on Android points somewhere unwritable.
        Newsmobile.setDataDir(filesDir.absolutePath)
        bridge = Newsmobile.start()
        val v = GophicsView(this)
        view = v
        setContentView(v)
        ViewCompat.setOnApplyWindowInsetsListener(v) { _, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime())
            bridge.setInsets(
                bars.top.toFloat(), bars.right.toFloat(),
                bars.bottom.toFloat(), bars.left.toFloat())
            insets
        }
    }

    // Run states, matching shell.AppState: 0 active, 1 inactive, 2 background.
    // onPause means "no longer frontmost" — a dialog over the app counts —
    // while onStop means "not visible", which is the one worth persisting on.
    override fun onPause() {
        super.onPause()
        bridge.focused(false)
        bridge.setAppState(1)
    }

    override fun onResume() {
        super.onResume()
        bridge.focused(true)
        bridge.setAppState(0)
    }

    override fun onStop() {
        super.onStop()
        bridge.setAppState(2)
    }
}

class GophicsView(private val activity: Activity) :
    SurfaceView(activity), SurfaceHolder.Callback, Choreographer.FrameCallback {

    private var lastNanos = 0L
    private var running = false
    private var imeShown = false
    private var frameCount = 0
    private var frameTimeSum = 0.0
    private var nativeWin = 0L    // ANativeWindow*, from NativeSurface
    private var surfaceSet = false
    // Whether the surface was configured. The per-frame decision asks
    // bridge.gpuActive() instead: a surface can configure successfully and
    // still never present, and caching the answer renders into nothing forever.
    private var gpuConfigured = false
    private var blitBitmap: Bitmap? = null

    init {
        holder.addCallback(this)
        isFocusable = true
        isFocusableInTouchMode = true
        isHapticFeedbackEnabled = true
    }

    // --- IME: the view is a text editor whose InputConnection forwards
    // committed and composing text into the bridge. ---

    override fun onCheckIsTextEditor(): Boolean = true

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        outAttrs.inputType = EditorInfo.TYPE_CLASS_TEXT
        outAttrs.imeOptions = EditorInfo.IME_FLAG_NO_FULLSCREEN
        return object : BaseInputConnection(this, false) {
            override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
                bridge.composition(2, "", 0, "") // end any preedit
                bridge.text(text.toString())
                return true
            }
            override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean {
                bridge.composition(1, text.toString(), text.length.toLong().toInt().toLong(), "")
                return true
            }
            override fun finishComposingText(): Boolean {
                bridge.composition(2, "", 0, "")
                return true
            }
            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
                repeat(beforeLength) { bridge.key(2, true) } // KeyBackspace
                return true
            }
            override fun sendKeyEvent(event: KeyEvent): Boolean {
                if (event.action == KeyEvent.ACTION_DOWN) {
                    when (event.keyCode) {
                        KeyEvent.KEYCODE_DEL -> bridge.key(2, true)
                        KeyEvent.KEYCODE_ENTER -> bridge.key(1, true)
                        else -> {
                            val ch = event.unicodeChar
                            if (ch != 0) bridge.text(String(Character.toChars(ch)))
                        }
                    }
                }
                return true
            }
        }
    }

    private fun syncIME() {
        val want = bridge.textInputActive()
        if (want == imeShown) return
        imeShown = want
        val imm = activity.getSystemService(Activity.INPUT_METHOD_SERVICE) as InputMethodManager
        if (want) {
            requestFocus()
            imm.showSoftInput(this, 0)
        } else {
            imm.hideSoftInputFromWindow(windowToken, 0)
        }
    }

    // --- Surface + frame loop ---

    override fun surfaceCreated(holder: SurfaceHolder) {
        running = true
        val dark = (resources.configuration.uiMode and
            Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
        bridge.setDarkMode(dark)
        nativeWin = NativeSurface.acquire(holder.surface) // ANativeWindow* for the GPU
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, w: Int, h: Int) {
        val d = resources.displayMetrics.density
        // Hand the surface to the Go side once it has a size; the GPU renders
        // directly to it. Resize thereafter reconfigures the surface.
        if (!surfaceSet && nativeWin != 0L) {
            bridge.setSurface(0, nativeWin, w.toLong(), h.toLong(), d)
            gpuConfigured = bridge.gpuActive()
            surfaceSet = true
            Log.i("gophics", if (gpuConfigured) "GPU present" else "GPU unavailable — CPU blit")
        }
        bridge.resize(w.toLong(), h.toLong(), d)
    }

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        running = false
        Choreographer.getInstance().removeFrameCallback(this)
        bridge.clearSurface()
        NativeSurface.release(nativeWin)
        nativeWin = 0L
        surfaceSet = false
    }

    // Hand the Surface back to the CPU after the GPU gives up on it.
    //
    // A Surface can only be driven by one API at a time: while it is connected
    // to Vulkan through the ANativeWindow, holder.lockCanvas throws
    // IllegalArgumentException, so the CPU fallback draws nothing and the screen
    // stays black — with the real cause three stack frames down in a
    // SurfaceHolder log nobody is reading. Disconnecting first is what makes the
    // fallback actually a fallback.
    private fun releaseGPUSurface() {
        if (nativeWin == 0L) return
        bridge.clearSurface()
        NativeSurface.release(nativeWin)
        nativeWin = 0L
        surfaceSet = false
        gpuConfigured = false
        Log.i("gophics", "GPU surface released — presenting on the CPU")
    }

    override fun doFrame(frameNanos: Long) {
        if (!running) return
        val dt = if (lastNanos == 0L) 1.0 / 60 else (frameNanos - lastNanos) / 1e9
        lastNanos = frameNanos

        if (bridge.needsFrame()) {
            val t0 = System.nanoTime()
            if (bridge.gpuActive()) {
                bridge.renderFrame(dt) // GPU-presents directly to the ANativeWindow
            } else {
                releaseGPUSurface() // no-op unless the GPU just retired
                presentCPU(dt) // emulator / no GPU: rasterize on CPU and blit
            }
            // Frame pacing: log a rolling average every 60 rendered frames.
            frameTimeSum += (System.nanoTime() - t0) / 1e6
            if (++frameCount == 60) {
                Log.i("gophics", "avg frame %.2f ms over 60 frames".format(frameTimeSum / 60))
                frameCount = 0
                frameTimeSum = 0.0
            }
            while (true) {
                val url = bridge.takeOpenedURL()
                if (url.isEmpty()) break
                activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            }
            while (true) {
                val h = bridge.takeHaptic().toInt()
                if (h < 0) break
                playHaptic(h)
            }
            // The reader asking to sign in to a publisher. Polled rather than
            // pushed because the bind surface only calls into Go, never out.
            val loginDomain = Newsmobile.pendingLoginDomain()
            if (loginDomain.isNotEmpty()) {
                val loginURL = Newsmobile.pendingLoginURL()
                Newsmobile.clearPendingLogin()
                activity.runOnUiThread { LoginSheet.show(activity, loginDomain, loginURL) }
            }
        }
        syncIME()
        Choreographer.getInstance().postFrameCallback(this)
    }

    // playHaptic maps a gophics shell.HapticKind (drained from the bridge) to the
    // closest Android performHapticFeedback constant. FLAG_IGNORE_VIEW_SETTING
    // fires it even if the view's own haptic flag is off; the system "touch
    // feedback" setting still gates it, which is the behaviour we want.
    private fun playHaptic(kind: Int) {
        val effect = when (kind) {
            0 -> HapticFeedbackConstants.CLOCK_TICK    // selection — light tick
            1 -> HapticFeedbackConstants.KEYBOARD_TAP  // light impact
            2 -> HapticFeedbackConstants.CONTEXT_CLICK // medium impact
            3 -> HapticFeedbackConstants.LONG_PRESS    // heavy impact
            4 -> HapticFeedbackConstants.CONFIRM       // success
            5 -> HapticFeedbackConstants.REJECT        // warning
            6 -> HapticFeedbackConstants.REJECT        // error
            else -> HapticFeedbackConstants.VIRTUAL_KEY
        }
        performHapticFeedback(effect, HapticFeedbackConstants.FLAG_IGNORE_VIEW_SETTING)
    }

    // presentCPU rasterizes one frame on the Go side (bridge.snapshot →
    // RGBA8888) and blits it into the SurfaceView with lockCanvas — the present
    // path when the GPU surface is unavailable (emulator). The frame is
    // physical-pixel sized, matching the surface, so it blits 1:1.
    private fun presentCPU(dt: Double) {
        val px = bridge.snapshot(dt) ?: return
        val w = bridge.frameWidth().toInt(); val h = bridge.frameHeight().toInt()
        if (w == 0 || h == 0 || px.size < w * h * 4) return
        var bmp = blitBitmap
        if (bmp == null || bmp.width != w || bmp.height != h) {
            bmp = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
            blitBitmap = bmp
        }
        bmp.copyPixelsFromBuffer(ByteBuffer.wrap(px))
        val canvas = holder.lockCanvas() ?: return
        try {
            canvas.drawBitmap(bmp, 0f, 0f, null)
        } finally {
            holder.unlockCanvasAndPost(canvas)
        }
    }

    // --- Physical keys ---
    //
    // A hardware keyboard does not go through the InputConnection: injected and
    // physical key events are dispatched to the View, and a view that only
    // implements an InputConnection silently drops every one of them. On a
    // tablet with a keyboard case, a Chromebook, or DeX, that is an app you
    // cannot type into — and it looks like the app is frozen rather than like a
    // missing handler. `adb shell input text` lands here too, which is how this
    // was noticed.
    //
    // Codes are shell.KeyCode: 1 Enter, 2 Backspace, 3 Delete, 4 Escape,
    // 5 Tab, 6 Left, 7 Right, 8 Up, 9 Down, 10 Home, 11 End.
    override fun onKeyDown(keyCode: Int, event: KeyEvent): Boolean {
        val code = when (keyCode) {
            KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> 1L
            KeyEvent.KEYCODE_DEL -> 2L
            KeyEvent.KEYCODE_FORWARD_DEL -> 3L
            KeyEvent.KEYCODE_ESCAPE -> 4L
            KeyEvent.KEYCODE_TAB -> 5L
            KeyEvent.KEYCODE_DPAD_LEFT -> 6L
            KeyEvent.KEYCODE_DPAD_RIGHT -> 7L
            KeyEvent.KEYCODE_DPAD_UP -> 8L
            KeyEvent.KEYCODE_DPAD_DOWN -> 9L
            KeyEvent.KEYCODE_MOVE_HOME -> 10L
            KeyEvent.KEYCODE_MOVE_END -> 11L
            else -> 0L
        }
        if (code != 0L) {
            bridge.key(code, true)
            return true
        }
        // Printable characters arrive as text, not as key codes — the same
        // split the Go side makes, where plain typing is Text and only
        // shortcuts are keys.
        val ch = event.unicodeChar
        if (ch != 0) {
            bridge.text(String(Character.toChars(ch)))
            return true
        }
        return super.onKeyDown(keyCode, event)
    }

    override fun onKeyUp(keyCode: Int, event: KeyEvent): Boolean {
        // The Go side tracks press/release for modifiers; releases for the keys
        // above are reported so a held arrow does not look stuck down.
        val code = when (keyCode) {
            KeyEvent.KEYCODE_ENTER, KeyEvent.KEYCODE_NUMPAD_ENTER -> 1L
            KeyEvent.KEYCODE_DEL -> 2L
            KeyEvent.KEYCODE_FORWARD_DEL -> 3L
            KeyEvent.KEYCODE_ESCAPE -> 4L
            KeyEvent.KEYCODE_TAB -> 5L
            KeyEvent.KEYCODE_DPAD_LEFT -> 6L
            KeyEvent.KEYCODE_DPAD_RIGHT -> 7L
            KeyEvent.KEYCODE_DPAD_UP -> 8L
            KeyEvent.KEYCODE_DPAD_DOWN -> 9L
            KeyEvent.KEYCODE_MOVE_HOME -> 10L
            KeyEvent.KEYCODE_MOVE_END -> 11L
            else -> 0L
        }
        if (code != 0L) {
            bridge.key(code, false)
            return true
        }
        return super.onKeyUp(keyCode, event)
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val phase = when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> 0L
            MotionEvent.ACTION_MOVE -> 1L
            MotionEvent.ACTION_UP -> 2L
            else -> 3L
        }
        bridge.touch(phase, e.x.toFloat(), e.y.toFloat())
        return true
    }

    // --- Accessibility: expose gophics's semantics tree to TalkBack as a
    // virtual view hierarchy (the Go side owns the pixels, so there are no
    // real child Views). ---

    override fun getAccessibilityNodeProvider(): AccessibilityNodeProvider = a11yProvider

    private val idToIndex = HashMap<Int, Int>()
    private var rootId = -1

    private fun refreshA11y() {
        val count = bridge.a11yRefresh().toInt()
        idToIndex.clear()
        rootId = -1
        for (i in 0 until count) {
            val id = bridge.a11yID(i.toLong()).toInt()
            idToIndex[id] = i
            if (bridge.a11yParent(i.toLong()).toInt() == -1) rootId = id
        }
    }

    private val a11yProvider = object : AccessibilityNodeProvider() {
        override fun createAccessibilityNodeInfo(virtualViewId: Int): AccessibilityNodeInfo? {
            val loc = IntArray(2)
            getLocationOnScreen(loc)

            if (virtualViewId == HOST_VIEW_ID) {
                refreshA11y()
                val info = AccessibilityNodeInfo.obtain(this@GophicsView)
                onInitializeAccessibilityNodeInfo(info)
                val ri = idToIndex[rootId]
                if (ri != null) {
                    val cc = bridge.a11yChildCount(ri.toLong()).toInt()
                    for (j in 0 until cc) {
                        info.addChild(this@GophicsView, bridge.a11yChild(ri.toLong(), j.toLong()).toInt())
                    }
                }
                return info
            }

            val idx = idToIndex[virtualViewId] ?: return null
            val i = idx.toLong()
            val info = AccessibilityNodeInfo.obtain(this@GophicsView, virtualViewId)
            info.packageName = context.packageName
            info.className = "gophics." + bridge.a11yRole(i)
            val label = bridge.a11yLabel(i)
            val value = bridge.a11yValue(i)
            info.contentDescription = if (value.isNotEmpty()) "$label, $value" else label
            val x = bridge.a11yX(i).toInt()
            val y = bridge.a11yY(i).toInt()
            info.setBoundsInScreen(Rect(loc[0] + x, loc[1] + y,
                loc[0] + x + bridge.a11yW(i).toInt(), loc[1] + y + bridge.a11yH(i).toInt()))
            info.isVisibleToUser = true
            info.isFocusable = true
            val parent = bridge.a11yParent(i).toInt()
            info.setParent(this@GophicsView, if (parent == rootId) HOST_VIEW_ID else parent)
            if (bridge.a11yTappable(i)) {
                info.isClickable = true
                info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLICK)
            }
            val cc = bridge.a11yChildCount(i).toInt()
            for (j in 0 until cc) {
                info.addChild(this@GophicsView, bridge.a11yChild(i, j.toLong()).toInt())
            }
            return info
        }

        override fun performAction(virtualViewId: Int, action: Int, arguments: android.os.Bundle?): Boolean {
            if (action == AccessibilityNodeInfo.ACTION_CLICK) {
                bridge.a11yActivate(virtualViewId.toLong())
                return true
            }
            return false
        }
    }
}
