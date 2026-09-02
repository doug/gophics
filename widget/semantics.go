package widget

import (
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/layout"
	"github.com/doug/gophics/paint"
)

// Semantics overrides or supplies the semantic description of its subtree
// (label decorative graphics, hide ornaments, group controls). Zero-value
// fields defer to derived semantics.
type Semantics struct {
	Role   layout.Role
	Label  string
	Hidden bool
	// Value is the node's current content or setting as text — the slider's
	// number, the field's contents, the progress percentage.
	Value string
	// Hint describes what activating the node does ("opens the thread").
	Hint string
	// Checked marks a toggleable node's state; nil means not toggleable.
	Checked *bool
	// Expanded marks an expandable node's state; nil means not expandable. A
	// leaf in a tree is not a collapsed branch, which is why this is a pointer
	// rather than a bool.
	Expanded *bool
	// Disabled and Selected mirror the ARIA states of the same name.
	Disabled bool
	Selected bool
	// OnActivate is invoked when assistive technology activates the node — a
	// VoiceOver double-tap or a TalkBack activate. Set it to the same action
	// the control's tap handler runs, so a control that is drawn rather than
	// composed from Interactive is still operable without sight.
	OnActivate func()
	Child      Widget
}

func (sw Semantics) createBox(Ctx) layout.Box { return &semBox{} }
func (sw Semantics) updateBox(_ Ctx, b layout.Box) {
	sb := b.(*semBox)
	sb.info = layout.SemInfo{
		Role: sw.Role, Label: sw.Label, Hidden: sw.Hidden,
		Value: sw.Value, Hint: sw.Hint, Checked: sw.Checked,
		Expanded: sw.Expanded,
		Disabled: sw.Disabled, Selected: sw.Selected,
		OnActivate: sw.OnActivate,
	}
	if sb.info.Role == layout.RoleNone && (sw.Label != "" || sw.Hidden) {
		sb.info.Role = layout.RoleGroup
	}
}
func (sw Semantics) childWidgets() []Widget { return []Widget{sw.Child} }
func (sw Semantics) soleChild() Widget      { return sw.Child }
func (sw Semantics) attach(b layout.Box, kids []layout.Box) {
	b.(*semBox).Child = first(kids)
}

type semBox struct {
	info  layout.SemInfo
	Child layout.Box
	size  geom.Size
}

func (b *semBox) Layout(cs layout.Constraints) geom.Size {
	if b.Child != nil {
		b.size = b.Child.Layout(cs)
	} else {
		b.size = cs.Constrain(geom.Size{})
	}
	return b.size
}

func (b *semBox) Size() geom.Size { return b.size }

func (b *semBox) Paint(c paint.Canvas, at geom.Pt) {
	if b.Child != nil {
		b.Child.Paint(c, at)
	}
}

func (b *semBox) AddHits(p geom.Pt, hits *[]layout.Hit) {
	if b.Child != nil && p.X >= 0 && p.Y >= 0 && p.X < b.size.W && p.Y < b.size.H {
		b.Child.AddHits(p, hits)
	}
}

func (b *semBox) Semantics() layout.SemInfo { return b.info }

func (b *semBox) VisitChildren(visit func(layout.Box, geom.Pt)) {
	if b.Child != nil {
		visit(b.Child, geom.Pt{})
	}
}
