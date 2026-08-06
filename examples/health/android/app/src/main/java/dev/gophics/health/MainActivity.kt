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
import androidx.lifecycle.lifecycleScope
import kotlinx.coroutines.launch
import java.nio.ByteBuffer
import java.time.Instant
import java.time.LocalDate
import java.time.ZoneId
import java.time.temporal.ChronoUnit
import healthmobile.Healthmobile

/**
 * Host for the gophics health dashboard: the Go side (package healthui) owns the
 * UI; this activity owns the surface, vsync, and touch, and feeds it real
 * on-device data read from Health Connect. Metric codes match healthui.Metric:
 * 0 heart rate, 1 steps, 2 weight, 3 sleep.
 */
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
            lifecycleScope.launch { pump(client) }
        } else {
            Healthmobile.setAuthorized(false)
            Log.w("gophics", "Health Connect permissions not granted")
        }
    }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val err = Healthmobile.start("Health Connect")
        if (err.isNotEmpty()) throw RuntimeException("gophics start: $err")
        setContentView(GophicsView(this))

        when (HealthConnectClient.getSdkStatus(this)) {
            HealthConnectClient.SDK_AVAILABLE -> {
                val client = HealthConnectClient.getOrCreate(this)
                healthClient = client
                lifecycleScope.launch {
                    val granted = client.permissionController.getGrantedPermissions()
                    if (granted.containsAll(permissions)) {
                        pump(client)
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

        // Heart rate → metric 0. The UI's HR chart is a live 60-second window
        // (T in seconds), but Health Connect gives historical samples; place the
        // most recent up-to-60 real readings one-per-second ending at now so the
        // widget shows a live-looking trace of genuine bpm values.
        try {
            val recs = client.readRecords(
                ReadRecordsRequest(HeartRateRecord::class,
                    TimeRangeFilter.between(now.minus(6, ChronoUnit.HOURS), now))
            ).records
            val bpms = ArrayList<Double>()
            for (rec in recs) for (s in rec.samples) bpms.add(s.beatsPerMinute.toDouble())
            val last = bpms.takeLast(60)
            val base = last.size - 1
            for ((i, bpm) in last.withIndex()) Healthmobile.pushSample(0, (i - base).toDouble(), bpm, 60)
            Log.i("gophics", "heart rate: ${bpms.size} samples, showing ${last.size}")
        } catch (e: Exception) { Log.w("gophics", "heart rate read: ${e.message}") }
    }
}

class GophicsView(private val activity: Activity) :
    SurfaceView(activity), SurfaceHolder.Callback, Choreographer.FrameCallback {

    private var lastNanos = 0L
    private var running = false
    private var nativeWin = 0L
    private var surfaceSet = false
    private var gpuActive = false
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
        val d = resources.displayMetrics.density.toDouble()
        if (!surfaceSet && nativeWin != 0L) {
            Healthmobile.setSurface(0, nativeWin, w.toLong(), h.toLong(), d)
            gpuActive = Healthmobile.gpuActive()
            surfaceSet = true
            Log.i("gophics", if (gpuActive) "GPU present" else "GPU unavailable — CPU blit")
        }
        Healthmobile.resize(w.toLong(), h.toLong(), d)
    }

    override fun surfaceDestroyed(holder: SurfaceHolder) {
        running = false
        Choreographer.getInstance().removeFrameCallback(this)
        Healthmobile.clearSurface()
        NativeSurface.release(nativeWin)
        nativeWin = 0L
        surfaceSet = false
    }

    override fun doFrame(frameNanos: Long) {
        if (!running) return
        val dt = if (lastNanos == 0L) 1.0 / 60 else (frameNanos - lastNanos) / 1e9
        lastNanos = frameNanos
        if (Healthmobile.needsFrame()) {
            if (gpuActive) Healthmobile.renderFrame(dt) else presentCPU(dt)
        }
        Choreographer.getInstance().postFrameCallback(this)
    }

    private fun presentCPU(dt: Double) {
        val px = Healthmobile.snapshot(dt) ?: return
        val w = Healthmobile.frameWidth().toInt(); val h = Healthmobile.frameHeight().toInt()
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
        Healthmobile.touch(phase, e.x.toDouble(), e.y.toDouble())
        return true
    }
}
