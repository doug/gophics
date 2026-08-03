package widget

import (
	"strings"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/layout"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/shell"
)

// SelectionArea makes every Text in its subtree selectable as one continuous
// range: drag across titles, paragraphs, and list items to select, then copy
// with Cmd/Ctrl+C. It is framework-level — it paints its own highlight and
// copies to the platform clipboard — so it behaves identically on web,
// terminal, desktop, and mobile (the Flutter SelectionArea model), with no
// dependency on native/DOM text.
//
// Text widgets opt in automatically by reading the area's registry from context;
// no change to the child tree is needed.
type SelectionArea struct {
	Child Widget
	// SelectionColor is the highlight fill; zero alpha → a translucent blue.
	SelectionColor paint.Color
}

func (a SelectionArea) CreateState() State { return &selectionAreaState{} }

type selectionAreaState struct {
	StateBase[SelectionArea]
	ctx Ctx
	reg *selectionRegistry
}

func (s *selectionAreaState) Init(ctx Ctx) {
	s.ctx = ctx
	s.reg = &selectionRegistry{}
}

func (s *selectionAreaState) Build(ctx Ctx) Widget {
	col := s.W().SelectionColor
	if col.A == 0 {
		col = paint.Color{R: 0.36, G: 0.62, B: 0.98, A: 0.35}
	}
	r := s.reg
	r.selColor = col
	return Provide[*selectionRegistry]{Value: r, Child: Interactive{
		Handler: Handler{
			// Modality-aware drag ownership (Flutter parity):
			//   - mouse: a drag that begins on text selects (any direction),
			//     beating a deeper scroll; a drag on empty space scrolls.
			//   - touch: a drag scrolls, UNLESS a long-press already started a
			//     selection — then the drag extends it.
			DragPriority: func(touch bool) bool {
				if touch {
					return r.selecting
				}
				return r.pressedText
			},
			OnPress: func(p geom.Pt) {
				// Record the caret and clear any prior selection; the drag (mouse)
				// or long-press (touch) turns it into a visible selection.
				if pt, ok := r.locate(p); ok {
					s.SetState(func() {
						r.anchor, r.focus, r.has, r.pressedText, r.selecting = pt, pt, true, true, false
					})
				} else {
					s.SetState(func() { r.has, r.pressedText, r.selecting = false, false, false })
				}
			},
			// Long-press starts a selection at the word under the pointer (the
			// touch entry point; on desktop it's a harmless bonus). A subsequent
			// drag then extends it via DragPriority.
			OnLongPress: func() {
				if r.pressedText {
					s.SetState(func() { r.selectWord(); r.has, r.selecting = true, true })
				}
			},
			OnDrag: func(pos, _ geom.Pt) {
				if pt, ok := r.locate(pos); ok {
					s.SetState(func() { r.focus, r.has = pt, true })
				}
			},
			OnKey: func(k shell.Key) {
				if k.Kind == shell.KeyPress && k.Mods.Command() && k.Code == shell.KeyC {
					if txt := r.selectedText(); txt != "" {
						if cb := s.ctx.Clipboard(); cb != nil {
							_ = cb.ClipboardWrite(txt)
						}
					}
				}
			},
		},
		Child: selAnchor{reg: r, child: s.W().Child},
	}}
}

// selPoint is a position in the selection model: a fragment index (in paint
// order) and a linear rune offset within it.
type selPoint struct {
	frag int
	off  int
}

// selFrag is one registered text fragment for the current frame.
type selFrag struct {
	origin    geom.Pt // absolute
	rect      geom.Rect
	lines     []string
	lineStart []int // linear rune offset at each line's start
	linearLen int
	font      string
	size      float32
	lineH     float32
	painter   *paint.Painter
}

// selectionRegistry coordinates a single selection across all the text
// fragments painted inside a SelectionArea. It implements layout.SelectionSink.
type selectionRegistry struct {
	origin        geom.Pt    // the area's absolute origin (pointer coords rebase to it)
	frags         []*selFrag // rebuilt every paint, in paint (reading) order
	anchor, focus selPoint
	has           bool
	pressedText   bool // last press landed on text → drag should select, not scroll
	selecting     bool // a touch long-press has started a selection (drag extends it)
	selColor      paint.Color
}

// selectWord expands the current caret (anchor) to the word around it — the
// touch long-press entry point.
func (r *selectionRegistry) selectWord() {
	if r.anchor.frag < 0 || r.anchor.frag >= len(r.frags) {
		return
	}
	lo, hi := r.frags[r.anchor.frag].wordAt(r.anchor.off)
	r.anchor = selPoint{r.anchor.frag, lo}
	r.focus = selPoint{r.anchor.frag, hi}
}

// beginFrame resets the per-frame fragment list. Called by the anchor box at
// the start of each paint, before descendants register.
func (r *selectionRegistry) beginFrame(origin geom.Pt) {
	r.origin = origin
	r.frags = r.frags[:0]
}

// RegisterText implements layout.SelectionSink.
func (r *selectionRegistry) RegisterText(f layout.TextFragment) (int, int, paint.Color) {
	fr := &selFrag{
		origin: f.Origin, lines: f.Lines, font: f.Font, size: f.Size,
		lineH: f.LineH, painter: f.Painter,
	}
	off := 0
	var maxW float32
	for _, ln := range f.Lines {
		fr.lineStart = append(fr.lineStart, off)
		off += len([]rune(ln)) + 1
		if w := f.Painter.MeasureWidthIn(f.Font, ln, f.Size); w > maxW {
			maxW = w
		}
	}
	if off > 0 {
		off-- // drop the trailing virtual newline
	}
	fr.linearLen = off
	h := f.Ascent + f.Descent + float32(len(f.Lines)-1)*f.LineH
	fr.rect = geom.Rect{Min: f.Origin, Max: geom.Pt{X: f.Origin.X + maxW, Y: f.Origin.Y + h}}

	idx := len(r.frags)
	r.frags = append(r.frags, fr)
	lo, hi := r.rangeFor(idx)
	return lo, hi, r.selColor
}

// ordered returns the selection endpoints in reading order.
func (r *selectionRegistry) ordered() (selPoint, selPoint) {
	a, b := r.anchor, r.focus
	if a.frag > b.frag || (a.frag == b.frag && a.off > b.off) {
		return b, a
	}
	return a, b
}

// rangeFor returns the [lo, hi) linear rune range to highlight in fragment idx.
func (r *selectionRegistry) rangeFor(idx int) (int, int) {
	if !r.has {
		return 0, 0
	}
	a, b := r.ordered()
	if idx < a.frag || idx > b.frag || idx >= len(r.frags) {
		return 0, 0
	}
	lo, hi := 0, r.frags[idx].linearLen
	if idx == a.frag {
		lo = a.off
	}
	if idx == b.frag {
		hi = b.off
	}
	if lo >= hi {
		return 0, 0
	}
	return lo, hi
}

// locate maps an area-local point to a selection position, choosing the
// fragment under the point or, failing that, the vertically nearest one (so a
// drag past the text still extends to the closest run).
func (r *selectionRegistry) locate(local geom.Pt) (selPoint, bool) {
	abs := local.Add(r.origin)
	best := -1
	var bestDist float32 = 1e18
	for i, fr := range r.frags {
		if fr.rect.Contains(abs) {
			best = i
			break
		}
		d := vDist(fr.rect, abs)
		if d < bestDist {
			bestDist, best = d, i
		}
	}
	if best < 0 {
		return selPoint{}, false
	}
	fr := r.frags[best]
	return selPoint{best, fr.offsetAt(geom.Pt{X: abs.X - fr.origin.X, Y: abs.Y - fr.origin.Y})}, true
}

// selectedText returns the selected runes across fragments, lines and
// fragments joined with newlines.
func (r *selectionRegistry) selectedText() string {
	if !r.has {
		return ""
	}
	a, b := r.ordered()
	var parts []string
	for i := a.frag; i <= b.frag && i < len(r.frags); i++ {
		fr := r.frags[i]
		lo, hi := 0, fr.linearLen
		if i == a.frag {
			lo = a.off
		}
		if i == b.frag {
			hi = b.off
		}
		if t := fr.textRange(lo, hi); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, "\n")
}

// offsetAt maps a fragment-local point to a linear rune offset (clamped).
func (fr *selFrag) offsetAt(p geom.Pt) int {
	if len(fr.lines) == 0 {
		return 0
	}
	li := int(p.Y / fr.lineH)
	li = min(max(li, 0), len(fr.lines)-1)
	runes := []rune(fr.lines[li])
	col := len(runes)
	for i := 1; i <= len(runes); i++ {
		w := fr.painter.MeasureWidthIn(fr.font, string(runes[:i]), fr.size)
		prev := fr.painter.MeasureWidthIn(fr.font, string(runes[:i-1]), fr.size)
		if p.X < (prev+w)/2 { // past the glyph midpoint selects the next
			col = i - 1
			break
		}
	}
	return fr.lineStart[li] + col
}

// wordAt returns the linear range of the whitespace-delimited word containing
// the offset (for long-press word selection).
func (fr *selFrag) wordAt(off int) (int, int) {
	for li := len(fr.lines) - 1; li >= 0; li-- {
		start := fr.lineStart[li]
		if off < start {
			continue
		}
		runes := []rune(fr.lines[li])
		col := min(off-start, len(runes))
		word := func(r rune) bool { return r != ' ' && r != '\t' }
		lo, hi := col, col
		for lo > 0 && word(runes[lo-1]) {
			lo--
		}
		for hi < len(runes) && word(runes[hi]) {
			hi++
		}
		return start + lo, start + hi
	}
	return off, off
}

// textRange returns the runes in [lo, hi) across the fragment's lines, joined
// with newlines.
func (fr *selFrag) textRange(lo, hi int) string {
	if lo >= hi {
		return ""
	}
	var out []string
	for li, ln := range fr.lines {
		runes := []rune(ln)
		start := fr.lineStart[li]
		a := max(lo-start, 0)
		z := min(hi-start, len(runes))
		if a < z {
			out = append(out, string(runes[a:z]))
		}
	}
	return strings.Join(out, "\n")
}

// vDist is the vertical distance from a point to a rect (0 if within its band).
func vDist(r geom.Rect, p geom.Pt) float32 {
	switch {
	case p.Y < r.Min.Y:
		return r.Min.Y - p.Y
	case p.Y > r.Max.Y:
		return p.Y - r.Max.Y
	default:
		return 0
	}
}

// selAnchor is a single-child render widget whose box captures the area's
// absolute origin and resets the registry at the start of each paint.
type selAnchor struct {
	reg   *selectionRegistry
	child Widget
}

func (w selAnchor) createBox(Ctx) layout.Box            { return &selAnchorBox{reg: w.reg} }
func (w selAnchor) updateBox(_ Ctx, b layout.Box)       { b.(*selAnchorBox).reg = w.reg }
func (w selAnchor) childWidgets() []Widget              { return []Widget{w.child} }
func (w selAnchor) attach(b layout.Box, k []layout.Box) { b.(*selAnchorBox).child = first(k) }

type selAnchorBox struct {
	layout.Base
	reg   *selectionRegistry
	child layout.Box
}

func (b *selAnchorBox) Layout(cs layout.Constraints) geom.Size {
	var sz geom.Size
	if b.child != nil {
		sz = b.child.Layout(cs)
	} else {
		sz = cs.Constrain(geom.Size{})
	}
	return b.Done(cs, sz)
}

func (b *selAnchorBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.reg != nil {
		b.reg.beginFrame(at)
	}
	if b.child != nil {
		b.child.Paint(c, at)
	}
}

func (b *selAnchorBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.child != nil {
		b.child.AddHits(p, hits)
	}
}

func (b *selAnchorBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.child != nil {
		visit(b.child, geom.Pt{})
	}
}
