// Package anim provides animation primitives: curves and controllers.
// It is Flutter's animation/ analog (PLAN.md M4), driven by the app's frame
// tickers — a Controller registered with widget.Ctx.AddTicker advances every
// frame while running and stops consuming work when idle.
package anim

import "time"

// Curve maps linear progress t in [0,1] to eased progress.
type Curve func(t float32) float32

func Linear(t float32) float32 { return t }

func EaseIn(t float32) float32 { return t * t * t }

func EaseOut(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

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

// Forward animates toward 1.
func (c *Controller) Forward() {
	if c.progress < 1 {
		c.dir = 1
	}
}

// Reverse animates toward 0.
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

// Lerp linearly interpolates between a and b.
func Lerp(a, b, t float32) float32 { return a + (b-a)*t }

func clamp01(v float32) float32 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}
