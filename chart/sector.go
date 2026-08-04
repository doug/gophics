package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// SectorMark draws a pie (Inner == 0) or donut (Inner > 0) of the plot area:
// one wedge per datum, sized by its Y as a fraction of the total. It ignores
// the axes, so hide them. Built on paint.FillPath.
type SectorMark struct {
	Data   []Datum
	Inner  float32       // donut hole radius as a fraction of the outer radius
	Colors []paint.Color // optional per-slice overrides; else the series palette
}

func (SectorMark) xDomain() (lo, hi float64, c []string) { return math.Inf(1), math.Inf(-1), nil }
func (SectorMark) yDomain() (lo, hi float64)             { return math.Inf(1), math.Inf(-1) }

func (s SectorMark) sliceColor(i int, th theme) paint.Color {
	if i < len(s.Colors) && s.Colors[i].A > 0 {
		return s.Colors[i]
	}
	return th.series[i%len(th.series)]
}

func (s SectorMark) draw(p Plot) {
	var total float64
	for _, d := range s.Data {
		total += d.Y
	}
	if total <= 0 {
		return
	}
	a := p.Area
	cx, cy := a.Min.X+a.Dx()/2, a.Min.Y+a.Dy()/2
	r := min(a.Dx(), a.Dy())/2 - 4
	if r <= 0 {
		return
	}
	inner := r * clamp(s.Inner, 0, 0.9)
	ang := -math.Pi / 2 // start at 12 o'clock
	for i, d := range s.Data {
		frac := d.Y / total
		sweep := frac * 2 * math.Pi * float64(p.T)
		p.Canvas.FillPath(wedge(cx, cy, r, inner, ang, ang+sweep), s.sliceColor(i, p.th))
		ang += frac * 2 * math.Pi
	}
}

// legendEntries makes each slice its own legend row (label + slice color).
func (s SectorMark) legendEntries(th theme) []legendEntry {
	out := make([]legendEntry, 0, len(s.Data))
	for i, d := range s.Data {
		label := d.Label
		if label == "" {
			label = fmtNumber(d.Y)
		}
		out = append(out, legendEntry{label, s.sliceColor(i, th)})
	}
	return out
}

// wedge builds a filled wedge (or annular wedge when inner > 0) from angle a0 to
// a1, approximating the arcs with short line segments.
func wedge(cx, cy, r, inner float32, a0, a1 float64) *paint.Path {
	p := paint.NewPath()
	seg := int(math.Abs(a1-a0)/(math.Pi/48)) + 1
	at := func(rad float32, t float64) geom.Pt {
		return geom.Pt{X: cx + rad*float32(math.Cos(t)), Y: cy + rad*float32(math.Sin(t))}
	}
	for i := 0; i <= seg; i++ {
		t := a0 + (a1-a0)*float64(i)/float64(seg)
		if i == 0 {
			p.MoveTo(at(r, t))
		} else {
			p.LineTo(at(r, t))
		}
	}
	if inner <= 0 {
		p.LineTo(geom.Pt{X: cx, Y: cy})
	} else {
		for i := 0; i <= seg; i++ {
			t := a1 + (a0-a1)*float64(i)/float64(seg)
			p.LineTo(at(inner, t))
		}
	}
	return p.Close()
}
