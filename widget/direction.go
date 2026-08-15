package widget

// Reading direction. A UI in Arabic, Hebrew, Persian or Urdu does not just
// render its text right-to-left — the whole layout mirrors: a back arrow
// points right, a row of controls runs from the right edge, and the padding
// that hugged the left now hugs the right.
//
// gophics handles this the way Flutter does, and for the same reason: the
// direction is provided to a subtree, and widgets opt into mirroring through
// *directional* fields (Padding.Start/End, Align.Directional) rather than
// having Left and Right silently change meaning. Code that means "the left
// edge of the screen" — a media scrubber, a chart axis, a number line — keeps
// saying Left and keeps getting left.

// Direction is a subtree's reading direction.
type Direction uint8

const (
	// DirLTR reads left to right. It is the zero value, so a UI that never
	// mentions direction behaves exactly as it did before.
	DirLTR Direction = iota
	// DirRTL reads right to left.
	DirRTL
)

// RTL reports whether d is right-to-left.
func (d Direction) RTL() bool { return d == DirRTL }

// Directionality provides a reading direction to its subtree. Install it above
// the app (usually from the user's locale) and every directional widget below
// mirrors:
//
//	widget.Directionality{Dir: widget.DirRTL, Child: app}
type Directionality struct {
	Dir   Direction
	Child Widget
}

func (d Directionality) Build(Ctx) Widget {
	return Provide[Direction]{Value: d.Dir, Child: d.Child}
}

// DirectionOf returns the reading direction in scope, defaulting to DirLTR
// when no Directionality was installed.
func DirectionOf(c Ctx) Direction {
	d, _ := Of[Direction](c)
	return d
}
