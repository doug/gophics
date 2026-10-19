package anim

import (
	"math"
	"time"
)

// SpringSpec is a spring's physical parameterisation: the three constants of
// m·x″ = −k·(x − target) − c·x′.
//
// The fields are the physics rather than a designer-facing shorthand because
// this is what the solver integrates, and because the numbers already in the
// tree are expressed this way — the scroll view's edge bounce has been tuned
// at stiffness 220 against measured native scroll views, and a spec that could
// not say "220" exactly would have forced that value to be re-derived through
// a conversion. SpringWith is the shorthand, and it builds one of these, so
// there is still only one description of a spring.
//
// The zero value is a critically damped spring at stiffness 220 — the scroll
// bounce's spring, which is a reasonable default for anything settling against
// an edge. Zero fields are filled individually, so SpringSpec{Stiffness: 400}
// is a stiffer critically damped spring and nothing else needs saying.
type SpringSpec struct {
	// Stiffness is k, in units of acceleration per unit of displacement
	// (1/s² when Mass is 1). Higher is faster and tighter. 0 → 220.
	Stiffness float32
	// Damping is c, the velocity-opposing force coefficient. 0 → critical
	// damping, 2·√(k·m), the fastest approach that does not overshoot.
	//
	// Below critical the spring overshoots and oscillates in; above it
	// crawls in. There is deliberately no way to ask for zero damping: an
	// undamped spring never settles, so it would never release the frame
	// loop.
	Damping float32
	// Mass is m. 0 → 1. Changing it is a second way to scale the response,
	// and usually leaving it alone and moving Stiffness is clearer; it is
	// here because a spec that fixes mass is not a physical spec, and
	// callers porting constants from one do exist.
	Mass float32
}

// defaultStiffness is the scroll view's edge-bounce spring. See SpringSpec.
const defaultStiffness = 220

// SpringWith builds a spec from the form a UI author can reason about:
// response is roughly how long one cycle of the spring takes — the time it
// needs to make its move — and dampingFraction is the damping ratio ζ, where
// 1 arrives without overshoot, below 1 overshoots by more the lower it goes,
// and above 1 is sluggish. 0.8 overshoots a few percent and is the usual
// choice for motion that should read as elastic.
//
// It exists next to the physical form because the physical form is unhelpful
// at a call site: nobody can see that stiffness 220 settles in about 300ms,
// but everybody can see what SpringWith(300*time.Millisecond, 0.8) does. The
// conversion is ωₙ = 2π/response, k = m·ωₙ², c = 2·ζ·m·ωₙ.
//
// A response of 0 or less, or a dampingFraction of 0 or less, falls back to
// the SpringSpec zero value's critically damped default rather than producing
// a spring that cannot settle.
func SpringWith(response time.Duration, dampingFraction float32) SpringSpec {
	if response <= 0 || dampingFraction <= 0 {
		return SpringSpec{}
	}
	const m = 1
	omega := float32(2 * math.Pi / response.Seconds())
	return SpringSpec{
		Stiffness: m * omega * omega,
		Damping:   2 * dampingFraction * m * omega,
		Mass:      m,
	}
}

// resolve fills the zero fields and returns the constants the solver uses.
func (sp SpringSpec) resolve() (k, c, m float32) {
	m = sp.Mass
	if m <= 0 {
		m = 1
	}
	k = sp.Stiffness
	if k <= 0 {
		k = defaultStiffness
	}
	c = sp.Damping
	if c <= 0 {
		c = 2 * float32(math.Sqrt(float64(k*m))) // critical: c = 2√(km)
	}
	return k, c, m
}

// Frequency returns the undamped natural frequency ωₙ = √(k/m), in radians per
// second. It is the scale every other number about a spring is relative to:
// the settling tolerances default to it, and so does the integrator's step.
func (sp SpringSpec) Frequency() float32 {
	k, _, m := sp.resolve()
	return float32(math.Sqrt(float64(k / m)))
}

// DampingRatio returns ζ = c / (2·√(k·m)): below 1 underdamped (overshoots and
// rings in), exactly 1 critically damped (the fastest approach with no
// overshoot), above 1 overdamped (approaches without overshoot, slowly).
//
// All three regimes come out of the same integration — there are no cases in
// the solver — so this is for callers and tests that want to say which motion
// they asked for.
func (sp SpringSpec) DampingRatio() float32 {
	k, c, m := sp.resolve()
	return c / (2 * float32(math.Sqrt(float64(k*m))))
}

// SpringSimulation integrates a damped spring toward Target, from a given
// position *and velocity*, on the frame ticker contract.
//
// It is not a Curve and it is not Spring (which is one — see the note on
// Spring for why the two are different animals and must not be confused). A
// Curve maps progress to progress over a duration decided in advance; this
// integrates state forward through whatever time actually passed and has no
// duration at all. It is done when it has stopped moving, which is a property
// of the motion rather than of a clock.
//
// On the name: the obvious move was to rename the curve to SpringCurve and
// take Spring for the solver, and it was rejected on both counts. It breaks
// every existing caller and the measured API manifest, and what it buys is the
// ambiguous word: "Spring" on its own does not say which of the two things it
// is, so anyone who meets both still has to go and look. A name that states
// the mechanism cannot be mistaken for a curve by someone filling in a Curve
// field from autocomplete, and it is what Flutter calls this, which is the
// vocabulary the rest of the package already borrows. So Spring stays the
// curve, and everything that integrates is a SpringSimulation.
//
// The reason to reach for it is velocity handoff. A gesture released mid-flight
// has a measured speed, and a fixed-duration tween cannot accept it: the
// animation restarts from rest, so a flung card stops dead the instant the
// finger lifts and then sets off again. That discontinuity is most of what
// separates a native-feeling interface from a web-feeling one. Start takes the
// speed the gesture was travelling at and the first frame continues at it.
//
//	s.spring = &anim.SpringSimulation{
//	    Spec:     anim.SpringWith(320*time.Millisecond, 0.8),
//	    Target:   0,
//	    OnChange: func() { s.SetState(nil) },
//	}
//	ctx.AddTicker(s.spring)                  // in Init; RemoveTicker in Dispose
//	s.spring.Start(s.offset, releaseVelocity) // on release
//
// Target is read on every tick, so assigning it mid-flight retargets the
// spring without interrupting it — the velocity carries straight over, which
// is the right behaviour for a destination that moves (a sheet whose detent
// changes under the finger). Retargeting a settled spring needs it woken:
// Start(s.Value(), s.Velocity()).
//
// Not safe for concurrent use (UI goroutine only).
type SpringSimulation struct {
	// Spec is the spring itself. Its zero value is a usable one.
	Spec SpringSpec
	// Target is where the spring is pulling to, in the value's own units. It
	// is read on every tick, so it may be moved mid-flight.
	Target float32
	// Rest is how close to Target counts as arrived, in the value's own
	// units. 0 → 1e-3, which is sub-pixel for a 0..1 driver; a spring over
	// pixels wants something like 0.4.
	Rest float32
	// RestVelocity is how slow counts as stopped, in units per second.
	// 0 → Rest·ωₙ, i.e. covering the rest tolerance in one radian of the
	// spring's own period. Both conditions must hold, or a spring caught
	// crossing its target at speed would be declared settled and the
	// overshoot it exists to produce would be cut off.
	RestVelocity float32
	// MaxStep caps the integration substep, in seconds; a tick longer than
	// this is subdivided. 0 → 1/240s, which keeps ωₙ·h small enough that the
	// integration error over a settle is under a percent for any spring a UI
	// would use.
	//
	// Raise it only to reproduce an integration that was *tuned* at a
	// coarser step, where the integrator's error is part of the measured
	// feel. widget.Scroll's edge bounce does exactly that: it was tuned
	// against native scroll views while taking one unsubdivided step per
	// frame, which runs about 8% short on the first frame of a hard bounce,
	// and quietly integrating it properly would change physics that were
	// measured rather than chosen. Nothing else should want this.
	//
	// It is a ceiling, not the step: the step is also held under the
	// oscillator's and the damping term's own stability limits, so a stiff
	// spring subdivides further however this is set.
	MaxStep float32
	// OnChange is called after every tick that advanced the simulation,
	// including the one that settles it.
	OnChange func()

	x, v   float32
	active bool
}

// defaultMaxStep is the integrator's substep ceiling when MaxStep is unset.
//
// Semi-implicit (symplectic) Euler is the integrator because it is cheap and
// because it does not pump energy into an oscillator the way explicit Euler
// does — but it is only conditionally stable, and a spring stepped past
// ωₙ·h ≈ 2 diverges instead of settling. A dropped frame, a slow layout pass,
// or a window the compositor stalled all hand a tick a dt far larger than a
// frame, so the step is subdivided rather than taken whole.
//
// 1/240s rather than a frame time because stability is the floor, not the
// goal. One step per frame is stable for an ordinary UI spring and still runs
// visibly short: at ωₙ·h ≈ 0.25 the first frame after a fast handoff covers
// about two thirds of the ground it should, which is precisely the stall the
// handoff exists to remove. Four substeps bring that inside a few percent and
// cost four multiply-adds.
const defaultMaxStep = 1.0 / 240

// maxSubsteps bounds the subdivision. Beyond it the tick is not a dropped
// frame, it is a backgrounded tab or a suspended machine, and replaying
// seconds of simulation to discover the spring has long since landed is
// wasted work with a visible hitch at the end of it. The spring is snapped to
// rest instead, which is where it would have arrived.
const maxSubsteps = 512

// Start places the spring at from and sets it moving at velocity, in units per
// second, then activates it. This is the velocity handoff: pass the speed the
// gesture was measured at and the motion continues through the release instant
// without a step in velocity.
//
// A velocity of 0 is a legitimate request — a spring starting from rest — so
// Start always activates, even when the spring is already at Target. The first
// tick then settles it and reports it done.
func (s *SpringSimulation) Start(from, velocity float32) {
	s.x, s.v, s.active = from, velocity, true
}

// Value returns the current position.
func (s *SpringSimulation) Value() float32 { return s.x }

// Velocity returns the current speed in units per second. A gesture that grabs
// a spring in flight reads this before Stop to hand the motion back.
func (s *SpringSimulation) Velocity() float32 { return s.v }

// Running reports whether the simulation is still moving. The frame loop reads
// it through the ticker registry to decide whether to ask for another frame.
func (s *SpringSimulation) Running() bool { return s.active }

// Stop halts the simulation where it is, without calling OnChange. The
// position stays put, so a gesture that interrupts a spring continues from
// what is on screen; the velocity is dropped, because a stopped spring that
// still claimed a speed would hand a stale one to whatever read it next.
func (s *SpringSimulation) Stop() { s.v, s.active = 0, false }

// Tick advances the simulation by dt seconds and reports whether it is still
// running. It implements the app's ticker contract.
func (s *SpringSimulation) Tick(dt float64) bool {
	// NaN is checked, not assumed away. A single NaN dt poisons x and v, and a
	// NaN can never satisfy the settling test, so the simulation would report
	// itself running for the life of the widget and hold the frame loop awake
	// with nothing moving — the failure Owner.TickerCount exists to catch,
	// arriving from a clock rather than from a missing RemoveTicker.
	if !s.active || dt <= 0 || math.IsNaN(dt) {
		return s.active
	}
	k, c, m := s.Spec.resolve()
	omega := float32(math.Sqrt(float64(k / m)))

	// The stable step is the smallest of the MaxStep ceiling, the oscillator's
	// own limit, and the damping term's: a heavily overdamped spring is limited
	// by c/m rather than by ωₙ, and stepping it past h = m/c reverses the
	// velocity each step instead of draining it.
	step := s.MaxStep
	if step <= 0 {
		step = defaultMaxStep
	}
	if lim := 1 / omega; lim < step {
		step = lim
	}
	if lim := m / c; lim < step {
		step = lim
	}
	n := int(math.Ceil(dt / float64(step)))
	if n < 1 {
		n = 1
	}
	if n > maxSubsteps {
		s.x, s.v, s.active = s.Target, 0, false
		if s.OnChange != nil {
			s.OnChange()
		}
		return false
	}

	rest, restV := s.Rest, s.RestVelocity
	if rest <= 0 {
		rest = 1e-3
	}
	if restV <= 0 {
		restV = rest * omega
	}

	h := float32(dt / float64(n))
	for i := 0; i < n; i++ {
		// Semi-implicit Euler: the new velocity drives the position step.
		s.v += (-k*(s.x-s.Target) - c*s.v) / m * h
		s.x += s.v * h
		d := s.x - s.Target
		if d < 0 {
			d = -d
		}
		av := s.v
		if av < 0 {
			av = -av
		}
		if d < rest && av < restV {
			// Snap the last fraction of a unit away rather than leaving a
			// residue: a spring that stops 0.3px from its target leaves the
			// thing it was moving permanently that far out of place.
			s.x, s.v, s.active = s.Target, 0, false
			break
		}
	}
	if s.OnChange != nil {
		s.OnChange()
	}
	return s.active
}
