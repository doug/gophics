package layoutbox

import (
	"fmt"
	"github.com/doug/gophics/layout"
	"reflect"
	"strings"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
)

// Walk visits every box in the tree with its rect in root coordinates,
// depth-first (a box before its children). Boxes expose their children via
// layout.ChildVisitor; leaves are visited without recursing. This powers the
// debug-paint overlay and the tree inspector.
func Walk(root layout.Box, visit func(b layout.Box, rect geom.Rect, depth int)) {
	var rec func(b layout.Box, at geom.Pt, depth int)
	rec = func(b layout.Box, at geom.Pt, depth int) {
		visit(b, geom.Rect{Min: at, Max: at.Add(b.Size().Pt())}, depth)
		if v, ok := b.(layout.ChildVisitor); ok {
			v.VisitChildren(func(child layout.Box, off geom.Pt) {
				rec(child, at.Add(off), depth+1)
			})
		}
	}
	rec(root, geom.Pt{}, 0)
}

// InspectNode is one entry in a flattened render-tree dump.
type InspectNode struct {
	Type  string    // box type, e.g. "*layout.Flex"
	Rect  geom.Rect // in root coordinates
	Depth int
	Role  layout.Role // semantic role, if the box contributes one
	Label string      // semantic label, if any
}

// Inspect returns the render tree as a flat, depth-ordered slice — the data
// behind a widget inspector. Runs headless; no display needed.
func Inspect(root layout.Box) []InspectNode {
	var out []InspectNode
	Walk(root, func(b layout.Box, rect geom.Rect, depth int) {
		n := InspectNode{Type: boxTypeName(b), Rect: rect, Depth: depth}
		if s, ok := b.(layout.Semantic); ok {
			info := s.Semantics()
			n.Role, n.Label = info.Role, info.Label
		}
		out = append(out, n)
	})
	return out
}

func boxTypeName(b layout.Box) string { return reflect.TypeOf(b).String() }

// DeepestAt returns the smallest-area box whose bounds contain p (root
// coordinates) and its rect — the target an interactive inspector highlights.
func DeepestAt(root layout.Box, p geom.Pt) (layout.Box, geom.Rect, bool) {
	var box layout.Box
	var rect geom.Rect
	best := float32(1e18)
	found := false
	Walk(root, func(b layout.Box, r geom.Rect, _ int) {
		if b.Size().IsEmpty() || !r.Contains(p) {
			return
		}
		if a := r.Dx() * r.Dy(); a <= best {
			box, rect, best, found = b, r, a, true
		}
	})
	return box, rect, found
}

// InspectOverlay draws the interactive inspector: it highlights the deepest
// box under p and labels it with its type and size — Flutter's widget
// inspector, drawn straight onto the frame after content.
func InspectOverlay(root layout.Box, p geom.Pt, c paint.Canvas, painter *paint.Painter) {
	b, rect, ok := DeepestAt(root, p)
	if !ok {
		return
	}
	c.FillRect(rect, paint.Color{R: 0.36, G: 0.62, B: 0.98, A: 0.15})
	c.StrokeRRect(rect, 0, 1, paint.Color{R: 0.36, G: 0.62, B: 0.98, A: 0.9})

	label := fmt.Sprintf("%s  %.0f×%.0f", boxTypeName(b), rect.Dx(), rect.Dy())
	const fs float32 = 11
	chipW := painter.MeasureWidthIn("", label, fs) + 8
	chipH := fs + 6
	y := rect.Min.Y - chipH // above the box, or inside if it would clip off-screen
	if y < 0 {
		y = rect.Min.Y
	}
	chip := geom.RectXYWH(rect.Min.X, y, chipW, chipH)
	c.FillRRect(chip, 3, paint.Color{R: 0.09, G: 0.11, B: 0.15, A: 0.96})
	c.TextIn("", label, geom.Pt{X: chip.Min.X + 4, Y: chip.Min.Y + fs}, fs,
		paint.Color{R: 0.9, G: 0.93, B: 0.96, A: 1})

}

// DebugPaint strokes every box's bounds over the frame (Flutter's
// debugPaintSize). Nested boxes get progressively lighter hues so the
// hierarchy reads at a glance. Draw it after the app content.
func DebugPaint(root layout.Box, c paint.Canvas) {
	Walk(root, func(b layout.Box, rect geom.Rect, depth int) {
		if b.Size().IsEmpty() {
			return
		}
		h := float32(depth%6) / 6
		col := debugHue(h)
		c.StrokeRRect(rect, 0, 1, col)
	})
}

// debugHue maps [0,1) to a saturated color wheel for outline tinting.
func debugHue(h float32) paint.Color {
	r, g, b := hsv(h*360, 0.8, 0.95)
	return paint.Color{R: r, G: g, B: b, A: 0.7}
}

func hsv(h, s, v float32) (r, g, bl float32) {
	c := v * s
	x := c * (1 - absf(mod(h/60, 2)-1))
	m := v - c
	switch {
	case h < 60:
		r, g, bl = c, x, 0
	case h < 120:
		r, g, bl = x, c, 0
	case h < 180:
		r, g, bl = 0, c, x
	case h < 240:
		r, g, bl = 0, x, c
	case h < 300:
		r, g, bl = x, 0, c
	default:
		r, g, bl = c, 0, x
	}
	return r + m, g + m, bl + m
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}
func mod(a, b float32) float32 { return a - b*float32(int(a/b)) }

// String renders an InspectNode as an indented tree line.
func (n InspectNode) String() string {
	var pad strings.Builder
	for i := 0; i < n.Depth; i++ {
		pad.WriteString("  ")
	}
	label := ""
	if n.Label != "" {
		label = fmt.Sprintf(" %q", n.Label)
	}
	return fmt.Sprintf("%s%s [%.0fx%.0f @%.0f,%.0f]%s", pad.String(), n.Type,
		n.Rect.Dx(), n.Rect.Dy(), n.Rect.Min.X, n.Rect.Min.Y, label)
}
