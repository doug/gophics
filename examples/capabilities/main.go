// Command capabilities is a live inspector for the platform-capability layer
// (shell/*.go via ctx.<Cap>()): every capability the running platform exposes,
// shown working. Live readouts (connectivity, battery, app lifecycle, launch
// URL, gamepads) update themselves; the action cards drive the interactive ones
// (file picker, share sheet, notifications, geolocation, storage, clipboard,
// window control, soft keyboard, screen-reader announce, web view). A capability
// the platform can't provide renders greyed-out "unsupported" — so the same
// screen honestly reports what each target (web/desktop/mobile) actually has.
//
//	go run ./examples/capabilities        # desktop
//	# or open the web build in a browser (most capabilities are web-backed)
package main

import (
	"fmt"
	"log"
	"strings"
	"time"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type App struct{}

func (App) CreateState() widget.State { return &state{} }

type state struct {
	widget.StateBase[App]

	subscribed bool
	gp         shell.Gamepads
	gpSummary  string

	lastAction string
	typed      string
	storedNote string
	geo        string
	wv         shell.WebViewHandle
}

// subscribe wires up live-update sources the first time capabilities are
// available. Capabilities are nil at mount (the tree builds before the shell
// exists) and arrive on the first frame; the runtime rebuilds when they do, so
// this runs from Build, not Init, guarded to run exactly once.
func (s *state) subscribe(ctx widget.Ctx) {
	if s.subscribed {
		return
	}
	// Wait until the window has been wired (any capability present).
	if ctx.Connectivity() == nil && ctx.Gamepads() == nil && ctx.FilePicker() == nil {
		return
	}
	s.subscribed = true
	rebuild := func() { s.SetState(func() {}) }
	if c := ctx.Connectivity(); c != nil {
		c.OnChange(func(bool) { rebuild() })
	}
	if b := ctx.Battery(); b != nil {
		b.OnChange(rebuild)
	}
	if l := ctx.Lifecycle(); l != nil {
		l.OnChange(func(shell.AppState) { rebuild() })
	}
	if g := ctx.Gamepads(); g != nil {
		s.gp = g
		ctx.AddTicker(poller{s})
	}
	if st := ctx.SecureStorage(); st != nil {
		if v, ok := st.Get("note"); ok {
			s.storedNote = v
		}
	}
}

func (s *state) Dispose() {
	if s.wv != nil {
		s.wv.Close()
	}
}

// poller re-reads gamepad state each frame (input is inherently live), rebuilding
// only when the snapshot changes so a still controller doesn't churn.
type poller struct{ s *state }

func (p poller) Tick(float64) bool {
	sum := gamepadSummary(p.s.gp.Poll())
	if sum != p.s.gpSummary {
		p.s.SetState(func() { p.s.gpSummary = sum })
	}
	return true
}

func gamepadSummary(gs []shell.Gamepad) string {
	n := 0
	for _, g := range gs {
		if g.Connected {
			n++
		}
	}
	if n == 0 {
		return "no controller connected"
	}
	g := gs[0]
	return fmt.Sprintf("%d connected — %q: %d buttons, %d axes", n, g.ID, len(g.Buttons), len(g.Axes))
}

func (s *state) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)
	s.subscribe(ctx)

	rows := []widget.Widget{
		widget.Padding{Insets: geom.Insets{Top: 40, Bottom: 4}, Child: theme.Display("Capabilities")},
		theme.Label("What this platform exposes through ctx.<Cap>() — live."),
		widget.Sized{H: 16},
		sectionLabel(th, "Live"),

		liveCard(th, "Connectivity", ctx.Connectivity() != nil, connStatus(ctx)),
		liveCard(th, "Battery", ctx.Battery() != nil, battStatus(ctx)),
		liveCard(th, "Lifecycle", ctx.Lifecycle() != nil, lifeStatus(ctx)),
		liveCard(th, "Launch link", ctx.Links() != nil, linkStatus(ctx)),
		liveCard(th, "Gamepad", ctx.Gamepads() != nil, s.gpSummary),

		widget.Sized{H: 12},
		sectionLabel(th, "Actions"),
	}

	rows = append(rows,
		action(th, "File picker", ctx.FilePicker() != nil, "Open a file", func() {
			ctx.FilePicker().Open(shell.OpenOptions{}, func(f []shell.PickedFile, err error) {
				s.setAction(pickResult(f, err))
			})
		}, s.lastActionFor("File picker")),

		action(th, "Share sheet", ctx.Share() != nil, "Share a link", func() {
			ctx.Share().Share(shell.ShareItem{Title: "gophics", Text: "Drawn in pure Go.", URL: "https://github.com/doug/gophics"},
				func(error) { s.setAction("Share: share sheet opened") })
		}, s.lastActionFor("Share")),

		action(th, "Notifications", ctx.Notifier() != nil, "Notify me", func() {
			n := ctx.Notifier()
			n.Authorize(func(p shell.Permission) {
				if p == shell.PermissionGranted {
					n.Notify(shell.Notification{Title: "gophics", Body: "A local notification."})
					s.setAction("Notifications: posted")
				} else {
					s.setAction("Notifications: permission denied")
				}
			})
		}, s.lastActionFor("Notifications")),

		action(th, "Geolocation", ctx.Geolocation() != nil, "Locate me", func() {
			ctx.Geolocation().Current(func(lat, lon, acc float64, err error) {
				if err != nil {
					s.SetState(func() { s.geo = "denied / unavailable" })
					return
				}
				s.SetState(func() { s.geo = fmt.Sprintf("%.4f, %.4f (±%.0fm)", lat, lon, acc) })
			})
		}, s.geo),

		action(th, "Secure storage", ctx.SecureStorage() != nil, "Save a note", func() {
			note := "saved " + time.Now().Format("15:04:05")
			_ = ctx.SecureStorage().Set("note", note)
			s.SetState(func() { s.storedNote = note })
		}, s.storedNote),

		action(th, "Clipboard", ctx.Clipboard() != nil, "Copy a phrase", func() {
			_ = ctx.Clipboard().ClipboardWrite("gophics: one binary, everywhere")
			s.setAction("Clipboard: wrote a phrase — paste to check")
		}, s.lastActionFor("Clipboard")),

		action(th, "Window control", ctx.WindowControl() != nil, "Toggle fullscreen", func() {
			wc := ctx.WindowControl()
			wc.SetTitle("gophics ✦ capabilities")
			wc.SetFullscreen(!wc.Fullscreen())
			s.setAction("Window: title set, fullscreen toggled")
		}, s.lastActionFor("Window")),

		action(th, "Text input (IME)", ctx.TextInput() != nil, "Raise keyboard", func() {
			ctx.TextInput().Show(shell.TextInputOptions{Autocorrect: true}, shell.TextInputHandler{
				OnText: func(t string) { s.SetState(func() { s.typed += t }) },
				OnEditKey: func(k shell.EditKey) {
					if k == shell.EditBackspace && s.typed != "" {
						s.SetState(func() { s.typed = s.typed[:len(s.typed)-1] })
					}
				},
			})
		}, "typed: "+s.typed),

		action(th, "Accessibility", ctx.Accessibility() != nil, "Announce", func() {
			ctx.Accessibility().Announce("Hello from gophics — this went to your screen reader.", false)
			s.setAction("Accessibility: announced (assistive tech only)")
		}, s.lastActionFor("Accessibility")),

		webviewCard(th, s, ctx),

		action(th, "Open URL", true, "Open the repo", func() {
			_ = ctx.OpenURL("https://github.com/doug/gophics")
			s.setAction("OpenURL: opened in system browser")
		}, s.lastActionFor("OpenURL")),
	)

	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch
	body := widget.Scroll{Child: widget.Padding{Insets: geom.InsetsSymmetric(16, 0), Child: capWidth(col)}}
	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg, Child: body}}
}

// --- live status readers (synchronous getters, read fresh each Build) --------

func connStatus(ctx widget.Ctx) string {
	if c := ctx.Connectivity(); c != nil {
		return boolStr(c.Online(), "online", "offline")
	}
	return ""
}
func battStatus(ctx widget.Ctx) string {
	if b := ctx.Battery(); b != nil {
		return fmt.Sprintf("%d%%%s", int(b.Level()*100+0.5), chargingStr(b.Charging()))
	}
	return ""
}
func lifeStatus(ctx widget.Ctx) string {
	if l := ctx.Lifecycle(); l != nil {
		return "state: " + l.State().String() + "  (switch tabs)"
	}
	return ""
}
func linkStatus(ctx widget.Ctx) string {
	if l := ctx.Links(); l != nil {
		return l.Initial()
	}
	return ""
}

// --- cards -------------------------------------------------------------------

func liveCard(th theme.Theme, title string, have bool, value string) widget.Widget {
	return theme.Card{Child: widget.Row(
		widget.Sized{W: 150, Child: leftText(theme.Body(title))},
		widget.Expand(statusText(th, have, value)),
	)}
}

func action(th theme.Theme, title string, have bool, btn string, onTap func(), result string) widget.Widget {
	right := widget.Widget(theme.Body("— unsupported here"))
	if have {
		right = theme.Button{Label: btn, OnTap: onTap}
	}
	kids := []widget.Widget{widget.Row(
		widget.Sized{W: 150, Child: leftText(theme.Body(title))},
		widget.Expand(leftText(right)),
	)}
	if have && result != "" {
		kids = append(kids, widget.Sized{H: 6},
			widget.Text{S: result, Size: th.Type.Label, Color: th.Muted, Wrap: true})
	}
	c := widget.Column(kids...)
	c.CrossAlign = layout.CrossStretch
	return theme.Card{Child: c}
}

func webviewCard(th theme.Theme, s *state, ctx widget.Ctx) widget.Widget {
	have := ctx.WebView() != nil
	label := "Open web view"
	onTap := func() {
		if s.wv != nil {
			s.wv.Close()
			s.SetState(func() { s.wv = nil })
			return
		}
		h := ctx.WebView().Open("https://example.com", geom.RectXYWH(480, 120, 460, 320))
		s.SetState(func() { s.wv = h })
	}
	if s.wv != nil {
		label = "Close web view"
	}
	return action(th, "Web view (iframe)", have, label,
		onTap, boolStr(s.wv != nil, "open — a real iframe over the canvas", ""))
}

// --- small helpers -----------------------------------------------------------

func sectionLabel(th theme.Theme, s string) widget.Widget {
	return widget.Padding{Insets: geom.Insets{Top: 6, Bottom: 8}, Child: theme.Heading(s)}
}

func leftText(w widget.Widget) widget.Widget {
	c := widget.Column(w)
	c.CrossAlign = layout.CrossStart
	return c
}

func statusText(th theme.Theme, have bool, value string) widget.Widget {
	if !have {
		return leftText(widget.Text{S: "— unsupported here", Size: th.Type.Body, Color: th.Muted})
	}
	return leftText(widget.Text{S: value, Size: th.Type.Body, Color: th.Text, Wrap: true})
}

func (s *state) setAction(msg string) { s.SetState(func() { s.lastAction = msg }) }

func (s *state) lastActionFor(title string) string {
	if strings.HasPrefix(s.lastAction, title) {
		return s.lastAction
	}
	return ""
}

func boolStr(b bool, t, f string) string {
	if b {
		return t
	}
	return f
}

func chargingStr(c bool) string {
	if c {
		return " (charging)"
	}
	return ""
}

func pickResult(f []shell.PickedFile, err error) string {
	if err != nil {
		return "File picker: error"
	}
	if len(f) == 0 {
		return "File picker: cancelled"
	}
	return fmt.Sprintf("File picker: %s (%d bytes)", f[0].Name, len(f[0].Data))
}

const contentMaxW = 760

func capWidth(child widget.Widget) widget.Widget {
	return widget.LayoutBuilder{Build: func(cs layout.Constraints) widget.Widget {
		pad := (cs.Max.W - contentMaxW) / 2
		if pad <= 0 {
			return child
		}
		return widget.Padding{Insets: geom.Insets{Left: pad, Right: pad}, Child: child}
	}}
}

func main() {
	if err := app.Run(App{}, app.Config{
		Title:        "gophics — capabilities",
		Size:         geom.Size{W: 900, H: 720},
		Background:   theme.Light().Bg,
		Font:         goregular.TTF,
		FontFamilies: map[string][]byte{"bold": gobold.TTF},
	}); err != nil {
		log.Fatal(err)
	}
}
