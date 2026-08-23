// Reference Android PreviewHost for gophics's shell/mobile camera bridge.
//
// Wire once at startup:  bridge.setPreviewHost(GophicsPreview(activity, bridge))
// and route the runtime-permission result to onPermissionResult().
//
// This is the camera counterpart to GophicsMonitor: an open stream the Go side
// draws live, rather than a one-shot capture that ends with a result. A mirror
// or a scanner needs the former — the frame has to be current, not eventual.
//
// Camera2 rather than CameraX, to keep the host free of AndroidX camera
// dependencies an app may not otherwise want. The cost is doing the YUV→RGBA
// conversion here; see convert().

package com.example.gophics

import android.Manifest
import android.app.Activity
import android.content.Context
import android.content.pm.PackageManager
import android.graphics.ImageFormat
import android.hardware.camera2.CameraCaptureSession
import android.hardware.camera2.CameraCharacteristics
import android.hardware.camera2.CameraDevice
import android.hardware.camera2.CameraManager
import android.hardware.camera2.CaptureRequest
import android.media.ImageReader
import android.os.Handler
import android.os.HandlerThread
import android.os.Looper
import androidx.core.app.ActivityCompat
import androidx.core.content.ContextCompat
import mobile.Bridge
import mobile.PreviewHost

class GophicsPreview(
    private val activity: Activity,
    private val bridge: Bridge,
) : PreviewHost {

    private val ui = Handler(Looper.getMainLooper())
    private fun ui(f: () -> Unit) = ui.post(f)

    private var device: CameraDevice? = null
    private var session: CameraCaptureSession? = null
    private var reader: ImageReader? = null

    // The camera's own thread. Frames must not be delivered on the UI thread:
    // DeliverPreviewFrame touches no app code, and marshalling it through the
    // main looper would add a frame of latency and jank the UI at 30fps.
    private var thread: HandlerThread? = null
    private var handler: Handler? = null

    // rgba is reused across frames; Go copies out of it immediately.
    private var rgba: ByteArray = ByteArray(0)

    private var pendingAuth = 0
    private var activeReq = 0

    // --- permission ---------------------------------------------------------

    override fun authorizeCamera(reqID: Int) {
        val granted = ContextCompat.checkSelfPermission(activity, Manifest.permission.CAMERA) ==
            PackageManager.PERMISSION_GRANTED
        if (granted) {
            ui { bridge.deliverPermission(reqID, true) }
            return
        }
        pendingAuth = reqID
        ActivityCompat.requestPermissions(activity, arrayOf(Manifest.permission.CAMERA), REQ_CAMERA)
    }

    /** Route Activity.onRequestPermissionsResult here. */
    fun onPermissionResult(requestCode: Int, grantResults: IntArray) {
        if (requestCode != REQ_CAMERA || pendingAuth == 0) return
        val ok = grantResults.isNotEmpty() && grantResults[0] == PackageManager.PERMISSION_GRANTED
        val id = pendingAuth
        pendingAuth = 0
        ui { bridge.deliverPermission(id, ok) }
    }

    // --- streaming ----------------------------------------------------------

    override fun startPreview(reqID: Int, facing: Int, width: Int) {
        if (ContextCompat.checkSelfPermission(activity, Manifest.permission.CAMERA) !=
            PackageManager.PERMISSION_GRANTED
        ) {
            ui { bridge.failPreview(reqID, "camera permission not granted") }
            return
        }
        val mgr = activity.getSystemService(Context.CAMERA_SERVICE) as CameraManager
        val id = pickCamera(mgr, facing)
        if (id == null) {
            ui { bridge.failPreview(reqID, "no camera on this device") }
            return
        }

        val w = if (width > 0) width else 640
        val h = w * 3 / 4

        val t = HandlerThread("gophics-camera").also { it.start() }
        thread = t
        handler = Handler(t.looper)
        activeReq = reqID

        // maxImages=2: one being converted while the next is filled. More would
        // queue stale frames, and a preview wants the newest, not every one.
        val r = ImageReader.newInstance(w, h, ImageFormat.YUV_420_888, 2)
        reader = r
        r.setOnImageAvailableListener({ ir ->
            val img = ir.acquireLatestImage() ?: return@setOnImageAvailableListener
            try {
                val n = img.width * img.height * 4
                if (rgba.size != n) rgba = ByteArray(n)
                convert(img, rgba)
                // Camera thread on purpose — see the class comment.
                bridge.deliverPreviewFrame(activeReq, rgba, img.width, img.height)
            } catch (e: Throwable) {
                // A dropped frame is survivable; taking the process down is not.
            } finally {
                img.close()
            }
        }, handler)

        try {
            mgr.openCamera(id, object : CameraDevice.StateCallback() {
                override fun onOpened(cam: CameraDevice) {
                    device = cam
                    try {
                        @Suppress("DEPRECATION")
                        cam.createCaptureSession(
                            listOf(r.surface),
                            object : CameraCaptureSession.StateCallback() {
                                override fun onConfigured(s: CameraCaptureSession) {
                                    session = s
                                    val b = cam.createCaptureRequest(CameraDevice.TEMPLATE_PREVIEW)
                                    b.addTarget(r.surface)
                                    b.set(
                                        CaptureRequest.CONTROL_AF_MODE,
                                        CaptureRequest.CONTROL_AF_MODE_CONTINUOUS_PICTURE,
                                    )
                                    s.setRepeatingRequest(b.build(), null, handler)
                                    ui { bridge.deliverPreviewReady(reqID) }
                                }

                                override fun onConfigureFailed(s: CameraCaptureSession) {
                                    ui { bridge.failPreview(reqID, "could not configure the camera") }
                                    release()
                                }
                            },
                            handler,
                        )
                    } catch (e: Throwable) {
                        ui { bridge.failPreview(reqID, e.message ?: "camera session failed") }
                        release()
                    }
                }

                override fun onDisconnected(cam: CameraDevice) = release()

                override fun onError(cam: CameraDevice, error: Int) {
                    ui { bridge.failPreview(reqID, "camera error $error") }
                    release()
                }
            }, handler)
        } catch (e: SecurityException) {
            ui { bridge.failPreview(reqID, "camera permission not granted") }
            release()
        } catch (e: Throwable) {
            ui { bridge.failPreview(reqID, e.message ?: "could not open the camera") }
            release()
        }
    }

    override fun stopPreview(reqID: Int) = release()

    private fun release() {
        try { session?.close() } catch (_: Throwable) {}
        try { device?.close() } catch (_: Throwable) {}
        try { reader?.close() } catch (_: Throwable) {}
        session = null
        device = null
        reader = null
        thread?.quitSafely()
        thread = null
        handler = null
        activeReq = 0
    }

    private fun pickCamera(mgr: CameraManager, facing: Int): String? {
        val want = if (facing == 1) {
            CameraCharacteristics.LENS_FACING_BACK
        } else {
            CameraCharacteristics.LENS_FACING_FRONT
        }
        var fallback: String? = null
        for (id in mgr.cameraIdList) {
            val f = mgr.getCameraCharacteristics(id).get(CameraCharacteristics.LENS_FACING)
            if (f == want) return id
            if (fallback == null) fallback = id
        }
        // Any camera beats none: a tablet with only a back camera should still
        // show something when the app asked for the front one.
        return fallback
    }

    // convert turns one YUV_420_888 image into RGBA8888.
    //
    // Camera2 delivers YUV because that is what the hardware produces; the GPU
    // path upstream wants RGBA. Doing it here rather than in Go keeps one copy
    // out of the bind boundary — the alternative is shipping three planes
    // across and converting there, which costs an extra crossing per frame.
    //
    // Integer BT.601, which is what every Android camera pipeline assumes.
    private fun convert(img: android.media.Image, out: ByteArray) {
        val w = img.width
        val h = img.height
        val yP = img.planes[0]
        val uP = img.planes[1]
        val vP = img.planes[2]
        val yB = yP.buffer
        val uB = uP.buffer
        val vB = vP.buffer
        val yRow = yP.rowStride
        val uvRow = uP.rowStride
        val uvPix = uP.pixelStride

        var o = 0
        for (y in 0 until h) {
            val yLine = y * yRow
            val uvLine = (y shr 1) * uvRow
            for (x in 0 until w) {
                val yv = (yB.get(yLine + x).toInt() and 0xff) - 16
                val uvIdx = uvLine + (x shr 1) * uvPix
                val u = (uB.get(uvIdx).toInt() and 0xff) - 128
                val v = (vB.get(uvIdx).toInt() and 0xff) - 128

                val yy = 1192 * yv
                out[o] = clamp((yy + 1634 * v) shr 10)
                out[o + 1] = clamp((yy - 833 * v - 400 * u) shr 10)
                out[o + 2] = clamp((yy + 2066 * u) shr 10)
                out[o + 3] = 0xFF.toByte()
                o += 4
            }
        }
    }

    private fun clamp(v: Int): Byte =
        when {
            v < 0 -> 0
            v > 255 -> 255.toByte()
            else -> v.toByte()
        }

    companion object {
        const val REQ_CAMERA = 4711
    }
}
