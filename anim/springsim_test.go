package anim

import (
	"math"
	"testing"
	"time"
)

// run ticks sim at a fixed dt until it settles or tickCap frames pass, and
// returns every position it passed through (the start position first).
func run(sim *SpringSimulation, dt float64, tickCap int) []float32 {
	out := []float32{sim.Value()}
	for i := 0; i < tickCap; i++ {
		running := sim.Tick(dt)
		out = append(out, sim.Value())
		if !running {
			break
		}
	}
	return out
}

func extremes(xs []float32) (lo, hi float32) {
	lo, hi = xs[0], xs[0]
	for _, x := range xs {
		if x < lo {
			lo = x
		}
		if x > hi {
			hi = x
		}
	}
	return lo, hi
}

// The point of the whole exercise: a gesture released at speed hands that speed
// to the spring, and the first frame after the handoff moves at it. A
// fixed-duration tween always leaves from rest, so its first frame moves by
// almost nothing and the motion visibly stalls and restarts — that step is
// what reads as "web" rather than "native".
func TestSpringHandsOffVelocityWithoutADiscontinuity(t *testing.T) {
	const (
		x0 = 60.0   // px past the edge at release
		v0 = 1400.0 // px/s the finger was travelling
	)
	spec := SpringSpec{Stiffness: 220} // critically damped, ωₙ = √220
	sim := &SpringSimulation{Spec: spec, Rest: 0.4, RestVelocity: 6}
	sim.Start(x0, v0)
	if got := sim.Velocity(); got != v0 {
		t.Fatalf("Start did not take the velocity: Velocity() = %v, want %v", got, v0)
	}

	// Continuity is a claim about an instant, so measure it at one: as the
	// step shrinks, the first step's average speed has to converge on the
	// speed that was handed in. A fixed-duration tween cannot pass this at
	// any step size, because it always leaves from rest — which is the
	// failure this whole type exists to remove.
	for _, dt := range []float64{1e-3, 1e-4} {
		s := &SpringSimulation{Spec: spec, Rest: 0.4, RestVelocity: 6}
		s.Start(x0, v0)
		s.Tick(dt)
		speed := float64(s.Value()-x0) / dt
		// One step of the spring's own force is a*dt and shrinks with dt;
		// anything larger is a discontinuity rather than acceleration.
		tol := math.Abs(float64(-220*x0-2*math.Sqrt(220)*v0)) * dt * 1.5
		if math.Abs(speed-v0) > tol {
			t.Errorf("dt=%g: first step travelled at %.1f px/s, want %v ± %.1f", dt, speed, v0, tol)
		}
	}

	// At a real frame time the handoff still has to land close to the true
	// motion, or the "no visible jump" claim is only true in the limit. The
	// closed form for a critically damped spring settling to 0 is
	// x(t) = (x₀ + (v₀ + ωₙ·x₀)·t)·e^(−ωₙ·t).
	const dt = 1.0 / 60
	omega := math.Sqrt(220)
	exact := (x0 + (v0+omega*x0)*dt) * math.Exp(-omega*dt)
	sim.Tick(dt)
	if rel := math.Abs(float64(sim.Value())-exact) / (exact - x0); rel > 0.12 {
		t.Errorf("first frame reached %v, exact solution %v (%.0f%% of the travel missing)",
			sim.Value(), exact, rel*100)
	}

	// And the same spring given no velocity must move a small fraction of
	// that in the same frame, or the comparison proves nothing.
	rest := &SpringSimulation{Spec: spec, Rest: 0.4, RestVelocity: 6}
	rest.Start(x0, 0)
	rest.Tick(dt)
	if moved := math.Abs(float64(rest.Value() - x0)); moved > math.Abs(float64(sim.Value()-x0))/5 {
		t.Errorf("a spring started from rest moved %.2f px, nearly as far as one handed %v px/s; "+
			"the velocity is not reaching the integration", moved, v0)
	}
}

// Target is read every tick, so moving it mid-flight retargets without
// interrupting: the velocity carries straight over. That is the other half of
// handoff — a destination that changes under the finger (a sheet whose detent
// moves) must not restart the motion either.
func TestSpringRetargetKeepsVelocity(t *testing.T) {
	sim := &SpringSimulation{Spec: SpringSpec{Stiffness: 220}, Rest: 0.01}
	sim.Start(0, 800)
	for i := 0; i < 6; i++ {
		sim.Tick(1.0 / 60)
	}
	before := sim.Velocity()
	if before == 0 {
		t.Fatalf("spring is not moving, so there is no velocity to carry over")
	}
	sim.Target = 50
	if after := sim.Velocity(); after != before {
		t.Errorf("retarget changed the velocity from %v to %v", before, after)
	}
	if !sim.Running() {
		t.Error("retarget stopped the simulation")
	}
	xs := run(sim, 1.0/60, 600)
	if sim.Running() {
		t.Fatal("retargeted spring never settled")
	}
	if sim.Value() != 50 {
		t.Errorf("settled at %v, want the new target 50 (path length %d)", sim.Value(), len(xs))
	}
}

// All three damping regimes come out of one integration, and each has to
// actually behave like its name — an "underdamped" spring that does not
// overshoot is just a slow ease, and that is the thing a spring is for.
func TestSpringDampingRegimes(t *testing.T) {
	const dt = 1.0 / 60

	under := &SpringSimulation{Spec: SpringWith(300*time.Millisecond, 0.4), Rest: 0.01}
	crit := &SpringSimulation{Spec: SpringWith(300*time.Millisecond, 1.0), Rest: 0.01}
	over := &SpringSimulation{Spec: SpringWith(300*time.Millisecond, 2.5), Rest: 0.01}

	for name, sim := range map[string]*SpringSimulation{"under": under, "crit": crit, "over": over} {
		if got := sim.Spec.DampingRatio(); math.Abs(float64(got)-wantRatio(name)) > 1e-4 {
			t.Fatalf("%s: DampingRatio() = %v, want %v", name, got, wantRatio(name))
		}
		sim.Start(1, 0) // released from 1, settling to 0
	}

	underXs := run(under, dt, 2000)
	critXs := run(crit, dt, 2000)
	overXs := run(over, dt, 2000)

	lo, _ := extremes(underXs)
	if lo > -0.05 {
		t.Errorf("underdamped spring undershot only to %v; ζ=0.4 must cross its target by a visible margin", lo)
	}
	if lo, _ := extremes(critXs); lo < -0.02 {
		t.Errorf("critically damped spring reached %v; ζ=1 must not overshoot", lo)
	}
	if lo, _ := extremes(overXs); lo < -0.02 {
		t.Errorf("overdamped spring reached %v; ζ=2.5 must not overshoot", lo)
	}
	// Critical damping is by definition the fastest non-overshooting
	// approach, so the overdamped one must take longer to arrive.
	if len(overXs) <= len(critXs) {
		t.Errorf("overdamped settled in %d frames, critical in %d; overdamped must be the slower of the two",
			len(overXs), len(critXs))
	}
	for _, sim := range []*SpringSimulation{under, crit, over} {
		if sim.Running() || sim.Value() != 0 {
			t.Errorf("spring (ζ=%v) ended at %v running=%v", sim.Spec.DampingRatio(), sim.Value(), sim.Running())
		}
	}
}

func wantRatio(name string) float64 {
	switch name {
	case "under":
		return 0.4
	case "over":
		return 2.5
	}
	return 1.0
}

// A simulation has no duration, so nothing else makes it stop: without a
// settling criterion it approaches its target asymptotically, keeps reporting
// itself active, and holds the frame loop awake for the life of the widget —
// the exact leak Owner.TickerCount exists to catch.
func TestSpringSettlingTerminates(t *testing.T) {
	cases := []struct {
		name string
		spec SpringSpec
		x0   float32
		v0   float32
	}{
		{"zero value from rest", SpringSpec{}, 80, 0},
		{"edge bounce", SpringSpec{Stiffness: 220}, 0, 2000},
		{"soft and bouncy", SpringWith(600*time.Millisecond, 0.3), 1, -4},
		{"very stiff", SpringSpec{Stiffness: 20000}, 10, 500},
		{"heavy overdamping", SpringSpec{Stiffness: 100, Damping: 80}, 25, 0},
		{"already at rest", SpringSpec{}, 0, 0},
	}
	for _, c := range cases {
		sim := &SpringSimulation{Spec: c.spec, Rest: 0.4, RestVelocity: 6}
		sim.Start(c.x0, c.v0)
		frames := 0
		for sim.Tick(1.0 / 60) {
			frames++
			if frames > 600 { // ten seconds; nothing in a UI settles that slowly
				t.Errorf("%s: still running after %d frames at %v (v=%v)", c.name, frames, sim.Value(), sim.Velocity())
				break
			}
		}
		if sim.Running() {
			continue
		}
		// Settling snaps to the target rather than leaving a residue: a band
		// parked 0.3px out stays 0.3px out forever.
		if sim.Value() != sim.Target || sim.Velocity() != 0 {
			t.Errorf("%s: settled at %v with velocity %v, want exactly the target and 0",
				c.name, sim.Value(), sim.Velocity())
		}
	}
}

// Two springs handed the same dt sequence must trace the same path. Frame
// times vary, so the integrator may not carry hidden per-instance state or
// consult a clock of its own; a golden test of a gesture would be untestable
// otherwise.
func TestSpringIsDeterministicAtAFixedDt(t *testing.T) {
	mk := func() *SpringSimulation {
		s := &SpringSimulation{Spec: SpringWith(250*time.Millisecond, 0.7), Rest: 0.01}
		s.Start(1, -3.5)
		return s
	}
	a, b := mk(), mk()
	for i := 0; i < 200; i++ {
		ra, rb := a.Tick(1.0/60), b.Tick(1.0/60)
		if ra != rb || a.Value() != b.Value() || a.Velocity() != b.Velocity() {
			t.Fatalf("diverged at frame %d: %v/%v vs %v/%v", i, a.Value(), a.Velocity(), b.Value(), b.Velocity())
		}
		if !ra {
			break
		}
	}
	if a.Running() {
		t.Fatal("200 frames and still running")
	}
}

// Semi-implicit Euler is only conditionally stable: stepped past ωₙ·h ≈ 2 it
// diverges instead of settling. Dropped frames and stalled compositors hand
// ticks far longer than a frame, so the solver subdivides. This proves it —
// and proves the hazard is real by running the unsubdivided integrator the
// scroll view used to carry over the same dt and watching it blow up.
func TestSpringSurvivesADroppedFrame(t *testing.T) {
	for _, dt := range []float64{0.05, 0.1, 0.25, 0.5, 1.0, 4.0} {
		sim := &SpringSimulation{Spec: SpringSpec{Stiffness: 220}, Rest: 0.4, RestVelocity: 6}
		sim.Start(100, 2000)
		for i := 0; i < 200; i++ {
			v := float64(sim.Value())
			if math.IsNaN(v) || math.IsInf(v, 0) {
				t.Fatalf("dt=%v: value went to %v at step %d", dt, v, i)
			}
			// The analytic motion peaks well under this; anything past it is
			// the integrator gaining energy, which is what divergence looks
			// like before it reaches Inf.
			if math.Abs(v) > 1000 {
				t.Fatalf("dt=%v: value reached %v at step %d — the simulation is gaining energy", dt, v, i)
			}
			if !sim.Tick(dt) {
				break
			}
		}
		if sim.Running() {
			t.Errorf("dt=%v: never settled", dt)
		}
		if sim.Value() != 0 {
			t.Errorf("dt=%v: settled at %v, want 0", dt, sim.Value())
		}
	}

	// The same spring, one unsubdivided step per tick — the integrator this
	// replaced. At a dropped frame's dt it diverges, which is the reason the
	// subdivision is not optional.
	const k, dt = 220.0, 0.1
	d := 2 * math.Sqrt(k)
	x, v := 100.0, 2000.0
	diverged := false
	for i := 0; i < 50; i++ {
		v += (-k*x - d*v) * dt
		x += v * dt
		if math.Abs(x) > 1e6 {
			diverged = true
			break
		}
	}
	if !diverged {
		t.Error("the unsubdivided integrator stayed bounded at dt=0.1; " +
			"if that is now true, TestSpringSurvivesADroppedFrame is no longer testing anything")
	}
}

// The scroll view's edge bounce was measured against native scroll views and
// its physics must survive being promoted out of widget/scroll.go. That is
// what MaxStep: 0.02 buys — at any frame time up to 20ms the subdivision is a
// no-op, so the promoted solver reproduces the old private one's arithmetic
// exactly. Exactly, not approximately: a few percent per frame compounds over
// a settle, and the bounce was tuned against a device.
func TestSpringMatchesTheScrollBounceItReplaced(t *testing.T) {
	const (
		k  = 220.0 // widget.overStiffness
		x0 = 73.0
		v0 = -1250.0
	)
	for _, dt := range []float64{1.0 / 60, 1.0 / 120, 1.0 / 90} {
		sim := &SpringSimulation{Spec: SpringSpec{Stiffness: k}, MaxStep: 0.02, Rest: 0.4, RestVelocity: 6}
		sim.Start(x0, v0)

		// The integrator widget/scroll.go carried, verbatim.
		x, v, active := float32(x0), float32(v0), true
		d := 2 * float32(math.Sqrt(k))
		for i := 0; active; i++ {
			v += (-k*x - d*v) * float32(dt)
			x += v * float32(dt)
			if x > -0.4 && x < 0.4 && v > -6 && v < 6 {
				x, v, active = 0, 0, false
			}
			running := sim.Tick(dt)
			if sim.Value() != x || sim.Velocity() != v {
				t.Fatalf("dt=%v frame %d: promoted solver at %v/%v, original at %v/%v",
					dt, i, sim.Value(), sim.Velocity(), x, v)
			}
			if running != active {
				t.Fatalf("dt=%v frame %d: running=%v, original active=%v", dt, i, running, active)
			}
			if i > 1000 {
				t.Fatal("neither settled")
			}
		}
	}
}

// SpringWith is the designer-facing form, and it has to actually mean what it
// says: the response is the spring's natural period and the fraction is ζ.
func TestSpringWith(t *testing.T) {
	spec := SpringWith(300*time.Millisecond, 0.8)
	wantOmega := 2 * math.Pi / 0.3
	if got := float64(spec.Frequency()); math.Abs(got-wantOmega)/wantOmega > 1e-5 {
		t.Errorf("Frequency() = %v, want %v", got, wantOmega)
	}
	if got := spec.DampingRatio(); math.Abs(float64(got)-0.8) > 1e-5 {
		t.Errorf("DampingRatio() = %v, want 0.8", got)
	}
	// Nonsense in must not produce a spring that cannot settle, because an
	// unsettling spring pins the frame loop awake rather than looking wrong.
	for _, bad := range []SpringSpec{SpringWith(0, 0.8), SpringWith(300*time.Millisecond, 0), SpringWith(-1, -1)} {
		if got := bad.DampingRatio(); math.Abs(float64(got)-1) > 1e-5 {
			t.Errorf("degenerate SpringWith gave ζ=%v, want the critically damped fallback", got)
		}
	}
	// The zero value is the documented scroll-bounce spring.
	var zero SpringSpec
	if got := zero.Frequency(); math.Abs(float64(got)-math.Sqrt(defaultStiffness)) > 1e-4 {
		t.Errorf("zero SpringSpec Frequency() = %v, want √%v", got, float64(defaultStiffness))
	}
	if got := zero.DampingRatio(); math.Abs(float64(got)-1) > 1e-5 {
		t.Errorf("zero SpringSpec is not critically damped: ζ=%v", got)
	}
}

// Stop and Start are what a gesture grabbing a spring mid-flight uses, and
// OnChange must fire for every frame that moved — including the one that
// settles, or the last few pixels never get drawn.
func TestSpringStopStartAndOnChange(t *testing.T) {
	changes := 0
	sim := &SpringSimulation{Spec: SpringSpec{Stiffness: 220}, Rest: 0.4, RestVelocity: 6,
		OnChange: func() { changes++ }}
	if sim.Tick(1.0 / 60) {
		t.Fatal("an idle simulation must not tick")
	}
	if changes != 0 {
		t.Fatal("an idle tick called OnChange")
	}
	sim.Start(50, 900)
	sim.Tick(1.0 / 60)
	sim.Tick(1.0 / 60)
	if changes != 2 {
		t.Fatalf("OnChange fired %d times over 2 frames, want 2", changes)
	}
	held, grabbed := sim.Value(), sim.Velocity()
	if grabbed == 0 {
		t.Fatal("nothing to hand back")
	}
	sim.Stop()
	if sim.Running() {
		t.Error("Stop left the simulation running")
	}
	if sim.Value() != held {
		t.Errorf("Stop moved the value from %v to %v; a grabbed spring must stay where it is drawn", held, sim.Value())
	}
	if sim.Velocity() != 0 {
		t.Errorf("Stop left a stale velocity of %v", sim.Velocity())
	}
	changes = 0
	sim.Tick(1.0 / 60)
	if changes != 0 {
		t.Error("a stopped simulation still called OnChange")
	}
	// Handing the grabbed speed back in resumes the motion where it left off.
	sim.Start(held, grabbed)
	sim.Tick(1.0 / 60)
	if changes != 1 || !sim.Running() {
		t.Errorf("restart: changes=%d running=%v", changes, sim.Running())
	}
}

// A NaN dt poisons the state and then can never satisfy the settling test, so
// the simulation would claim to be running forever and pin the frame loop
// awake with nothing moving.
func TestSpringIgnoresANaNTick(t *testing.T) {
	sim := &SpringSimulation{Spec: SpringSpec{Stiffness: 220}, Rest: 0.4, RestVelocity: 6}
	sim.Start(40, 500)
	sim.Tick(1.0 / 60)
	x, v := sim.Value(), sim.Velocity()
	if !sim.Tick(math.NaN()) {
		t.Error("a NaN tick deactivated the simulation")
	}
	if sim.Value() != x || sim.Velocity() != v {
		t.Fatalf("a NaN tick moved the simulation to %v/%v from %v/%v", sim.Value(), sim.Velocity(), x, v)
	}
	frames := 0
	for sim.Tick(1.0 / 60) {
		if frames++; frames > 600 {
			t.Fatal("never settled after a NaN tick")
		}
	}
	if sim.Value() != 0 {
		t.Errorf("settled at %v, want 0", sim.Value())
	}
}
