package ui

import (
	"strings"

	"github.com/doug/gophics/examples/news/internal/library"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// HostLogin is set by a platform host that can present a sign-in web view.
//
// The framework's WebView capability is implemented for the web shell only, and
// it exposes no way to read cookies back — so this deliberately does not use
// it. On Android and iOS the host activity already owns a native view hierarchy
// and can present a WebView or WKWebView over the gophics surface; when the user
// finishes logging in, the host reads the session out of CookieManager or
// WKHTTPCookieStore and hands it back through the gomobile bind surface.
//
// Desktop and web builds leave this nil and get the paste-a-header path
// instead, which is also how the flow is exercised in development.
var HostLogin func(domain, url string)

// signInPage is how a paid subscription reaches the reader.
type signInPage struct {
	FeedID string
	URL    string
	Title  string
}

func (signInPage) CreateState() widget.State { return &signInState{} }

type signInState struct {
	widget.StateBase[signInPage]
	pasted    string
	saveErr   string
	verifying bool
	verified  *library.VerifyResult
}

func (s *signInState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Of(ctx)
	lib := env(ctx).Lib
	domain := library.FeedDomain(s.W().URL)
	st := library.Cookies(domain)

	kids := []widget.Widget{s.statusCard(th, domain, st)}

	if HostLogin != nil {
		kids = append(kids, s.webViewCard(th, domain))
	} else {
		kids = append(kids, s.pasteCard(ctx, th, domain))
	}
	if st.Present {
		kids = append(kids, s.verifyCard(ctx, th, lib))
	}

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStretch

	title := s.W().Title
	if title == "" {
		title = domain
	}
	return page(ctx, header(th, "Subscription", title, backButton(ctx)),
		widget.Scroll{Child: col})
}

func (s *signInState) statusCard(th theme.Theme, domain string, st library.CookieStatus) widget.Widget {
	title, detail, color := "Not signed in", "", th.Muted
	switch {
	case st.Healthy():
		title = "Signed in"
		detail = plural(st.Count, "cookie", "cookies") + " saved for " + domain
		if st.Expired > 0 {
			detail += " (" + itoa(st.Expired) + " expired, which is normal)"
		}
		color = th.Success
	case st.Present:
		title = "Session expired"
		detail = "Every saved cookie for " + domain + " has expired. Sign in again."
		color = th.Warning
	default:
		detail = "This publisher only puts a teaser in its feed. Signing in lets the reader fetch the full article the same way your browser does."
	}

	col := widget.Column(
		widget.Text{S: title, Font: "bold", Size: th.Type.Heading, Color: color},
		widget.Sized{H: 6},
		widget.Text{S: detail, Size: th.Type.Body, Color: th.Muted, Wrap: true},
	)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

// webViewCard is the phone path: the host shows the publisher's own login page.
func (s *signInState) webViewCard(th theme.Theme, domain string) widget.Widget {
	col := widget.Column(
		widget.Text{S: "Sign in with your subscription", Font: "bold",
			Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 6},
		widget.Text{S: "Opens " + domain + " so you can log in as usual. The reader keeps the session on this device and sends it only to " + domain + ".",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 12},
		button(th, "Open "+domain, func() {
			HostLogin(domain, "https://"+domain+"/")
		}),
	)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

// pasteCard is the desktop path, and the fallback anywhere without a host web
// view. One line copied out of a browser's network panel is the whole session.
func (s *signInState) pasteCard(ctx widget.Ctx, th theme.Theme, domain string) widget.Widget {
	field := widget.Decorated{Color: th.Bg, Radius: th.Radius,
		Child: widget.Padding{All: 10, Child: widget.TextField{
			Value:            s.pasted,
			Placeholder:      "name=value; other=value",
			Multiline:        true,
			Size:             th.Type.Label,
			TextColor:        th.Text,
			PlaceholderColor: th.Muted,
			CaretColor:       th.Primary,
			SelectionColor:   th.Selection,
			OnChange:         func(v string) { s.SetState(func() { s.pasted = v }) },
		}},
	}

	steps := []string{
		"Open an article on " + domain + " in your browser, signed in.",
		"Open the developer tools, Network tab, and reload.",
		"Click the first document request.",
		"Under Request Headers, right-click the Cookie line and copy its value.",
		"Paste it below.",
	}
	kids := []widget.Widget{
		widget.Text{S: "Paste your browser's session", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 8},
	}
	for i, st := range steps {
		kids = append(kids,
			widget.Text{S: itoa(i+1) + ". " + st, Size: th.Type.Caption, Color: th.Muted, Wrap: true},
			widget.Sized{H: 4})
	}
	kids = append(kids,
		widget.Sized{H: 8},
		widget.Text{S: "Copy every cookie for the site, not one you picked out — which cookie carries the session is undocumented and changes.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 10},
		field,
	)
	if s.saveErr != "" {
		kids = append(kids, widget.Sized{H: 8},
			widget.Text{S: s.saveErr, Size: th.Type.Caption, Color: th.Danger, Wrap: true})
	}
	kids = append(kids, widget.Sized{H: 12}, button(th, "Save session", func() {
		err := library.SaveCookies(domain, s.pasted)
		s.SetState(func() {
			if err != nil {
				s.saveErr = err.Error()
				return
			}
			s.saveErr, s.pasted, s.verified = "", "", nil
		})
	}))

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

// verifyCard runs the only check that answers the real question.
func (s *signInState) verifyCard(ctx widget.Ctx, th theme.Theme, lib *library.Library) widget.Widget {
	kids := []widget.Widget{
		widget.Text{S: "Check it works", Font: "bold", Size: th.Type.Body, Color: th.Text},
		widget.Sized{H: 6},
		widget.Text{S: "Fetches one article twice — with and without the session — and compares how much of it came back.",
			Size: th.Type.Caption, Color: th.Muted, Wrap: true},
		widget.Sized{H: 12},
	}
	switch {
	case s.verifying:
		kids = append(kids, widget.Text{S: "Checking…", Size: th.Type.Body, Color: th.Muted})
	case s.verified != nil:
		color := th.Danger
		if s.verified.Working() {
			color = th.Success
		}
		kids = append(kids,
			widget.Text{S: s.verified.Summary(), Size: th.Type.Body, Color: color, Wrap: true},
			widget.Sized{H: 12})
		kids = append(kids, secondaryButton(th, "Check again", func() { s.verify(ctx, lib) }))
	default:
		kids = append(kids, secondaryButton(th, "Check now", func() { s.verify(ctx, lib) }))
	}

	if library.Cookies(library.FeedDomain(s.W().URL)).Present {
		kids = append(kids, widget.Sized{H: 10},
			secondaryButton(th, "Forget this session", func() {
				library.ClearCookies(library.FeedDomain(s.W().URL))
				s.SetState(func() { s.verified = nil })
			}))
	}

	col := widget.Column(kids...)
	col.CrossAlign = layout.CrossStart
	return card(th, col)
}

func (s *signInState) verify(ctx widget.Ctx, lib *library.Library) {
	f, ok := lib.Subs.ByID(s.W().FeedID)
	if !ok {
		f.URL = s.W().URL
	}
	s.SetState(func() { s.verifying = true })
	post := ctx.Post()
	lctx := ctx.Context()
	go func() {
		res := lib.VerifyCookies(lctx, f)
		post(func() {
			s.SetState(func() {
				s.verifying = false
				s.verified = &res
			})
		})
	}()
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return itoa(n) + " " + many
}

// SetHostCookies is called by a platform host after a sign-in web view closes.
// It is exported from here rather than from the mobile package so the desktop
// build can exercise the same path in a test.
func SetHostCookies(domain, cookieHeader string) error {
	if strings.TrimSpace(cookieHeader) == "" {
		return nil
	}
	return library.SaveCookies(domain, cookieHeader)
}
