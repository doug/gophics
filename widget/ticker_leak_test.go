package widget

import "testing"

// A widget that registers a ticker must unregister it when it goes away.
//
// This is a leak with no visible symptom: the orphaned controller is ticked on
// every animated frame and keeps requesting frames, so the UI still looks
// correct while the frame loop is pinned awake and the ticker slice grows. It
// has shipped here before — buttons and tappables registered a press-highlight
// controller in Init and implemented no Dispose at all — and a scrolling list
// of tap-rows is enough to grow it without bound.
//
// Nothing in the type system requires the pairing, so this test is what
// enforces it.

// tickerProbe is a Ticker that does nothing but be registered.
type tickerProbe struct{ ticks int }

func (t *tickerProbe) Tick(float64) bool { t.ticks++; return false }

// tidyTicker registers a ticker in Init and releases it in Dispose.
type tidyTicker struct{}

func (tidyTicker) CreateState() State { return &tidyTickerState{} }

type tidyTickerState struct {
	StateBase[tidyTicker]
	ctx Ctx
	t   tickerProbe
}

func (s *tidyTickerState) Init(ctx Ctx) {
	s.ctx = ctx
	ctx.AddTicker(&s.t)
}
func (s *tidyTickerState) Dispose()         { s.ctx.RemoveTicker(&s.t) }
func (s *tidyTickerState) Build(Ctx) Widget { return Sized{W: 10, H: 10} }

// leakyTicker registers a ticker and never releases it — the mistake this
// test exists to catch. It is asserted on below, so the guard itself is
// checked rather than assumed to work.
type leakyTicker struct{}

func (leakyTicker) CreateState() State { return &leakyTickerState{} }

type leakyTickerState struct {
	StateBase[leakyTicker]
	t tickerProbe
}

func (s *leakyTickerState) Init(ctx Ctx)     { ctx.AddTicker(&s.t) }
func (s *leakyTickerState) Build(Ctx) Widget { return Sized{W: 10, H: 10} }

// unmount replaces the tree with a widget of a different type, which disposes
// whatever was there before.
func unmount(o *Owner) {
	o.SetRoot(Sized{W: 1, H: 1})
	o.FlushBuilds()
}

func mount(o *Owner, w Widget) {
	o.SetRoot(w)
	o.FlushBuilds()
}

func TestTickerReleasedOnUnmount(t *testing.T) {
	o := newOwner()
	base := o.TickerCount()

	mount(o, tidyTicker{})
	if got := o.TickerCount(); got != base+1 {
		t.Fatalf("after mount: TickerCount() = %d, want %d — the widget never registered", got, base+1)
	}

	unmount(o)
	if got := o.TickerCount(); got != base {
		t.Errorf("after unmount: TickerCount() = %d, want %d — ticker outlived its widget", got, base)
	}
}

// The guard has to fail on the mistake, or it is decoration. A widget that
// registers without releasing must be caught.
func TestTickerLeakIsDetected(t *testing.T) {
	o := newOwner()
	base := o.TickerCount()

	mount(o, leakyTicker{})
	unmount(o)

	if o.TickerCount() == base {
		t.Fatal("a widget that never calls RemoveTicker went undetected — " +
			"TickerCount is not observing what this test claims it observes")
	}
}

// Mounting and unmounting repeatedly must not accumulate. A single-cycle check
// passes even if release is off by a constant; this is the version that catches
// growth, which is what the scrolling-list case actually does.
func TestRepeatedMountCyclesDoNotAccumulate(t *testing.T) {
	o := newOwner()
	base := o.TickerCount()

	for i := 0; i < 25; i++ {
		mount(o, tidyTicker{})
		unmount(o)
	}

	if got := o.TickerCount(); got != base {
		t.Errorf("after 25 mount/unmount cycles: TickerCount() = %d, want %d — leaking %d per cycle",
			got, base, (got-base)/25)
	}
}

// The catalog widgets in this package that animate. Each is mounted, unmounted,
// and required to leave nothing behind.
func TestCatalogWidgetsReleaseTickers(t *testing.T) {
	cases := []struct {
		name   string
		widget Widget
	}{
		{"Animated", Animated[float32]{
			Value: 1,
			Lerp:  func(a, b float32, t float32) float32 { return a + (b-a)*t },
			Build: func(float32) Widget { return Sized{W: 10, H: 10} },
		}},
		{"Dismissible", Dismissible{Child: Sized{W: 10, H: 10}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := newOwner()
			base := o.TickerCount()

			mount(o, tc.widget)
			unmount(o)

			if got := o.TickerCount(); got != base {
				t.Errorf("%s leaked %d ticker(s) on unmount", tc.name, got-base)
			}
		})
	}
}
