package layoutbox

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
)

// VisitChildren implementations for the built-in containers.

func (b *Padded) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, b.childOffset())
	}
}

func (b *Aligned) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, b.offset)
	}
}

func (b *Sized) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (b *Decorated) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}

func (f *Flex) VisitChildren(visit func(layout.Box, geom.Pt)) {
	for i, c := range f.Children {
		if i >= len(f.offsets) {
			break
		}
		visit(c.Box, f.offsets[i])
	}
}

func (v *Viewport) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if v.Child != nil {
		visit(v.Child, v.scrollPt())
	}
}

// Semantics implements Semantic for text boxes.
func (b *TextBox) Semantics() layout.SemInfo {
	return layout.SemInfo{Role: layout.RoleText, Label: b.Text}
}
