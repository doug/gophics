// Package anim provides animation primitives: curves and controllers.
// It is Flutter's animation/ analog (PLAN.md M4), driven by the app's frame
// tickers — a Controller registered with widget.Ctx.AddTicker advances every
// frame while running and stops consuming work when idle.
package anim

import (
	"math"
	"time"
)

// Curve maps linear progress t in [0,1] to eased progress. A curve must satisfy
// f(0)=0 and f(1)=1 (start and end pinned); only the shape in between differs.
type Curve func(t float32) float32

// Linear returns t unchanged — constant speed, no easing.
func Linear(t float32) float32 { return t }

// EaseIn accelerates from rest: a slow start ramping to full speed (cubic).
func EaseIn(t float32) float32 { return t * t * t }

// EaseOut decelerates to rest: full speed settling into a soft stop (cubic) —
// the usual choice for UI that enters and stays.
func EaseOut(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

// EaseInOut accelerates then decelerates: a symmetric cubic S-curve, and the
// Controller default.
func EaseInOut(t float32) float32 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - u*u*u/2
}

// Controller drives a value from 0 to 1 over Duration, through Curve.
// Zero value: 200ms, EaseInOut. Not safe for concurrent use (UI goroutine
// only). Register it as a ticker and repaint from OnChange:
//
//	s.ctrl = &anim.Controller{OnChange: func() { s.SetState(nil) }}
//	ctx.AddTicker(s.ctrl)     // in Init; RemoveTicker in Dispose
//	s.ctrl.Forward()          // on hover, tap, ...
type Controller struct {
	Duration time.Duration
	Curve    Curve
	// OnChange is called every frame the value changes, and once more when
	// the run completes.
	OnChange func()

	progress float32 // linear, 0..1
	dir      float32 // +1 forward, -1 reverse, 0 idle
}

// Spring is the step response of an underdamped spring: it rises to 1,
// overshoots by a few percent, and settles back.
//
// It is for motion that comes to rest against something — a card springing
// back to its place, a sheet settling into position. An ease-out arrives at
// rest by slowing to a stop, which reads as dead weight; a spring arrives by
// going slightly too far and returning, which reads as elastic. That small
// overshoot is most of what people mean when they say a gesture feels native.
//
// Do not use it for motion that leaves — a card being dismissed should not
// come back a few pixels before it goes. EaseOut is right there.
//
// The damping ratio is 0.72, giving about 4% overshoot, and the frequency is
// chosen so it has settled by the end of the duration.
func Spring(t float32) float32 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1 // exactly at rest at the end, whatever the tail is doing
	}
	const zeta = 0.72 // < 1 overshoots; at 1 it creeps in without any spring
	const omega = 7.0 // in units of the whole duration
	wd := omega * float32(math.Sqrt(1-zeta*zeta))
	decay := float32(math.Exp(-zeta * omega * float64(t)))
	return 1 - decay*(float32(math.Cos(float64(wd*t)))+
		(zeta*omega/wd)*float32(math.Sin(float64(wd*t))))
}

func (c *Controller) duration() float32 {
	if c.Duration <= 0 {
		return 0.2
	}
	return float32(c.Duration.Seconds())
}

// Value returns the current curved progress in [0,1].
func (c *Controller) Value() float32 {
	if c.Curve == nil {
		return EaseInOut(c.progress)
	}
	return c.Curve(c.progress)
}

// Running reports whether the controller is animating.
func (c *Controller) Running() bool { return c.dir != 0 }

// Forward animates toward 1. It only sets direction — unlike Jump it does not
// call OnChange, so if the frame loop is idle the caller must kick it (the same
// SetState/Invalidate that reacts to the gesture is enough). Once a frame runs,
// Tick self-sustains the animation until it completes.
func (c *Controller) Forward() {
	if c.progress < 1 {
		c.dir = 1
	}
}

// Reverse animates toward 0. Like Forward it only sets direction; see Forward
// for the frame-loop note.
func (c *Controller) Reverse() {
	if c.progress > 0 {
		c.dir = -1
	}
}

// Toggle animates toward the far end.
func (c *Controller) Toggle() {
	if c.dir == 1 || (c.dir == 0 && c.progress >= 1) {
		c.Reverse()
	} else {
		c.Forward()
	}
}

// Jump sets the value immediately, without animating.
func (c *Controller) Jump(v float32) {
	c.progress = clamp01(v)
	c.dir = 0
	if c.OnChange != nil {
		c.OnChange()
	}
}

// Tick advances the controller; it implements the app's ticker contract and
// reports whether the controller is still running.
func (c *Controller) Tick(dt float64) bool {
	if c.dir == 0 {
		return false
	}
	c.progress = clamp01(c.progress + c.dir*float32(dt)/c.duration())
	if c.progress <= 0 || c.progress >= 1 {
		c.dir = 0
	}
	if c.OnChange != nil {
		c.OnChange()
	}
	return c.dir != 0
}

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
