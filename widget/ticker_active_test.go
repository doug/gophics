package widget

import "testing"

// runProbe is a Ticker that reports whether it is animating, the way
// anim.Controller does.
type runProbe struct{ running bool }

func (r *runProbe) Tick(float64) bool { return r.running }
func (r *runProbe) Running() bool     { return r.running }

// mute is a Ticker that does not report its state; it must be treated as idle
// rather than crashing the query.
type mute struct{}

func (mute) Tick(float64) bool { return true }

// An animation started during Build must be visible to the frame loop
// afterwards. The pipeline ticks before it builds, so the tick that already ran
// cannot have seen it; if nothing asks again, the animation never gets a second
// frame and freezes at its start value while the rest of the UI has moved on.
// That is the switch whose knob does not slide.
type animStarter struct{ probe *runProbe }

func (a animStarter) CreateState() State { return &animStarterState{} }

type animStarterState struct {
	StateBase[animStarter]
	ctx     Ctx
	started bool
}

func (s *animStarterState) Init(ctx Ctx) { s.ctx = ctx; ctx.AddTicker(s.W().probe) }
func (s *animStarterState) Dispose()     { s.ctx.RemoveTicker(s.W().probe) }

func (s *animStarterState) Build(Ctx) Widget {
	// Exactly what theme.Switch does: react to the new state by starting an
	// animation, from inside Build.
	if !s.started {
		s.started = true
		s.W().probe.running = true
	}
	return Sized{W: 10, H: 10}
}

func TestTickersActiveSeesAnimationStartedDuringBuild(t *testing.T) {
	o := newOwner()
	probe := &runProbe{}

	if o.TickersActive() {
		t.Fatal("no tickers registered, but TickersActive() is true")
	}

	o.SetRoot(animStarter{probe: probe})
	o.FlushBuilds() // the Build starts the animation

	if !o.TickersActive() {
		t.Error("an animation started during Build is invisible to TickersActive — " +
			"the frame loop would not request another frame and it would freeze")
	}

	probe.running = false
	if o.TickersActive() {
		t.Error("TickersActive() still true after the animation finished")
	}
}

// A ticker that does not report its state must not be assumed to be animating,
// or the frame loop would spin forever.
func TestTickersActiveIgnoresTickersThatDoNotReport(t *testing.T) {
	o := newOwner()
	o.AddTicker(mute{})

	if o.TickersActive() {
		t.Error("a ticker with no Running() was treated as animating; the frame " +
			"loop would never go idle")
	}
}
