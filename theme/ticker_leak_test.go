package theme

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// Every animating control must release its ticker when it leaves the tree.
//
// This is where the bug actually shipped: Button and Tappable each registered a
// press-highlight controller in Init and implemented no Dispose, so the
// controller stayed in Owner.tickers, was advanced on every animated frame
// forever, and kept requesting frames. A list of tap-rows scrolling in and out
// grew the slice without bound and pinned the frame loop awake — with nothing
// wrong on screen to suggest it.
//
// Every other stateful widget in this package paired AddTicker with
// RemoveTicker correctly, which is the tell: the pairing is a convention the
// compiler cannot enforce, so it holds only while someone remembers. This test
// is what remembers.
func TestControlsReleaseTickersOnUnmount(t *testing.T) {
	cases := []struct {
		name   string
		widget widget.Widget
	}{
		{"Button", Button{Label: "ok", OnTap: func() {}}},
		{"Tappable", Tappable{Child: widget.Sized{W: 10, H: 10}, OnTap: func() {}}},
		{"Switch", Switch{}},
		{"Tooltip", Tooltip{Message: "tip", Child: widget.Sized{W: 10, H: 10}}},
		{"Progress", Progress{}},
		{"Spinner", Spinner{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := &widget.Owner{}
			base := o.TickerCount()

			o.SetRoot(tc.widget)
			o.FlushBuilds()

			// Replacing the root with a different widget type disposes the old one.
			o.SetRoot(widget.Sized{W: 1, H: 1})
			o.FlushBuilds()

			if got := o.TickerCount(); got != base {
				t.Errorf("%s leaked %d ticker(s) on unmount — Init calls AddTicker "+
					"with no matching RemoveTicker in Dispose", tc.name, got-base)
			}
		})
	}
}

// Repeated cycles are what the scrolling-list case actually does. A single
// mount/unmount passes even when release is off by a constant; this catches
// per-cycle growth.
func TestControlsDoNotAccumulateTickers(t *testing.T) {
	o := &widget.Owner{}
	base := o.TickerCount()

	for i := 0; i < 20; i++ {
		o.SetRoot(Button{Label: "ok", OnTap: func() {}})
		o.FlushBuilds()
		o.SetRoot(widget.Sized{W: 1, H: 1})
		o.FlushBuilds()
	}

	if got := o.TickerCount(); got != base {
		t.Errorf("after 20 Button mount/unmount cycles: TickerCount() = %d, want %d "+
			"— leaking %d per cycle", got, base, (got-base)/20)
	}
}
