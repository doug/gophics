package dev.gossamer.hn

import android.app.Activity
import android.content.Intent
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.net.Uri
import android.os.Bundle
import android.view.Choreographer
import android.view.MotionEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import java.nio.ByteBuffer
import hnmobile.Hnmobile

/**
 * Thin host for the gossamer HN app (M9 embedding model): the Go side owns
 * the UI; this activity owns the surface, vsync, and input, blitting the
 * bridge's RGBA frames into a SurfaceView.
 */
class MainActivity : Activity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val err = Hnmobile.start()
        if (err.isNotEmpty()) throw RuntimeException("gossamer start: $err")
        setContentView(GossamerView(this))
    }
}

class GossamerView(private val activity: Activity) :
    SurfaceView(activity), SurfaceHolder.Callback, Choreographer.FrameCallback {

    private var bitmap: Bitmap? = null
    private var lastNanos = 0L
    private var running = false

    init {
        holder.addCallback(this)
        isFocusable = true
        isFocusableInTouchMode = true
    }

    override fun surfaceCreated(holder: SurfaceHolder) {
        running = true
        val dark = (resources.configuration.uiMode and
            Configuration.UI_MODE_NIGHT_MASK) == Configuration.UI_MODE_NIGHT_YES
        Hnmobile.setDarkMode(dark)
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, w: Int, h: Int) {
        Hnmobile.resize(w.toLong(), h.toLong(), resources.displayMetrics.density.toDouble())
        bitmap = Bitmap.createBitmap(w, h, Bitmap.Config.ARGB_8888)
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
            val pixels = Hnmobile.renderFrame(dt)
            val bmp = bitmap
            if (pixels != null && bmp != null) {
                bmp.copyPixelsFromBuffer(ByteBuffer.wrap(pixels))
                val canvas: Canvas? = holder.lockCanvas()
                if (canvas != null) {
                    canvas.drawBitmap(bmp, 0f, 0f, null)
                    holder.unlockCanvasAndPost(canvas)
                }
            }
            // The UI may have asked to open links.
            while (true) {
                val url = Hnmobile.takeOpenedURL()
                if (url.isEmpty()) break
                activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(url)))
            }
        }
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
