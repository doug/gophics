// The sign-in view for paid sources.
//
// A publisher like The Economist puts a teaser in its feed and the article
// behind a login, so reading it in the app means sending the subscriber's own
// session cookie with the article fetch. Getting that cookie needs a real
// browser and a real login form, which the Go side does not have: gophics draws
// every pixel itself, and its WebView capability is implemented for the web
// shell only and exposes no cookie access at all.
//
// So the login happens here, in a WKWebView, and the session is read out of
// WKHTTPCookieStore and handed back over the bind surface. Nothing about the
// credentials passes through Go — only the resulting cookie header, which is
// stored 0600 in the app sandbox and sent only to the domain it came from.
import UIKit
import WebKit
import Newsmobile

enum LoginSheet {
    private static var presenting = false

    static func present(from view: UIView, domain: String, url: String) {
        guard !presenting, let host = view.window?.rootViewController else { return }
        presenting = true
        let vc = LoginViewController(domain: domain, url: url) { presenting = false }
        host.present(UINavigationController(rootViewController: vc), animated: true)
    }
}

final class LoginViewController: UIViewController {
    private let domain: String
    private let startURL: String
    private let onClose: () -> Void
    private var web: WKWebView!

    init(domain: String, url: String, onClose: @escaping () -> Void) {
        self.domain = domain
        self.startURL = url
        self.onClose = onClose
        super.init(nibName: nil, bundle: nil)
    }

    required init?(coder: NSCoder) { fatalError("not used") }

    override func viewDidLoad() {
        super.viewDidLoad()
        title = domain
        navigationItem.rightBarButtonItem = UIBarButtonItem(
            title: "Done", style: .done, target: self, action: #selector(done))
        navigationItem.leftBarButtonItem = UIBarButtonItem(
            title: "Cancel", style: .plain, target: self, action: #selector(cancel))

        // The default persistent data store, so a session survives the sheet
        // being closed and the app being restarted.
        let config = WKWebViewConfiguration()
        config.websiteDataStore = .default()
        web = WKWebView(frame: view.bounds, configuration: config)
        web.autoresizingMask = [.flexibleWidth, .flexibleHeight]
        view.addSubview(web)

        if let u = URL(string: startURL) {
            web.load(URLRequest(url: u))
        }
    }

    @objc private func done() {
        capture { [weak self] in
            self?.dismiss(animated: true) { self?.onClose() }
        }
    }

    @objc private func cancel() {
        dismiss(animated: true) { self.onClose() }
    }

    /// capture reads every cookie the web view holds for the site and hands the
    /// lot to the reader.
    ///
    /// Deliberately not picking out "the session cookie": which one carries the
    /// session is undocumented and changes between deployments. Cookies for
    /// other domains are dropped here by suffix match, and rejected again on
    /// the Go side by domain mismatch.
    private func capture(completion: @escaping () -> Void) {
        let store = web.configuration.websiteDataStore.httpCookieStore
        store.getAllCookies { cookies in
            let mine = cookies.filter { c in
                let d = c.domain.hasPrefix(".") ? String(c.domain.dropFirst()) : c.domain
                return d == self.domain || d.hasSuffix("." + self.domain) || self.domain.hasSuffix("." + d)
            }
            let header = mine.map { "\($0.name)=\($0.value)" }.joined(separator: "; ")

            if header.isEmpty {
                NSLog("gophics: no cookies captured for %@ — was the login completed?", self.domain)
            } else {
                let err = NewsmobileSetCookies(self.domain, header)
                if !err.isEmpty {
                    NSLog("gophics: storing cookies for %@ failed: %@", self.domain, err)
                } else {
                    NSLog("gophics: stored session for %@", self.domain)
                }
            }
            DispatchQueue.main.async(execute: completion)
        }
    }
}
