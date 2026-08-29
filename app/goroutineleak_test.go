package app_test

import (
	"bytes"
	"runtime"
	"runtime/pprof"
	"testing"
	"time"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/widget"
)

// A finished app must leave no goroutine blocked forever.
//
// gophics starts them in several places — the Post pump, image fetches, the
// caret blink timer, and whatever an app's own Init launches — and every one is
// a goroutine parked on a channel that something else is supposed to close. The
// tell for a leak is not a crash but a process that never quiets: the frame loop
// keeps being woken, or the test binary keeps a channel alive after the tree
// that owned it is gone.
//
// Until Go 1.27 that could only be approximated by counting goroutines and
// hoping the count settled, which is racy — a goroutine on its way out looks
// exactly like one that is stuck. runtime/pprof's goroutineleak profile reports
// the ones the runtime can prove are unreachable *and* blocked, which is the
// actual definition and needs no sleeping.
//
// theme's TestControlsReleaseTickersOnUnmount covers the neighbouring problem —
// gophics's own ticker registry — which is bookkeeping rather than goroutines.
// The two do not overlap.
func TestAppLifecycleLeaksNoGoroutines(t *testing.T) {
	if pprof.Lookup("goroutineleak") == nil {
		t.Skip("goroutineleak profile needs Go 1.27")
	}

	// A tree with the things that actually start goroutines: a text field (blink
	// timer), and state that posts back to the UI thread.
	run := func() {
		h, err := app.NewHeadless(leakProbe{}, app.Config{
			Size: geom.Size{W: 200, H: 120}, Background: paint.RGB(1, 1, 1), Font: goregular.TTF,
		}, 1)
		if err != nil {
			t.Fatal(err)
		}
		h.Render()
		h.Tap(geom.Pt{X: 100, Y: 60}) // focus the field: starts the caret blink
		for range 30 {
			h.Step(1.0 / 60)
		}
		h.Render()
	}
	run()

	// The profile reports goroutines the runtime can prove are unreachable and
	// blocked. That proof needs a GC, and a goroutine dropped during this one
	// is only unreachable after the next — so two, which is what makes this
	// deterministic rather than timing-dependent.
	runtime.GC()
	runtime.GC()

	p := pprof.Lookup("goroutineleak")
	if n := p.Count(); n > 0 {
		var buf bytes.Buffer
		_ = p.WriteTo(&buf, 1)
		t.Errorf("%d leaked goroutine(s) after the app was dropped:\n%s", n, buf.String())
	}
}

type leakProbe struct{}

func (leakProbe) CreateState() widget.State { return &leakProbeState{} }

type leakProbeState struct {
	widget.StateBase[leakProbe]
	text string
}

func (s *leakProbeState) Init(ctx widget.Ctx) {
	// The shape an app uses for background work: do something off the UI
	// goroutine and post the result back. A Post that outlives its tree is
	// exactly the leak this test is looking for.
	post := ctx.Post()
	go func() {
		time.Sleep(time.Millisecond)
		post(func() { s.SetState(func() { s.text = "loaded" }) })
	}()
}

func (s *leakProbeState) Build(widget.Ctx) widget.Widget {
	return widget.Center(widget.Sized{
		W: 160, H: 40,
		Child: widget.TextField{
			Value:    s.text,
			OnChange: func(v string) { s.SetState(func() { s.text = v }) },
		},
	})
}
