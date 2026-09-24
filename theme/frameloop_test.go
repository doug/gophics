package theme_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/apptest"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/theme"
	"github.com/doug/gophics/widget"
)

// progApp shows a Progress whose value the test flips after mount.
type progApp struct{ hook func(*progState) }

func (a progApp) CreateState() widget.State { return &progState{hook: a.hook} }

type progState struct {
	widget.StateBase[progApp]
	hook func(*progState)
	v    float32
}

func (s *progState) Init(widget.Ctx) { s.v = 0.5; s.hook(s) }

func (s *progState) Build(widget.Ctx) widget.Widget {
	return widget.Provide[theme.Theme]{Value: theme.Light(), Child: widget.Padding{All: 20,
		Child: theme.Progress{Value: s.v, Label: "Upload"}}}
}

// A Progress that turns indeterminate after mount starts its sweep from
// Build, after the frame's tick has run. The frame pipeline asks
// TickersActive after building to catch exactly that, so the ticker has to
// report itself — or the sweep waits for an unrelated event to move.
func TestProgressIndeterminateAfterMountKeepsFramesComing(t *testing.T) {
	var st *progState
	a := apptest.New(t, progApp{hook: func(s *progState) { st = s }},
		apptest.WithConfig(app.Config{Size: geom.Size{W: 300, H: 100}, Font: goregular.TTF}))
	a.Render()
	if a.Owner().TickersActive() {
		t.Fatal("a determinate Progress reports a live ticker")
	}
	st.SetState(func() { st.v = -1 })
	a.Render() // the build that starts the sweep
	if !a.Owner().TickersActive() {
		t.Fatal("indeterminate Progress is invisible to the post-build ticker check")
	}
	if !a.Step(1.0 / 60) {
		t.Fatal("the sweep does not advance")
	}
}

// Hovering a Tooltip over a stateless child: nothing in the tree calls
// SetState on hover, and pointer moves do not request frames, so the hover
// countdown has to ask for one itself and keep reporting itself live.
func TestTooltipHoverOverStaticChildRequestsFrames(t *testing.T) {
	a := apptest.New(t, widget.Provide[theme.Theme]{Value: theme.Light(),
		Child: widget.Center(theme.Tooltip{Message: "Tip text",
			Child: theme.Icon{Glyph: theme.IconHome, Size: 40}})},
		apptest.WithConfig(app.Config{Size: geom.Size{W: 400, H: 600}, Font: goregular.TTF}))
	a.Render()
	frames := 0
	a.Owner().RequestFrame = func() { frames++ }
	a.Move(geom.Pt{X: 200, Y: 300})
	if frames == 0 {
		t.Fatal("entering the tooltip's child requested no frame")
	}
	if !a.Owner().TickersActive() {
		t.Fatal("the hover countdown is invisible to the post-build ticker check")
	}
	for range 45 {
		a.Step(1.0 / 60)
		a.Render()
	}
	if !a.HasText("Tip text") {
		t.Fatal("tooltip did not appear after the delay")
	}
	if a.Owner().TickersActive() {
		t.Fatal("the countdown reports live after it fired")
	}
}
