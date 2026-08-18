package dev.gophics.news

import android.app.Activity
import android.app.AlertDialog
import android.util.Log
import android.view.ViewGroup
import android.webkit.CookieManager
import android.webkit.WebView
import android.webkit.WebViewClient
import android.widget.LinearLayout
import android.widget.TextView
import newsmobile.Newsmobile

/**
 * The sign-in view for paid sources.
 *
 * A publisher like The Economist puts a teaser in its feed and the article
 * behind a login, so reading it in the app means sending the subscriber's own
 * session cookie with the article fetch. Getting that cookie needs a real
 * browser and a real login form, which the Go side does not have: gophics draws
 * every pixel itself, and its WebView capability is implemented for the web
 * shell only and exposes no cookie access at all.
 *
 * So the login happens here, in a plain Android WebView, and the session is
 * read out of CookieManager and handed back over the bind surface. Nothing
 * about the credentials passes through Go — only the resulting cookie header,
 * which is stored 0600 in app-private storage and sent only to the domain it
 * came from.
 */
object LoginSheet {

    fun show(activity: Activity, domain: String, url: String) {
        val web = WebView(activity).apply {
            settings.javaScriptEnabled = true    // every login form needs it
            settings.domStorageEnabled = true
            layoutParams = LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT, 0, 1f)
            webViewClient = WebViewClient()
        }
        // Third-party cookies are commonly where the session actually lands,
        // because publishers put their identity provider on another host.
        CookieManager.getInstance().setAcceptThirdPartyCookies(web, true)

        val hint = TextView(activity).apply {
            text = "Sign in to $domain as usual, then tap Done. " +
                "The session stays on this device."
            setPadding(32, 32, 32, 16)
        }
        val root = LinearLayout(activity).apply {
            orientation = LinearLayout.VERTICAL
            addView(hint)
            addView(web)
        }

        val dialog = AlertDialog.Builder(activity)
            .setTitle(domain)
            .setView(root)
            .setPositiveButton("Done") { d, _ ->
                capture(domain, url)
                d.dismiss()
            }
            .setNegativeButton("Cancel") { d, _ -> d.dismiss() }
            .create()

        // Flush before reading: the cookie store is written lazily, and a
        // session captured a moment too early is a session that silently does
        // not work.
        dialog.setOnDismissListener { CookieManager.getInstance().flush() }
        dialog.show()
        web.loadUrl(url)
    }

    /**
     * capture reads every cookie the browser holds for the site and hands the
     * lot to the reader.
     *
     * Deliberately not picking out "the session cookie": which one carries the
     * session is undocumented and changes between deployments. Cookies for
     * other domains are rejected on the Go side by domain mismatch, so sending
     * all of one site's cookies is safe, merely more than strictly needed.
     */
    private fun capture(domain: String, url: String) {
        val cm = CookieManager.getInstance()
        cm.flush()
        val header = listOfNotNull(cm.getCookie(url), cm.getCookie("https://$domain/"))
            .filter { it.isNotBlank() }
            .joinToString("; ")

        if (header.isBlank()) {
            Log.w("gophics", "no cookies captured for $domain — was the login completed?")
            return
        }
        val err = Newsmobile.setCookies(domain, header)
        if (err.isNotEmpty()) {
            Log.w("gophics", "storing cookies for $domain failed: $err")
        } else {
            Log.i("gophics", "stored session for $domain")
        }
    }
}
