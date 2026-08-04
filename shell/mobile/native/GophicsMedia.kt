// Reference Android MediaHost for gophics's shell/mobile media bridge.
// Scaffolding — copy into the host app and verify on device. Type names from
// `gomobile bind` (here `mobile.*`) may differ; adjust to your bind package.
//
// Wire once at startup:  bridge.setMediaHost(GophicsMedia(activity, bridge))
// Camera uses ACTION_IMAGE_CAPTURE; route the Activity result to onPhotoResult().

package com.example.gophics

import android.app.Activity
import android.content.Intent
import android.media.AudioFormat
import android.media.AudioRecord
import android.media.MediaExtractor
import android.media.MediaPlayer
import android.media.MediaRecorder
import android.os.Handler
import android.os.Looper
import android.provider.MediaStore
import java.io.ByteArrayOutputStream
import java.io.File
import kotlin.concurrent.thread
import mobile.Bridge
import mobile.MediaHost

class GophicsMedia(
    private val activity: Activity,
    private val bridge: Bridge,
) : MediaHost {
    private val ui = Handler(Looper.getMainLooper())
    private fun ui(f: () -> Unit) = ui.post(f)

    // Camera
    private var captureReq = 0
    // Recording
    @Volatile private var recording = false
    private var recReq = 0
    private var recRate = 44100
    // Playback
    private var player: MediaPlayer? = null
    private var playReq = 0
    private val posTicker = object : Runnable {
        override fun run() {
            player?.let { bridge.setPlaybackPosition(playReq.toLong(), it.currentPosition.toLong()); ui.postDelayed(this, 50) }
        }
    }

    // --- Camera (ACTION_IMAGE_CAPTURE — native camera app) -------------------

    override fun authorizeCamera(reqID: Long) {
        // The capture intent needs no CAMERA permission.
        bridge.deliverPermission(reqID, true)
    }

    override fun capturePhoto(reqID: Long, facing: Long) {
        captureReq = reqID.toInt()
        val intent = Intent(MediaStore.ACTION_IMAGE_CAPTURE)
        // Provide a content-Uri via FileProvider in a real app; simplest path
        // returns a thumbnail in the result "data" extra (see onPhotoResult).
        activity.startActivityForResult(intent, REQ_CAPTURE)
    }

    /** Call from Activity.onActivityResult for REQ_CAPTURE. */
    fun onPhotoResult(ok: Boolean, jpeg: ByteArray?) {
        if (ok && jpeg != null) bridge.deliverPhoto(captureReq.toLong(), jpeg)
        else bridge.failCapture(captureReq.toLong(), "cancelled")
    }

    // --- Audio ---------------------------------------------------------------

    override fun authorizeMic(reqID: Long) {
        // Request RECORD_AUDIO at runtime elsewhere; assume granted here.
        bridge.deliverPermission(reqID, true)
    }

    override fun startRecording(reqID: Long) {
        recReq = reqID.toInt()
        val minBuf = AudioRecord.getMinBufferSize(
            recRate, AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT
        )
        val rec = AudioRecord(
            MediaRecorder.AudioSource.MIC, recRate,
            AudioFormat.CHANNEL_IN_MONO, AudioFormat.ENCODING_PCM_16BIT, maxOf(minBuf, 4096)
        )
        val pcm = ByteArrayOutputStream()
        recording = true
        rec.startRecording()
        bridge.deliverRecorderReady(reqID)
        thread(name = "gophics-rec") {
            val buf = ByteArray(4096)
            while (recording) {
                val n = rec.read(buf, 0, buf.size)
                if (n <= 0) continue
                pcm.write(buf, 0, n)
                var peak = 0
                var i = 0
                while (i + 1 < n) {
                    val s = (buf[i].toInt() and 0xff) or (buf[i + 1].toInt() shl 8)
                    val a = if (s > 32767) 65536 - s else if (s < 0) -s else s
                    if (a > peak) peak = a
                    i += 2
                }
                val level = peak / 32768f
                ui { bridge.setAudioLevel(reqID, level) }
            }
            rec.stop(); rec.release()
            val bytes = pcm.toByteArray()
            val ms = (bytes.size / 2) * 1000 / recRate
            ui { bridge.deliverPCM(reqID, bytes, recRate.toLong(), ms.toLong()) }
        }
    }

    override fun stopRecording(reqID: Long) { recording = false }

    override fun playClip(reqID: Long, wav: ByteArray) {
        playReq = reqID.toInt()
        val f = File.createTempFile("clip", ".wav", activity.cacheDir)
        f.writeBytes(wav)
        val p = MediaPlayer()
        p.setDataSource(f.absolutePath)
        p.setOnCompletionListener { bridge.playbackEnded(reqID); ui.removeCallbacks(posTicker) }
        p.prepare()
        p.start()
        player = p
        bridge.deliverPlaybackReady(reqID)
        ui.postDelayed(posTicker, 50)
    }

    override fun seekPlayback(reqID: Long, ms: Long) { player?.seekTo(ms.toInt()) }

    override fun stopPlayback(reqID: Long) {
        player?.stop(); player?.release(); player = null
        ui.removeCallbacks(posTicker)
        bridge.playbackEnded(reqID)
    }

    companion object { const val REQ_CAPTURE = 0x6701 }
}
