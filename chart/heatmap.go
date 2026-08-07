package chart

import (
	"math"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// ColorScale maps a value in [Lo, Hi] to a color along From→To. Unset From/To
// default to a light→deep green ramp (contribution-graph style).
type ColorScale struct {
	Lo, Hi   float64
	From, To paint.Color
}

var (
	heatFrom = paint.RGB(0.90, 0.93, 0.90)
	heatTo   = paint.RGB(0.13, 0.55, 0.30)
)

func (s ColorScale) at(v float64) paint.Color {
	from, to := s.From, s.To
	if from.A == 0 && to.A == 0 {
		from, to = heatFrom, heatTo
	}
	hi := s.Hi
	if hi <= s.Lo {
		hi = s.Lo + 1
	}
	t := (v - s.Lo) / (hi - s.Lo)
	return paint.Lerp(from, to, float32(math.Max(0, math.Min(1, t))))
}

// Cell is one heatmap tile at grid position (X column, Y row) with value V.
type Cell struct {
	X, Y int
	V    float64
}

// RectMark draws a grid heatmap: one rounded tile per cell, colored by V through
// Scale. It lays out its own square grid within the plot area (hide the axes).
type RectMark struct {
	Cells      []Cell
	Cols, Rows int // 0 → inferred from the cells
	Scale      ColorScale
	Gap        float32 // px between tiles; 0 → 3
	Round      float32 // tile corner radius; 0 → 2
}

func (RectMark) xDomain() (lo, hi float64, c []string) { return math.Inf(1), math.Inf(-1), nil }
func (RectMark) yDomain() (lo, hi float64)             { return math.Inf(1), math.Inf(-1) }

func (m RectMark) draw(p plot) {
	if len(m.Cells) == 0 {
		return
	}
	cols, rows := m.Cols, m.Rows
	if cols == 0 || rows == 0 {
		for _, c := range m.Cells {
			cols = max(cols, c.X+1)
			rows = max(rows, c.Y+1)
		}
	}
	gap := m.Gap
	if gap == 0 {
		gap = 3
	}
	round := m.Round
	if round == 0 {
		round = 2
	}
	a := p.Area
	cell := min((a.Dx()-gap*float32(cols+1))/float32(cols), (a.Dy()-gap*float32(rows+1))/float32(rows))
	if cell <= 0 {
		return
	}
	step := cell + gap
	// Center the grid in the plot area.
	ox := a.Min.X + (a.Dx()-(float32(cols)*step-gap))/2
	oy := a.Min.Y + (a.Dy()-(float32(rows)*step-gap))/2
	for _, c := range m.Cells {
		reveal := clamp(p.T*float32(cols)-float32(c.X), 0, 1) // sweep in by column
		if reveal <= 0 {
			continue
		}
		x := ox + float32(c.X)*step
		y := oy + float32(c.Y)*step
		p.Canvas.FillRRect(geom.RectXYWH(x, y, cell, cell), round, m.Scale.at(c.V).WithAlpha(reveal))
	}
}
