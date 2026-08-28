// Reference Android host for the platform capabilities: share sheet, local
// notifications, keystore-backed storage, file picker and location.
//
// Copy this into your app and register it once, after Start:
//
//     val platform = GophicsPlatform(bridge, this)   // `this` = the Activity
//     bridge.setShareHost(platform)
//     bridge.setNotifyHost(platform)
//     bridge.setSecureHost(platform)
//     bridge.setFileHost(platform)
//     bridge.setLocationHost(platform)
//     bridge.setFilesDir(filesDir.absolutePath)
//
// Register only what you use: a capability whose host is not set reads as nil
// in Go, which is how an app knows to hide the affordance.
//
// The activity must forward two things, because Android delivers them to the
// Activity rather than to us:
//
//     override fun onActivityResult(rc: Int, res: Int, data: Intent?) {
//         super.onActivityResult(rc, res, data)
//         platform.onActivityResult(rc, res, data)
//     }
//     override fun onRequestPermissionsResult(rc: Int, p: Array<String>, g: IntArray) {
//         super.onRequestPermissionsResult(rc, p, g)
//         platform.onRequestPermissionsResult(rc, g)
//     }
//
// Manifest permissions: POST_NOTIFICATIONS (API 33+) for notify, and
// ACCESS_FINE_LOCATION / ACCESS_COARSE_LOCATION for location. `gophics run`
// syncs these from the capabilities your Go code actually uses.
package dev.gophics.host

import android.Manifest
import android.app.Activity
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.location.Location
import android.location.LocationListener
import android.location.LocationManager
import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import androidx.core.app.ActivityCompat
import androidx.core.app.NotificationCompat
import androidx.core.content.ContextCompat
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec
import mobile.Bridge
import mobile.FileHost
import mobile.LocationHost
import mobile.NotifyHost
import mobile.SecureHost
import mobile.ShareHost

class GophicsPlatform(
    private val bridge: Bridge,
    private val activity: Activity,
) : ShareHost, NotifyHost, SecureHost, FileHost, LocationHost {

    private companion object {
        const val CHANNEL_ID = "gophics"
        const val PREFS = "gophics_secure"
        const val KEY_ALIAS = "gophics_secure_key"
        const val GCM_TAG_BITS = 128
        const val IV_BYTES = 12

        // Request codes. Android hands results back by int, and the reqID from
        // Go is unbounded, so the code is an index into pending maps rather than
        // the reqID itself.
        const val RC_PICK = 0x6001
        const val RC_SAVE = 0x6002
        const val RC_NOTIFY_PERM = 0x6003
        const val RC_LOCATION_PERM = 0x6004
    }

    // ---- Share ----

    override fun share(
        reqID: Long, title: String?, text: String?, url: String?,
        fileName: String?, fileData: ByteArray?,
    ) {
        val body = listOfNotNull(text?.takeIf { it.isNotEmpty() }, url?.takeIf { it.isNotEmpty() })
            .joinToString("\n")
        if (body.isEmpty() && fileData == null) {
            bridge.deliverShareResult(reqID, "nothing to share")
            return
        }
        val intent = Intent(Intent.ACTION_SEND).apply {
            type = "text/plain"
            if (!title.isNullOrEmpty()) putExtra(Intent.EXTRA_SUBJECT, title)
            if (body.isNotEmpty()) putExtra(Intent.EXTRA_TEXT, body)
        }
        // A file needs a content:// URI from a FileProvider to be readable by the
        // receiving app; a file:// URI throws FileUriExposedException since N.
        // Apps that share files should declare a provider and extend this.
        try {
            activity.startActivity(Intent.createChooser(intent, title ?: ""))
            // ACTION_SEND reports nothing back, and a chooser dismissal is
            // indistinguishable from a completed share — the same ambiguity iOS
            // has, resolved the same way.
            bridge.deliverShareResult(reqID, "")
        } catch (e: Exception) {
            bridge.deliverShareResult(reqID, e.message ?: "share failed")
        }
    }

    // ---- Local notifications ----

    private var notifyReq: Long = -1

    override fun authorizeNotify(reqID: Long) {
        // Before API 33 there is no runtime permission: posting is allowed
        // unless the user turned the app's notifications off, which
        // areNotificationsEnabled reports.
        if (Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU) {
            val mgr = activity.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
            bridge.deliverNotifyPermission(reqID, mgr.areNotificationsEnabled())
            return
        }
        if (ContextCompat.checkSelfPermission(activity, Manifest.permission.POST_NOTIFICATIONS)
            == PackageManager.PERMISSION_GRANTED
        ) {
            bridge.deliverNotifyPermission(reqID, true)
            return
        }
        notifyReq = reqID
        ActivityCompat.requestPermissions(
            activity, arrayOf(Manifest.permission.POST_NOTIFICATIONS), RC_NOTIFY_PERM,
        )
    }

    override fun notify(title: String?, body: String?, tag: String?) {
        val mgr = activity.getSystemService(Context.NOTIFICATION_SERVICE) as NotificationManager
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O &&
            mgr.getNotificationChannel(CHANNEL_ID) == null
        ) {
            mgr.createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "Notifications", NotificationManager.IMPORTANCE_DEFAULT),
            )
        }
        val n = NotificationCompat.Builder(activity, CHANNEL_ID)
            .setContentTitle(title ?: "")
            .setContentText(body ?: "")
            .setSmallIcon(activity.applicationInfo.icon)
            .setAutoCancel(true)
            .build()
        // A non-empty tag coalesces: posting with the same tag replaces rather
        // than stacking. An empty tag gets a fresh id so each one stands alone.
        if (!tag.isNullOrEmpty()) {
            mgr.notify(tag, 0, n)
        } else {
            mgr.notify(System.identityHashCode(n), n)
        }
    }

    // ---- Secure storage ----
    //
    // Android has no keychain that stores arbitrary strings. The equivalent is
    // a Keystore-held AES key that never leaves the secure hardware, used to
    // encrypt values kept in a private SharedPreferences file — which is what
    // EncryptedSharedPreferences does, reimplemented here so this file has no
    // dependency beyond androidx.core.

    private val prefs by lazy {
        activity.getSharedPreferences(PREFS, Context.MODE_PRIVATE)
    }

    private fun secretKey(): SecretKey {
        val ks = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
        (ks.getEntry(KEY_ALIAS, null) as? KeyStore.SecretKeyEntry)?.let { return it.secretKey }
        val gen = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
        gen.init(
            KeyGenParameterSpec.Builder(
                KEY_ALIAS,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT,
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build(),
        )
        return gen.generateKey()
    }

    override fun secureGet(key: String?): String {
        if (key == null) return ""
        val blob = prefs.getString(key, null) ?: return ""
        return try {
            val raw = android.util.Base64.decode(blob, android.util.Base64.NO_WRAP)
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(
                Cipher.DECRYPT_MODE, secretKey(),
                GCMParameterSpec(GCM_TAG_BITS, raw, 0, IV_BYTES),
            )
            String(cipher.doFinal(raw, IV_BYTES, raw.size - IV_BYTES), Charsets.UTF_8)
        } catch (e: Exception) {
            "" // an undecryptable value is gone, not a crash
        }
    }

    override fun secureHas(key: String?): Boolean =
        key != null && prefs.contains(key)

    override fun secureSet(key: String?, value: String?): String {
        if (key == null) return "empty key"
        return try {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(Cipher.ENCRYPT_MODE, secretKey())
            val body = cipher.doFinal((value ?: "").toByteArray(Charsets.UTF_8))
            val out = cipher.iv + body
            prefs.edit().putString(key, android.util.Base64.encodeToString(out, android.util.Base64.NO_WRAP)).apply()
            ""
        } catch (e: Exception) {
            e.message ?: "keystore write failed"
        }
    }

    override fun secureDelete(key: String?): String {
        if (key == null) return ""
        prefs.edit().remove(key).apply()
        return ""
    }

    // ---- Files ----

    private var pickReq: Long = -1
    private var saveReq: Long = -1
    private var savePending: ByteArray? = null

    private fun mimeOf(accept: String?): Pair<String, Array<String>> {
        if (accept.isNullOrEmpty()) return "*/*" to emptyArray()
        val mimes = accept.split(",")
            .map { it.trim() }
            .filter { it.isNotEmpty() && !it.startsWith(".") }
        return if (mimes.isEmpty()) "*/*" to emptyArray() else mimes[0] to mimes.toTypedArray()
    }

    override fun pickFiles(reqID: Long, accept: String?, multiple: Boolean) {
        pickReq = reqID
        val (type, extra) = mimeOf(accept)
        val intent = Intent(Intent.ACTION_OPEN_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            setType(type)
            if (extra.size > 1) putExtra(Intent.EXTRA_MIME_TYPES, extra)
            putExtra(Intent.EXTRA_ALLOW_MULTIPLE, multiple)
        }
        activity.startActivityForResult(intent, RC_PICK)
    }

    override fun saveFile(reqID: Long, name: String?, accept: String?, data: ByteArray?) {
        saveReq = reqID
        savePending = data ?: ByteArray(0)
        val (type, _) = mimeOf(accept)
        val intent = Intent(Intent.ACTION_CREATE_DOCUMENT).apply {
            addCategory(Intent.CATEGORY_OPENABLE)
            setType(type)
            putExtra(Intent.EXTRA_TITLE, if (name.isNullOrEmpty()) "export" else name)
        }
        activity.startActivityForResult(intent, RC_SAVE)
    }

    /** Forward from the Activity's onActivityResult. */
    fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        when (requestCode) {
            RC_PICK -> finishPick(resultCode, data)
            RC_SAVE -> finishSave(resultCode, data)
        }
    }

    private fun finishPick(resultCode: Int, data: Intent?) {
        val reqID = pickReq
        if (reqID < 0) return
        pickReq = -1
        if (resultCode != Activity.RESULT_OK || data == null) {
            bridge.deliverPickedDone(reqID) // cancel is an empty selection
            return
        }
        val uris = mutableListOf<Uri>()
        data.clipData?.let { clip -> for (i in 0 until clip.itemCount) uris.add(clip.getItemAt(i).uri) }
        data.data?.let { uris.add(it) }
        for (uri in uris) {
            // The content URI is only readable while this grant lasts, which is
            // why the bytes are read here and not handed to Go as a path.
            val bytes = try {
                activity.contentResolver.openInputStream(uri)?.use { it.readBytes() }
            } catch (e: Exception) {
                null
            } ?: continue
            bridge.deliverPickedFile(reqID, displayName(uri), bytes)
        }
        bridge.deliverPickedDone(reqID)
    }

    private fun finishSave(resultCode: Int, data: Intent?) {
        val reqID = saveReq
        val body = savePending
        if (reqID < 0) return
        saveReq = -1
        savePending = null
        if (resultCode != Activity.RESULT_OK || data?.data == null) {
            bridge.deliverSaveDone(reqID, "") // cancel is not an error
            return
        }
        try {
            activity.contentResolver.openOutputStream(data.data!!)?.use { it.write(body ?: ByteArray(0)) }
            bridge.deliverSaveDone(reqID, "")
        } catch (e: Exception) {
            bridge.deliverSaveDone(reqID, e.message ?: "write failed")
        }
    }

    private fun displayName(uri: Uri): String {
        activity.contentResolver.query(uri, null, null, null, null)?.use { c ->
            val i = c.getColumnIndex(android.provider.OpenableColumns.DISPLAY_NAME)
            if (i >= 0 && c.moveToFirst()) return c.getString(i)
        }
        return uri.lastPathSegment ?: "file"
    }

    // ---- Location ----

    private val watching = mutableMapOf<Long, Boolean>() // reqID → isWatch
    private var listener: LocationListener? = null

    override fun startLocation(reqID: Long, watch: Boolean) {
        watching[reqID] = watch
        val fine = ContextCompat.checkSelfPermission(activity, Manifest.permission.ACCESS_FINE_LOCATION)
        val coarse = ContextCompat.checkSelfPermission(activity, Manifest.permission.ACCESS_COARSE_LOCATION)
        if (fine != PackageManager.PERMISSION_GRANTED && coarse != PackageManager.PERMISSION_GRANTED) {
            ActivityCompat.requestPermissions(
                activity,
                arrayOf(Manifest.permission.ACCESS_FINE_LOCATION, Manifest.permission.ACCESS_COARSE_LOCATION),
                RC_LOCATION_PERM,
            )
            return // the fix follows the grant
        }
        beginUpdates()
    }

    override fun stopLocation(reqID: Long) {
        watching.remove(reqID)
        if (watching.isNotEmpty()) return
        val mgr = activity.getSystemService(Context.LOCATION_SERVICE) as LocationManager
        listener?.let { mgr.removeUpdates(it) }
        listener = null
    }

    private fun beginUpdates() {
        if (listener != null) return
        val mgr = activity.getSystemService(Context.LOCATION_SERVICE) as LocationManager
        val provider = when {
            mgr.isProviderEnabled(LocationManager.GPS_PROVIDER) -> LocationManager.GPS_PROVIDER
            mgr.isProviderEnabled(LocationManager.NETWORK_PROVIDER) -> LocationManager.NETWORK_PROVIDER
            else -> {
                failAll("location services are off")
                return
            }
        }
        val l = object : LocationListener {
            override fun onLocationChanged(loc: Location) {
                for (reqID in watching.keys.toList()) {
                    bridge.deliverLocation(reqID, loc.latitude, loc.longitude, loc.accuracy.toDouble())
                }
            }

            override fun onProviderDisabled(p: String) = failAll("location services are off")
            override fun onProviderEnabled(p: String) {}
            @Deprecated("required by the interface on older API levels")
            override fun onStatusChanged(p: String?, status: Int, extras: Bundle?) {}
        }
        listener = l
        try {
            mgr.requestLocationUpdates(provider, 1000L, 1f, l)
        } catch (e: SecurityException) {
            failAll("location permission denied")
        }
    }

    private fun failAll(msg: String) {
        for (reqID in watching.keys.toList()) bridge.failLocation(reqID, msg)
        watching.clear()
    }

    /** Forward from the Activity's onRequestPermissionsResult. */
    fun onRequestPermissionsResult(requestCode: Int, grantResults: IntArray) {
        val granted = grantResults.isNotEmpty() &&
            grantResults.any { it == PackageManager.PERMISSION_GRANTED }
        when (requestCode) {
            RC_NOTIFY_PERM -> {
                if (notifyReq >= 0) {
                    bridge.deliverNotifyPermission(notifyReq, granted)
                    notifyReq = -1
                }
            }
            RC_LOCATION_PERM -> {
                if (granted) beginUpdates() else failAll("location permission denied")
            }
        }
    }
}
