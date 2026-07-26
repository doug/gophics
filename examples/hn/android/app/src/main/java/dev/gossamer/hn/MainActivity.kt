package dev.gossamer.hn

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
import android.view.KeyEvent
import android.view.MotionEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import android.view.inputmethod.BaseInputConnection
import android.view.inputmethod.EditorInfo
import android.view.inputmethod.InputConnection
import android.view.inputmethod.InputMethodManager
import androidx.core.view.ViewCompat
import androidx.core.view.WindowInsetsCompat
import java.nio.ByteBuffer
import hnmobile.Hnmobile

/**
 * Thin host for the gossamer HN app (M9 embedding model): the Go side owns
 * the UI; this activity owns the surface, vsync, input, IME, and intents.
 */
class MainActivity : Activity() {
    private var view: GossamerView? = null

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val err = Hnmobile.start()
        if (err.isNotEmpty()) throw RuntimeException("gossamer start: $err")
        val v = GossamerView(this)
        view = v
        setContentView(v)
        ViewCompat.setOnApplyWindowInsetsListener(v) { _, insets ->
            val bars = insets.getInsets(
                WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.ime())
            Hnmobile.setInsets(
                bars.top.toDouble(), bars.right.toDouble(),
                bars.bottom.toDouble(), bars.left.toDouble())
            insets
        }
    }

    override fun onPause() {
        super.onPause()
        Hnmobile.focused(false)
    }

    override fun onResume() {
        super.onResume()
        Hnmobile.focused(true)
    }
}

class GossamerView(private val activity: Activity) :
    SurfaceView(activity), SurfaceHolder.Callback, Choreographer.FrameCallback {

    private var bitmap: Bitmap? = null
    private var lastNanos = 0L
    private var running = false
    private var imeShown = false
    private var frameCount = 0
    private var frameTimeSum = 0.0

    init {
        holder.addCallback(this)
        isFocusable = true
        isFocusableInTouchMode = true
    }

    // --- IME: the view is a text editor whose InputConnection forwards
    // committed and composing text into the bridge. ---

    override fun onCheckIsTextEditor(): Boolean = true

    override fun onCreateInputConnection(outAttrs: EditorInfo): InputConnection {
        outAttrs.inputType = EditorInfo.TYPE_CLASS_TEXT
        outAttrs.imeOptions = EditorInfo.IME_FLAG_NO_FULLSCREEN
        return object : BaseInputConnection(this, false) {
            override fun commitText(text: CharSequence, newCursorPosition: Int): Boolean {
                Hnmobile.composition(2, "", 0, "") // end any preedit
                Hnmobile.text(text.toString())
                return true
            }
            override fun setComposingText(text: CharSequence, newCursorPosition: Int): Boolean {
                Hnmobile.composition(1, text.toString(), text.length.toLong().toInt().toLong(), "")
                return true
            }
            override fun finishComposingText(): Boolean {
                Hnmobile.composition(2, "", 0, "")
                return true
            }
            override fun deleteSurroundingText(beforeLength: Int, afterLength: Int): Boolean {
                repeat(beforeLength) { Hnmobile.key(2, true) } // KeyBackspace
                return true
            }
            override fun sendKeyEvent(event: KeyEvent): Boolean {
                if (event.action == KeyEvent.ACTION_DOWN) {
                    when (event.keyCode) {
                        KeyEvent.KEYCODE_DEL -> Hnmobile.key(2, true)
                        KeyEvent.KEYCODE_ENTER -> Hnmobile.key(1, true)
                        else -> {
                            val ch = event.unicodeChar
                            if (ch != 0) Hnmobile.text(String(Character.toChars(ch)))
                        }
                    }
                }
                return true
            }
        }
    }

    private fun syncIME() {
        val want = Hnmobile.textInputActive()
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
        Hnmobile.setDarkMode(dark)
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, w: Int, h: Int) {
        Hnmobile.resize(w.toLong(), h.toLong(), resources.displayMetrics.density.toDouble())
        bitmap = null // recreated at the frame's exact pixel size
    }

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        running = false
        Choreographer.getInstance().removeFrameCallback(this)
    }

    override fun doFrame(frameNanos: Long) {
        if (!running) return
        val dt = if (lastNanos == 0L) 1.0 / 60 else (frameNanos - lastNanos) / 1e9
        lastNanos = frameNanos

        if (Hnmobile.needsFrame()) {
            val t0 = System.nanoTime()
            val pixels = Hnmobile.renderFrame(dt)
            val fw = Hnmobile.frameWidth().toInt()
            val fh = Hnmobile.frameHeight().toInt()
            if (pixels != null && fw > 0 && fh > 0) {
                var bmp = bitmap
                if (bmp == null || bmp.width != fw || bmp.height != fh) {
                    bmp = Bitmap.createBitmap(fw, fh, Bitmap.Config.ARGB_8888)
                    bitmap = bmp
                }
                bmp.copyPixelsFromBuffer(ByteBuffer.wrap(pixels))
                val canvas: Canvas? = holder.lockCanvas()
                if (canvas != null) {
                    canvas.drawBitmap(bmp, null, Rect(0, 0, width, height), null)
                    holder.unlockCanvasAndPost(canvas)
                }
            }
            // Frame pacing: log a rolling average every 60 rendered frames.
            frameTimeSum += (System.nanoTime() - t0) / 1e6
            if (++frameCount == 60) {
                Log.i("gossamer", "avg frame %.2f ms over 60 frames".format(frameTimeSum / 60))
                frameCount = 0
                frameTimeSum = 0.0
            }
            while (true) {
                val url = Hnmobile.takeOpenedURL()
                if (url.isEmpty()) break
                activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            }
        }
        syncIME()
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun onTouchEvent(e: MotionEvent): Boolean {
        val phase = when (e.actionMasked) {
            MotionEvent.ACTION_DOWN -> 0L
            MotionEvent.ACTION_MOVE -> 1L
            MotionEvent.ACTION_UP -> 2L
            else -> 3L
        }
        Hnmobile.touch(phase, e.x.toDouble(), e.y.toDouble())
        return true
    }
}
