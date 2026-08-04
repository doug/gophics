package paint

import "github.com/doug/gophics/geom"

// pathVerb is a single path command.
type pathVerb uint8

const (
	verbMove pathVerb = iota
	verbLine
	verbClose
)

// Path is a retained 2-D path built from move/line/close commands and filled
// with Canvas.FillPath. It is always passed by pointer: the scene recorder
// compares it by identity + Gen() rather than element-by-element, so a path
// stored across frames diffs in O(1) and a rebuilt or mutated one repaints.
//
// Fills use the non-zero winding rule. Subpaths that aren't explicitly Closed
// are implicitly closed when filled.
type Path struct {
	verbs   []pathVerb
	pts     []geom.Pt
	gen     uint64
	min     geom.Pt
	max     geom.Pt
	started bool
}

// NewPath returns an empty path.
func NewPath() *Path { return &Path{} }

// MoveTo starts a new subpath at pt.
func (p *Path) MoveTo(pt geom.Pt) *Path {
	p.verbs = append(p.verbs, verbMove)
	p.pts = append(p.pts, pt)
	p.grow(pt)
	p.gen++
	return p
}

// LineTo adds a segment to pt (starting a subpath if none is open).
func (p *Path) LineTo(pt geom.Pt) *Path {
	if len(p.verbs) == 0 {
		return p.MoveTo(pt)
	}
	p.verbs = append(p.verbs, verbLine)
	p.pts = append(p.pts, pt)
	p.grow(pt)
	p.gen++
	return p
}

// Close closes the current subpath back to its start.
func (p *Path) Close() *Path {
	p.verbs = append(p.verbs, verbClose)
	p.gen++
	return p
}

// Reset clears the path for reuse (keeping capacity) and bumps the generation.
func (p *Path) Reset() *Path {
	p.verbs = p.verbs[:0]
	p.pts = p.pts[:0]
	p.started = false
	p.gen++
	return p
}

// Empty reports whether the path has no commands.
func (p *Path) Empty() bool { return len(p.verbs) == 0 }

// Gen is a counter bumped on every mutation; the scene op captures it so an
// in-place-mutated path (same pointer) still diffs as changed.
func (p *Path) Gen() uint64 { return p.gen }

// Bounds is the axis-aligned bounding box of the path's points.
func (p *Path) Bounds() geom.Rect {
	if !p.started {
		return geom.Rect{}
	}
	return geom.Rect{Min: p.min, Max: p.max}
}

func (p *Path) grow(pt geom.Pt) {
	if !p.started {
		p.min, p.max, p.started = pt, pt, true
		return
	}
	if pt.X < p.min.X {
		p.min.X = pt.X
	}
	if pt.Y < p.min.Y {
		p.min.Y = pt.Y
	}
	if pt.X > p.max.X {
		p.max.X = pt.X
	}
	if pt.Y > p.max.Y {
		p.max.Y = pt.Y
	}
}
