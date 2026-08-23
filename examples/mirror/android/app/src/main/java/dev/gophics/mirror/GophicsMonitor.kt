// Reference Android MonitorHost for gophics's shell/mobile live-capture Bind.
//
// Wire once at startup:  Bind.setMonitorHost(GophicsMonitor(activity, bridge))
// and route the runtime-permission result to onPermissionResult().
//
// This is the streaming counterpart to GophicsMedia: MediaHost records one clip
// and hands it back when it stops, while MonitorHost keeps an open stream that
// the Go side analyses live. A tuner or a singing app needs the latter — the
// audio has to be current, not eventual.

package dev.gophics.mirror

import android.Manifest
import android.app.Activity
import android.content.pm.PackageManager
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaRecorder
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.os.Process
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import bind.Bind
import bind.MonitorHost

class GophicsMonitor(
    private val activity: Activity,
) : MonitorHost {

    private val ui = Handler(Looper.getMainLooper())
    private fun ui(f: () -> Unit) = ui.post(f)

    @Volatile private var running = false
    private var worker: Thread? = null
    private var pendingPermReq = 0L

    // --- Permission ----------------------------------------------------------

    override fun authorizeMic(reqID: Long) {
        if (ContextCompat.checkSelfPermission(activity, Manifest.permission.RECORD_AUDIO)
            == PackageManager.PERMISSION_GRANTED
        ) {
            Bind.deliverPermission(reqID, true)
            return
        }
        pendingPermReq = reqID
        ActivityCompat.requestPermissions(
            activity, arrayOf(Manifest.permission.RECORD_AUDIO), REQ_RECORD_AUDIO
        )
    }

    /** Call from Activity.onRequestPermissionsResult for REQ_RECORD_AUDIO. */
    fun onPermissionResult(granted: Boolean) {
        if (pendingPermReq != 0L) {
            Bind.deliverPermission(pendingPermReq, granted)
            pendingPermReq = 0L
        }
    }

    // --- Monitoring ----------------------------------------------------------

    override fun startMonitoring(reqID: Long) {
        if (running) stopMonitoring(reqID)

        if (ContextCompat.checkSelfPermission(activity, Manifest.permission.RECORD_AUDIO)
            != PackageManager.PERMISSION_GRANTED
        ) {
            ui { Bind.failMonitoring(reqID, "microphone permission not granted") }
            return
        }

        val rec = openRecorder()
        if (rec == null) {
            ui { Bind.failMonitoring(reqID, "could not open the microphone") }
            return
        }

        running = true
        try {
            rec.startRecording()
        } catch (e: IllegalStateException) {
            running = false
            rec.release()
            ui { Bind.failMonitoring(reqID, "microphone busy: ${e.message}") }
            return
        }

        val rate = rec.sampleRate
        ui { Bind.deliverMonitorReady(reqID, rate.toLong()) }

        worker = Thread({
            // Audio priority: a scheduling stall here shows up as a dropped
            // buffer, which reads to the user as the pitch display stuttering.
            Process.setThreadPriority(Process.THREAD_PRIORITY_URGENT_AUDIO)
            val buf = ByteArray(READ_BYTES)
            while (running) {
                val n = rec.read(buf, 0, buf.size)
                if (n <= 0) {
                    // ERROR_INVALID_OPERATION / ERROR_DEAD_OBJECT: the device
                    // went away (a call came in, or the app was backgrounded).
                    if (n < 0) break
                    continue
                }
                // Deliberately NOT marshaled to the UI thread. This touches only
                // a mutex-guarded ring buffer on the Go side, runs hundreds of
                // times a second, and hopping threads would add latency to the
                // one path that must stay current. See MonitorHost's doc.
                Bind.deliverMonitorPCM(reqID, if (n == buf.size) buf else buf.copyOf(n))
            }
            try {
                rec.stop()
            } catch (_: IllegalStateException) {
                // Already stopped; releasing is what matters.
            }
            rec.release()
        }, "gophics-monitor").also { it.start() }
    }

    override fun stopMonitoring(reqID: Long) {
        running = false
        worker?.join(500)
        worker = null
    }

    /**
     * Opens an AudioRecord, preferring the least-processed input available.
     *
     * The source matters more than it looks. MIC runs the platform's voice
     * processing — automatic gain control, noise suppression, sometimes an
     * aggressive high-pass — all of which are tuned to make speech intelligible
     * over a phone line and all of which distort the harmonic structure that
     * pitch detection reads. UNPROCESSED asks for the raw capture path;
     * VOICE_RECOGNITION is the next best, since it also disables AGC on most
     * devices. MIC is the last resort.
     */
    private fun openRecorder(): AudioRecord? {
        val sources = mutableListOf<Int>()
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.N) {
            sources.add(MediaRecorder.AudioSource.UNPROCESSED)
        }
        sources.add(MediaRecorder.AudioSource.VOICE_RECOGNITION)
        sources.add(MediaRecorder.AudioSource.MIC)

        for (source in sources) {
            for (rate in RATES) {
                val min = AudioRecord.getMinBufferSize(
                    rate, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT
                )
                if (min <= 0) continue
                val rec = try {
                    AudioRecord(
                        source, rate,
                        AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT,
                        maxOf(min, READ_BYTES * 4)
                    )
                } catch (_: IllegalArgumentException) {
                    continue
                }
                if (rec.state == AudioRecord.STATE_INITIALIZED) return rec
                rec.release()
            }
        }
        return null
    }

    companion object {
        const val REQ_RECORD_AUDIO = 0x6702

        /** Preferred capture rates, best first. */
        private val RATES = intArrayOf(48000, 44100, 32000, 16000)

        /**
         * Bytes per read — 1024 frames of 16-bit mono, about 21 ms at 48 kHz.
         * Small enough that the Go ring buffer stays current, large enough that
         * the JNI crossing is not the dominant cost.
         */
        private const val READ_BYTES = 2048
    }
}
