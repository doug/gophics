package widget

import (
	"time"

	"github.com/doug/gophics/anim"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Animated tweens toward Value whenever it changes, rebuilding Build with
// the interpolated value each frame — gophics's implicit animation
// (Flutter's AnimatedContainer/AnimatedFoo, generalized). No controller to
// manage: set a new Value and it animates from wherever it currently is.
//
//	widget.Animated[paint.Color]{
//	    Value: bg, Duration: 150 * time.Millisecond, Lerp: paint.Lerp,
//	    Build: func(c paint.Color) widget.Widget {
//	        return widget.Decorated{Color: c, Child: child}
//	    },
//	}
//
// T must be comparable (to detect changes); Lerp interpolates it. The
// typed helpers below (AnimatedColor, AnimatedInsets, ...) prewire common
// cases.
type Animated[T comparable] struct {
	Value    T
	Duration time.Duration
	Curve    anim.Curve
	Lerp     func(a, b T, t float32) T
	Build    func(v T) Widget
}

func (a Animated[T]) CreateState() State { return &animatedState[T]{} }

type animatedState[T comparable] struct {
	StateBase[Animated[T]]
	ctx      Ctx
	ctrl     *anim.Controller
	from, to T
}

func (s *animatedState[T]) Init(ctx Ctx) {
	s.ctx = ctx
	s.ctrl = &anim.Controller{OnChange: func() { s.SetState(nil) }}
	ctx.AddTicker(s.ctrl)
	s.from, s.to = s.W().Value, s.W().Value
	s.ctrl.Jump(1) // start settled at the initial value
}

func (s *animatedState[T]) Dispose() { s.ctx.RemoveTicker(s.ctrl) }

func (s *animatedState[T]) current() T {
	return s.W().Lerp(s.from, s.to, s.ctrl.Value())
}

func (s *animatedState[T]) Build(Ctx) Widget {
	w := s.W()
	s.ctrl.Duration = w.Duration
	s.ctrl.Curve = w.Curve
	if w.Value != s.to {
		s.from = s.current() // continue smoothly from the current point
		s.to = w.Value
		s.ctrl.Jump(0)
		s.ctrl.Forward()
	}
	return w.Build(s.current())
}

// AnimateColor, AnimateInsets, and AnimateFloat are ergonomic constructors
// for the common cases — they prewire Lerp so the call site is just value +
// builder. Duration 0 means 150ms.
//
//	widget.AnimateColor(bg, 0, func(c paint.Color) widget.Widget {
//	    return widget.Decorated{Color: c, Radius: 8, Child: child}
//	})
func AnimateColor(value paint.Color, d time.Duration, build func(paint.Color) Widget) Widget {
	return Animated[paint.Color]{Value: value, Duration: orMS(d, 150), Lerp: paint.Lerp, Build: build}
}

// AnimateInsets tweens padding.
func AnimateInsets(value geom.Insets, d time.Duration, build func(geom.Insets) Widget) Widget {
	return Animated[geom.Insets]{Value: value, Duration: orMS(d, 150),
		Lerp: func(a, b geom.Insets, t float32) geom.Insets { return a.Lerp(b, t) }, Build: build}
}

// AnimateFloat tweens a scalar (a size, angle, or opacity driver).
func AnimateFloat(value float32, d time.Duration, build func(float32) Widget) Widget {
	return Animated[float32]{Value: value, Duration: orMS(d, 150), Lerp: geom.LerpFloat, Build: build}
}

// AnimatedScale smoothly scales child toward scale (about its center) whenever
// scale changes — the tap-to-grow / pop affordance. Duration 0 means 150ms.
//
// The interpolated scale's magnitude is clamped to a small epsilon (1e-3):
// paint.Transform treats SX==0 as "unset" (identity), so a tween to or through
// exactly 0 would flash the child at full size for a frame. Animating toward 0
// therefore settles at a visually-invisible near-zero scale rather than
// exactly 0 (negative scales keep their sign, so mirror tweens still work).
func AnimatedScale(scale float32, d time.Duration, child Widget) Widget {
	return AnimateFloat(scale, d, func(s float32) Widget {
		const eps = 1e-3
		if s >= 0 && s < eps {
			s = eps
		} else if s < 0 && s > -eps {
			s = -eps
		}
		return Transform{T: paint.Transform{SX: s, SY: s}, Center: true, Child: child}
	})
}

// AnimatedRotation smoothly rotates child toward radians (about its center).
func AnimatedRotation(radians float32, d time.Duration, child Widget) Widget {
	return AnimateFloat(radians, d, func(r float32) Widget {
		return Transform{T: paint.Transform{Rotation: r}, Center: true, Child: child}
	})
}

func orMS(d time.Duration, ms int) time.Duration {
	if d <= 0 {
		return time.Duration(ms) * time.Millisecond
	}
	return d
}
