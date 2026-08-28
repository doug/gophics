package dev.gophics.health

import android.app.Activity
import android.graphics.Bitmap
import android.os.Bundle
import android.util.Log
import android.view.Choreographer
import android.view.MotionEvent
import android.view.SurfaceHolder
import android.view.SurfaceView
import androidx.activity.ComponentActivity
import androidx.health.connect.client.HealthConnectClient
import androidx.health.connect.client.PermissionController
import androidx.health.connect.client.permission.HealthPermission
import androidx.health.connect.client.records.HeartRateRecord
import androidx.health.connect.client.records.SleepSessionRecord
import androidx.health.connect.client.records.StepsRecord
import androidx.health.connect.client.records.WeightRecord
import androidx.health.connect.client.request.ReadRecordsRequest
import androidx.health.connect.client.time.TimeRangeFilter
import androidx.lifecycle.Lifecycle
import androidx.lifecycle.lifecycleScope
import androidx.lifecycle.repeatOnLifecycle
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import java.nio.ByteBuffer
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.temporal.ChronoUnit
import healthmobile.Healthmobile
import mobile.Bridge

/**
 * Host for the gophics health dashboard: the Go side (package healthui) owns the
 * UI; this activity owns the surface, vsync, and touch, and feeds it real
 * on-device data read from Health Connect. Metric codes match healthui.Metric:
 * 0 heart rate, 1 steps, 2 weight, 3 sleep.
 */
// One bridge per process, at file scope because MainActivity and the surface
// view are separate classes and both drive it. gomobile assumes one anyway:
// Start builds the app once.
private lateinit var bridge: Bridge

class MainActivity : ComponentActivity() {
    private var healthClient: HealthConnectClient? = null

    private val permissions = setOf(
        HealthPermission.getReadPermission(HeartRateRecord::class),
        HealthPermission.getReadPermission(StepsRecord::class),
        HealthPermission.getReadPermission(WeightRecord::class),
        HealthPermission.getReadPermission(SleepSessionRecord::class),
    )

    // Health Connect's own permission sheet; result is the granted set.
    private val requestPerms = registerForActivityResult(
        PermissionController.createRequestPermissionResultContract()
    ) { granted ->
        val client = healthClient
        if (client != null && granted.containsAll(permissions)) {
            startFeeds(client)
        } else {
            Healthmobile.setAuthorized(false)
            Log.w("gophics", "Health Connect permissions not granted")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        bridge = Healthmobile.start("Health Connect")
        setContentView(GophicsView(this))

        when (HealthConnectClient.getSdkStatus(this)) {
            HealthConnectClient.SDK_AVAILABLE -> {
                val client = HealthConnectClient.getOrCreate(this)
                healthClient = client
                lifecycleScope.launch {
                    if (client.permissionController.getGrantedPermissions().containsAll(permissions)) {
                        startFeeds(client)
                    } else {
                        requestPerms.launch(permissions)
                    }
                }
            }
            else -> Log.w("gophics", "Health Connect unavailable (status=" +
                "${HealthConnectClient.getSdkStatus(this)}) — showing empty dashboard")
        }
    }

    /** pump reads each metric once and backfills the Go provider. The bind's
     *  batch PushSeries takes a []float64, which gomobile can't bind, so we feed
     *  each series point-by-point with the scalar PushSample (append order =
     *  oldest→newest; the provider is fresh each launch). */
    private suspend fun pump(client: HealthConnectClient) {
        Healthmobile.setAuthorized(true)
        val now = Instant.now()
        val zone = ZoneId.systemDefault()

        // Steps today, cumulative by hour → metric 1 (T = hour, V = cumulative).
        try {
            val startOfDay = LocalDate.now(zone).atStartOfDay(zone).toInstant()
            val recs = client.readRecords(
                ReadRecordsRequest(StepsRecord::class, TimeRangeFilter.between(startOfDay, now))
            ).records
            val perHour = DoubleArray(25)
            for (r in recs) {
                val h = ((r.startTime.epochSecond - startOfDay.epochSecond) / 3600).toInt().coerceIn(0, 24)
                perHour[h] += r.count.toDouble()
            }
            val hoursNow = ((now.epochSecond - startOfDay.epochSecond) / 3600).toInt().coerceIn(0, 24)
            var cum = 0.0
            for (h in 0..hoursNow) { cum += perHour[h]; Healthmobile.pushSample(1, h.toDouble(), cum, 0) }
            Log.i("gophics", "steps: ${recs.size} records, ${cum.toInt()} today")
        } catch (e: Exception) { Log.w("gophics", "steps read: ${e.message}") }

        // Weight, last 90 days → metric 2 (T = -daysAgo, V = kg).
        try {
            val recs = client.readRecords(
                ReadRecordsRequest(WeightRecord::class,
                    TimeRangeFilter.between(now.minus(90, ChronoUnit.DAYS), now))
            ).records.sortedBy { it.time }
            for (r in recs) {
                val t = -(now.epochSecond - r.time.epochSecond) / 86_400.0
                Healthmobile.pushSample(2, t, r.weight.inKilograms, 0)
            }
            Log.i("gophics", "weight: ${recs.size} records")
        } catch (e: Exception) { Log.w("gophics", "weight read: ${e.message}") }

        // Sleep, last 30 days → metric 3 (T = -nightsAgo, V = hours slept).
        try {
            val recs = client.readRecords(
                ReadRecordsRequest(SleepSessionRecord::class,
                    TimeRangeFilter.between(now.minus(30, ChronoUnit.DAYS), now))
            ).records.sortedBy { it.startTime }
            val today = LocalDate.now(zone)
            for (r in recs) {
                val t = -(ChronoUnit.DAYS.between(r.endTime.atZone(zone).toLocalDate(), today)).toDouble()
                val hrs = (r.endTime.epochSecond - r.startTime.epochSecond) / 3600.0
                Healthmobile.pushSample(3, t, hrs, 0)
            }
            Log.i("gophics", "sleep: ${recs.size} sessions")
        } catch (e: Exception) { Log.w("gophics", "sleep read: ${e.message}") }

    }

    /** startFeeds backfills steps/weight/sleep once, then polls heart rate live
     *  for as long as the activity is started. */
    private fun startFeeds(client: HealthConnectClient) {
        lifecycleScope.launch {
            pump(client)
            repeatOnLifecycle(Lifecycle.State.STARTED) {
                while (true) {
                    pollHeartRate(client)
                    delay(2500)
                }
            }
        }
    }

    // pollHeartRate re-reads the last 60 minutes of heart rate each tick and
    // pushes the most recent up-to-60 readings as metric 0. The chart plots by
    // index and PushSample(capN=60) trims to the last 60, so each poll refreshes
    // the window in place — genuine bpm, scrolling as new samples land. (Needs a
    // watch feeding Health Connect; with no source it simply reads 0.)
    private suspend fun pollHeartRate(client: HealthConnectClient) {
        try {
            val now = Instant.now()
            val recs = client.readRecords(
                ReadRecordsRequest(HeartRateRecord::class,
                    TimeRangeFilter.between(now.minus(60, ChronoUnit.MINUTES), now))
            ).records
            val bpms = ArrayList<Double>()
            for (rec in recs) for (s in rec.samples) bpms.add(s.beatsPerMinute.toDouble())
            val last = bpms.takeLast(60)
            val base = last.size - 1
            for ((i, bpm) in last.withIndex()) Healthmobile.pushSample(0, (i - base).toDouble(), bpm, 60)
        } catch (e: Exception) { Log.w("gophics", "heart rate poll: ${e.message}") }
    }
}

class GophicsView(private val activity: Activity) :
    SurfaceView(activity), SurfaceHolder.Callback, Choreographer.FrameCallback {

    private var lastNanos = 0L
    private var running = false
    private var nativeWin = 0L
    private var surfaceSet = false
    // See the note in the other hosts: the per-frame decision asks the bridge.
    private var gpuConfigured = false
    private var blitBitmap: Bitmap? = null

    init {
        holder.addCallback(this)
        isFocusable = true
        isFocusableInTouchMode = true
    }

    override fun surfaceCreated(holder: SurfaceHolder) {
        running = true
        nativeWin = NativeSurface.acquire(holder.surface)
        Choreographer.getInstance().postFrameCallback(this)
    }

    override fun surfaceChanged(holder: SurfaceHolder, format: Int, w: Int, h: Int) {
        val d = resources.displayMetrics.density
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
            if (bridge.gpuActive()) {
                bridge.renderFrame(dt)
            } else {
                releaseGPUSurface() // no-op unless the GPU just retired
                presentCPU(dt)
            }
        }
        Choreographer.getInstance().postFrameCallback(this)
    }

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
}
