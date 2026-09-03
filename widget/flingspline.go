package widget

import "math"

// Android's fling, as android.widget.OverScroller computes it.
//
// The model is a fixed position-vs-time spline stretched to a duration and a
// distance that both grow with the logarithm of the release velocity. It is
// reproduced here from the platform source rather than approximated with an
// exponential, because the harness's point is that the two are not the same
// curve: the spline front-loads its travel and stops dead, where an
// exponential tails off. Every constant below is Android's.
//
// The physical coefficient is expressed in density-independent pixels, which
// is what gophics's logical pixels are on Android. In the platform source it
// carries the display's ppi; dividing both velocity and distance by density
// cancels it, leaving 160ppi — so a fling here travels the same number of
// logical pixels as the platform's would on any screen.

const (
	splineDecelerationRate = 2.3582018 // ln(0.78) / ln(0.9)
	splineInflexion        = 0.35
	splineStartTension     = 0.5
	splineEndTension       = 1.0
	splineSamples          = 100
	splinePhysicalCoeff    = 9.80665 * 39.37 * 160 * 0.84 // gravity · in/m · dpi · tuning
)

// splinePosition is OverScroller's SPLINE_POSITION table: normalized position
// (0..1) at each hundredth of the fling's duration, solved once at init the
// way the platform does.
var splinePosition = func() [splineSamples + 1]float64 {
	var pos [splineSamples + 1]float64
	p1 := splineStartTension * splineInflexion
	p2 := 1.0 - splineEndTension*(1.0-splineInflexion)
	xMin := 0.0
	for i := 0; i < splineSamples; i++ {
		alpha := float64(i) / splineSamples
		xMax := 1.0
		var x, coef float64
		for {
			x = xMin + (xMax-xMin)/2
			coef = 3 * x * (1 - x)
			tx := coef*((1-x)*p1+x*p2) + x*x*x
			if math.Abs(tx-alpha) < 1e-5 {
				break
			}
			if tx > alpha {
				xMax = x
			} else {
				xMin = x
			}
		}
		pos[i] = coef*((1-x)*splineStartTension+x) + x*x*x
	}
	pos[splineSamples] = 1
	return pos
}()

// splineFling is the duration (seconds) and distance (logical px) of a fling
// released at speed v (px/s, sign ignored) under the given friction.
func splineFling(v, friction float64) (dur, dist float64) {
	v = math.Abs(v)
	if v == 0 {
		return 0, 0
	}
	decel := math.Log(splineInflexion * v / (friction * splinePhysicalCoeff))
	dur = math.Exp(decel / (splineDecelerationRate - 1))
	dist = friction * splinePhysicalCoeff * math.Exp(splineDecelerationRate/(splineDecelerationRate-1)*decel)
	return dur, dist
}

// splineAt is the fraction of the total distance covered at fraction t of the
// duration, by the platform's linear interpolation of its table.
func splineAt(t float64) float64 {
	if t <= 0 {
		return 0
	}
	if t >= 1 {
		return 1
	}
	i := int(t * splineSamples)
	frac := t*splineSamples - float64(i)
	return splinePosition[i] + frac*(splinePosition[i+1]-splinePosition[i])
}
