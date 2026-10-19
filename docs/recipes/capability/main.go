// Command capability is the cookbook's platform-capability recipe. It reads
// three capabilities — battery, share and haptics — and shows the shape every
// capability call has: ask the context, and treat nil as an answer.
//
//	go run ./docs/recipes/capability          # desktop: no haptics
//	GOOS=js GOARCH=wasm go build ./docs/recipes/capability
//
// nil means "this platform cannot tell you", and that is information. It is
// not an error and not an empty value: a build that cannot see controllers
// must not be confused with a machine that has none, or the app shows a
// pairing prompt forever. So the right response to nil is almost always to
// hide the affordance, which is what this does.
//
// The framework will not publish a capability that always answers nothing, for
// exactly that reason — desktop connectivity used to report online = true
// unconditionally and is now nil instead, which is the honest answer. The
// per-platform table lives in internal/capgen/README.md and is generated from
// the source, so it cannot drift from what the backends actually implement.
package main

import (
	"fmt"
	"log"

	"golang.org/x/image/font/gofont/gobold"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

type Panel struct{}

func (Panel) CreateState() widget.State { return &panelState{} }

type panelState struct {
	widget.StateBase[Panel]
	subscribed bool
	note       string
}

func (s *panelState) Build(ctx widget.Ctx) widget.Widget {
	th := theme.Auto(ctx)
	s.subscribe(ctx)

	rows := []widget.Widget{
		theme.Title("What this platform can do"),
		widget.Sized{H: 4},
		theme.Caption("Rows that are missing are not broken — the platform does not offer them"),
		widget.Sized{H: 16},
	}

	// Battery: present, so show it. A capability's own methods are the live
	// source; the subscription below is only what tells us to rebuild.
	if b := ctx.Battery(); b != nil {
		state := "on battery"
		if b.Charging() {
			state = "charging"
		}
		rows = append(rows, theme.ListTile{
			Title:    fmt.Sprintf("Battery — %.0f%%", b.Level()*100),
			Subtitle: state,
		})
	} else {
		// Saying so is right for a demo. A real app deletes the row.
		rows = append(rows, theme.ListTile{
			Title:    "Battery",
			Subtitle: "not reported here",
		})
	}

	// Share: the affordance only exists where the platform has a share sheet.
	// There is no fallback worth writing — a hand-rolled share menu that
	// cannot reach the OS targets is worse than no button.
	if sh := ctx.Share(); sh != nil {
		rows = append(rows, theme.ListTile{
			Title:    "Share this recipe",
			Subtitle: "opens the platform share sheet",
			OnTap: func() {
				sh.Share(shell.ShareItem{
					Title: "gophics cookbook",
					Text:  "Reading a platform capability, and handling nil.",
					URL:   "https://github.com/doug/gophics",
				}, func(err error) {
					// Completion, not success: most platforms give no
					// reliable "shared" signal, and a dismissal arrives here
					// with a nil error.
					s.SetState(func() {
						if err != nil {
							s.note = "share failed: " + err.Error()
							return
						}
						s.note = "share sheet closed"
					})
				})
			},
		})
	}

	// Haptics: the common case for a capability that *enriches* an action
	// rather than being the action. The tap must still work without it, so the
	// nil check guards one line and not the handler.
	rows = append(rows, theme.ListTile{
		Title:    "Tap me",
		Subtitle: hapticNote(ctx),
		OnTap: func() {
			if h := ctx.Haptic(); h != nil {
				h.Play(shell.HapticSelection)
			}
			s.SetState(func() { s.note = "tapped" })
		},
	})

	if s.note != "" {
		rows = append(rows,
			widget.Sized{H: 12},
			widget.Text{Value: s.note, Size: th.Type.Caption, Color: th.Muted})
	}

	col := widget.Column(rows...)
	col.CrossAlign = layout.CrossStretch

	return widget.Provide[theme.Theme]{Value: th, Child: widget.Fill{Color: th.Bg,
		Child: widget.Padding{All: 18, Child: col}}}
}

// subscribe registers for change notifications once.
//
// Capabilities are nil at mount — the tree builds before the shell is wired —
// and arrive on the first frame, with a rebuild when they do. So this belongs
// in Build and not in Init, guarded to run exactly once. Getting that wrong is
// the standard way an app concludes a capability is missing when it is not.
func (s *panelState) subscribe(ctx widget.Ctx) {
	if s.subscribed {
		return
	}
	b := ctx.Battery()
	if b == nil {
		return // either not wired yet, or genuinely absent — retry next frame
	}
	s.subscribed = true
	// OnChange takes no argument: it says something moved, and the app reads
	// Level() and Charging() again on the next build.
	b.OnChange(func() { s.SetState(nil) })
}

func hapticNote(ctx widget.Ctx) string {
	if ctx.Haptic() == nil {
		return "no haptics here — the tap still works"
	}
	return "with a selection tick"
}

func main() {
	if err := app.Run(Panel{}, config()); err != nil {
		log.Fatal(err)
	}
}

func config() app.Config {
	return app.Config{
		Title:          "recipe · capabilities",
		AppID:          "com.gophics.cookbook.capability",
		Size:           geom.Size{W: 440, H: 420},
		Background:     theme.Light().Bg,
		BackgroundDark: theme.Dark().Bg,
		Font:           goregular.TTF,
		FontFamilies:   map[string][]byte{theme.FontBold: gobold.TTF},
	}
}
