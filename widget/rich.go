package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/internal/layoutbox"
	"github.com/doug/gophics/layout"
)

// Rich displays a wrapped paragraph of styled spans; spans with Link set
// are tappable and report through OnLink.
type Rich struct {
	Spans  []layout.RichSpan
	Size   float32 // 0 → 14
	OnLink func(url string)
}

func (r Rich) size() float32 {
	if r.Size == 0 {
		return 14
	}
	return r.Size
}

func (r Rich) CreateState() State { return &richState{} }

type richState struct {
	StateBase[Rich]
	box       *richRef
	lastPress geom.Pt
}

type richRef struct{ b *layoutbox.RichBox }

func (s *richState) Init(Ctx) { s.box = &richRef{} }

func (s *richState) Build(Ctx) Widget {
	r := s.W()
	if r.OnLink == nil {
		return richView{state: s}
	}
	return Interactive{
		Gestures: Gestures{
			OnPress: func(p geom.Pt) { s.lastPress = p },
			OnTap: func() {
				if s.box.b == nil {
					return
				}
				if url, ok := s.box.b.LinkAt(s.lastPress); ok {
					s.W().OnLink(url)
				}
			},
		},
		Child: richView{state: s},
	}
}

type richView struct{ state *richState }

func (v richView) createBox(ctx Ctx) layout.Box {
	return &layoutbox.RichBox{Painter: ctx.Painter()}
}
func (v richView) updateBox(ctx Ctx, b layout.Box) {
	rb := b.(*layoutbox.RichBox)
	r := v.state.W()
	rb.Painter, rb.Spans, rb.TextSize = ctx.Painter(), r.Spans, r.size()
	v.state.box.b = rb
}
func (v richView) childWidgets() []Widget          { return nil }
func (v richView) attach(layout.Box, []layout.Box) {}
