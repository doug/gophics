package widget

import "testing"

// An Animated builds once for a mount and once for a value change; the
// controller is re-based silently, so settling it does not dirty the widget.
//
// Init settled the controller with Jump, whose OnChange called SetState on a
// widget that had not built yet, so every Animated mounted dirty and built
// twice; Build did the same on each new value.
func TestAnimatedBuildsOncePerChange(t *testing.T) {
	builds := 0
	root := func(v float32) Widget {
		return AnimateFloat(v, 0, func(v float32) Widget { builds++; return Sized{W: v} })
	}
	o := newOwner()
	o.SetRoot(root(1))
	if o.NeedsBuild() {
		t.Fatal("Animated mounted dirty")
	}
	o.FlushBuilds()
	if builds != 1 {
		t.Fatalf("Animated built %d times for one mount, want 1", builds)
	}

	o.SetRoot(root(0))
	o.FlushBuilds()
	if builds != 2 {
		t.Fatalf("Animated built %d times after a value change, want 2", builds)
	}
	if !o.TickersActive() {
		t.Fatal("the value change did not start the tween")
	}
	// The tween then runs on the frame clock, rebuilding as it goes.
	o.TickAll(0.05)
	o.FlushBuilds()
	if builds != 3 {
		t.Fatalf("Animated built %d times after a tick, want 3", builds)
	}
}
