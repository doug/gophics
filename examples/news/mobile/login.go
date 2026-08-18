package newsmobile

import (
	"sync"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/examples/news/ui"
)

// The sign-in handshake for paid sources.
//
// A publisher like The Economist puts a teaser in its feed and the article
// behind a login. Reading it here means sending the subscriber's own session
// cookie with the article fetch — which first means getting that cookie, which
// means a real browser view and a real login form.
//
// gophics cannot supply that view. shell.WebView is implemented for the web
// shell only, and even there it exposes no cookie access. What the host does
// have is its own native view hierarchy, so the login happens there: Android in
// a WebView reading android.webkit.CookieManager, iOS in a WKWebView reading
// WKHTTPCookieStore.
//
// The bind surface only lets the host call into Go, never the reverse, so the
// request travels by polling. The host already polls NeedsFrame every vsync;
// checking one more string costs nothing.
//
//	Kotlin:
//	    val domain = Newsmobile.pendingLoginDomain()
//	    if (domain.isNotEmpty()) {
//	        Newsmobile.clearPendingLogin()
//	        showLoginWebView(domain) { cookieHeader ->
//	            Newsmobile.setCookies(domain, cookieHeader)
//	        }
//	    }

var (
	loginMu     sync.Mutex
	loginDomain string
	loginURL    string
)

func init() {
	// Telling the UI a host web view exists is what makes the sign-in screen
	// offer the button instead of the paste-a-header instructions.
	ui.HostLogin = func(domain, url string) {
		loginMu.Lock()
		loginDomain, loginURL = domain, url
		loginMu.Unlock()
	}
}

// PendingLoginDomain returns the publisher the reader wants signed in to, or ""
// when there is nothing to do. Poll it alongside NeedsFrame.
func PendingLoginDomain() string {
	loginMu.Lock()
	defer loginMu.Unlock()
	return loginDomain
}

// PendingLoginURL is the page the host should load — the publisher's own site,
// where landing logged in is itself the confirmation that it worked.
func PendingLoginURL() string {
	loginMu.Lock()
	defer loginMu.Unlock()
	return loginURL
}

// ClearPendingLogin acknowledges the request. Call it as soon as the web view
// is presented, so a user who backs out without logging in is not shown the
// login again on the next frame.
func ClearPendingLogin() {
	loginMu.Lock()
	loginDomain, loginURL = "", ""
	loginMu.Unlock()
}

// SetCookies hands a captured session to the reader. The value is a request
// header — "name=value; other=value" — which is what both CookieManager.getCookie
// and a WKHTTPCookieStore enumeration naturally produce.
//
// The cookies are written 0600 under the data directory and are only ever sent
// to the domain they came from. An error string is returned when nothing usable
// was found, so the host can say so rather than appearing to succeed.
func SetCookies(domain, cookieHeader string) string {
	if err := ui.SetHostCookies(domain, cookieHeader); err != nil {
		return err.Error()
	}
	return ""
}

// HasCookies reports whether a usable session is stored for a domain, so a host
// can show the state in its own UI if it wants to.
func HasCookies(domain string) bool {
	return library.Cookies(domain).Healthy()
}
