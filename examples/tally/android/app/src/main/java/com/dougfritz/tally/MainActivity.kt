package com.dougfritz.tally

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
import tallymobile.Tallymobile

/**
 * Thin host for the Tally app (M9 embedding model): the Go side owns
 * the UI; this activity owns the surface, vsync, input, IME, and intents.
 */
class MainActivity : Activity() {
    private var view: GophicsView? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val err = Tallymobile.start()
        if (err.isNotEmpty()) throw RuntimeException("gophics start: $err")
        val v = GophicsView(this)
        view = v
        setContentView(v)
        ViewCompat.setOnApplyWindowInsetsListener(v) { _, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime())
            Tallymobile.setInsets(
                bars.top.toDouble(), bars.right.toDouble(),
                bars.bottom.toDouble(), bars.left.toDouble())
            insets
        }
    }

    override fun onPause() {
        super.onPause()
        Tallymobile.focused(false)
    }

    override fun onResume() {
        super.onResume()
        Tallymobile.focused(true)
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
    private var gpuActive = false  // false → CPU-blit present (emulator, GPU-less)
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
                Tallymobile.composition(2, "", 0, "") // end any preedit
                Tallymobile.text(text.toString())
                return true
            }
            override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean {
                Tallymobile.composition(1, text.toString(), text.length.toLong().toInt().toLong(), "")
                return true
            }
            override fun finishComposingText(): Boolean {
                Tallymobile.composition(2, "", 0, "")
                return true
            }
            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
                repeat(beforeLength) { Tallymobile.key(2, true) } // KeyBackspace
                return true
            }
            override fun sendKeyEvent(event: KeyEvent): Boolean {
                if (event.action == KeyEvent.ACTION_DOWN) {
                    when (event.keyCode) {
                        KeyEvent.KEYCODE_DEL -> Tallymobile.key(2, true)
                        KeyEvent.KEYCODE_ENTER -> Tallymobile.key(1, true)
                        else -> {
                            val ch = event.unicodeChar
                            if (ch != 0) Tallymobile.text(String(Character.toChars(ch)))
                        }
                    }
                }
                return true
            }
        }
    }

    private fun syncIME() {
        val want = Tallymobile.textInputActive()
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
        Tallymobile.setDarkMode(dark)
        nativeWin = NativeSurface.acquire(holder.surface) // ANativeWindow* for the GPU
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, w: Int, h: Int) {
        val d = resources.displayMetrics.density.toDouble()
        // Hand the surface to the Go side once it has a size; the GPU renders
        // directly to it. Resize thereafter reconfigures the surface.
        if (!surfaceSet && nativeWin != 0L) {
            Tallymobile.setSurface(0, nativeWin, w.toLong(), h.toLong(), d)
            gpuActive = Tallymobile.gpuActive()
            surfaceSet = true
            Log.i("gophics", if (gpuActive) "GPU present" else "GPU unavailable — CPU blit")
        }
        Tallymobile.resize(w.toLong(), h.toLong(), d)
    }

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        running = false
        Choreographer.getInstance().removeFrameCallback(this)
        Tallymobile.clearSurface()
        NativeSurface.release(nativeWin)
        nativeWin = 0L
        surfaceSet = false
    }

    override fun doFrame(frameNanos: Long) {
        if (!running) return
        val dt = if (lastNanos == 0L) 1.0 / 60 else (frameNanos - lastNanos) / 1e9
        lastNanos = frameNanos

        if (Tallymobile.needsFrame()) {
            val t0 = System.nanoTime()
            if (gpuActive) {
                Tallymobile.renderFrame(dt) // GPU-presents directly to the ANativeWindow
            } else {
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
                val url = Tallymobile.takeOpenedURL()
                if (url.isEmpty()) break
                activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            }
            while (true) {
                val h = Tallymobile.takeHaptic().toInt()
                if (h < 0) break
                playHaptic(h)
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

    // presentCPU rasterizes one frame on the Go side (Tallymobile.snapshot →
    // RGBA8888) and blits it into the SurfaceView with lockCanvas — the present
    // path when the GPU surface is unavailable (emulator). The frame is
    // physical-pixel sized, matching the surface, so it blits 1:1.
    private fun presentCPU(dt: Double) {
        val px = Tallymobile.snapshot(dt) ?: return
        val w = Tallymobile.frameWidth().toInt(); val h = Tallymobile.frameHeight().toInt()
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

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val phase = when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> 0L
            MotionEvent.ACTION_MOVE -> 1L
            MotionEvent.ACTION_UP -> 2L
            else -> 3L
        }
        Tallymobile.touch(phase, e.x.toDouble(), e.y.toDouble())
        return true
    }

    // --- Accessibility: expose gophics's semantics tree to TalkBack as a
    // virtual view hierarchy (the Go side owns the pixels, so there are no
    // real child Views). ---

    override fun getAccessibilityNodeProvider(): AccessibilityNodeProvider = a11yProvider

    private val idToIndex = HashMap<Int, Int>()
    private var rootId = -1

    private fun refreshA11y() {
        val count = Tallymobile.a11yRefresh().toInt()
        idToIndex.clear()
        rootId = -1
        for (i in 0 until count) {
            val id = Tallymobile.a11yID(i.toLong()).toInt()
            idToIndex[id] = i
            if (Tallymobile.a11yParent(i.toLong()).toInt() == -1) rootId = id
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
                    val cc = Tallymobile.a11yChildCount(ri.toLong()).toInt()
                    for (j in 0 until cc) {
                        info.addChild(this@GophicsView, Tallymobile.a11yChild(ri.toLong(), j.toLong()).toInt())
                    }
                }
                return info
            }

            val idx = idToIndex[virtualViewId] ?: return null
            val i = idx.toLong()
            val info = AccessibilityNodeInfo.obtain(this@GophicsView, virtualViewId)
            info.packageName = context.packageName
            info.className = "gophics." + Tallymobile.a11yRole(i)
            val label = Tallymobile.a11yLabel(i)
            val value = Tallymobile.a11yValue(i)
            info.contentDescription = if (value.isNotEmpty()) "$label, $value" else label
            val x = Tallymobile.a11yX(i).toInt()
            val y = Tallymobile.a11yY(i).toInt()
            info.setBoundsInScreen(Rect(loc[0] + x, loc[1] + y,
                loc[0] + x + Tallymobile.a11yW(i).toInt(), loc[1] + y + Tallymobile.a11yH(i).toInt()))
            info.isVisibleToUser = true
            info.isFocusable = true
            val parent = Tallymobile.a11yParent(i).toInt()
            info.setParent(this@GophicsView, if (parent == rootId) HOST_VIEW_ID else parent)
            if (Tallymobile.a11yTappable(i)) {
                info.isClickable = true
                info.addAction(AccessibilityNodeInfo.AccessibilityAction.ACTION_CLICK)
            }
            val cc = Tallymobile.a11yChildCount(i).toInt()
            for (j in 0 until cc) {
                info.addChild(this@GophicsView, Tallymobile.a11yChild(i, j.toLong()).toInt())
            }
            return info
        }

        override fun performAction(virtualViewId: Int, action: Int, arguments: android.os.Bundle?): Boolean {
            if (action == AccessibilityNodeInfo.ACTION_CLICK) {
                Tallymobile.a11yActivate(virtualViewId.toLong())
                return true
            }
            return false
        }
    }
}
