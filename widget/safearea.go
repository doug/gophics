package widget

import (
	"github.com/doug/gophics/geom"
)

// Edge selects the sides of a SafeArea to inset. The zero value means every
// edge, so the common case — keep all of my content clear of the hardware — is
// SafeArea{Child: …} with nothing else to remember.
type Edge uint8

const (
	Top Edge = 1 << iota
	Bottom
	Left
	Right

	// Horizontal and Vertical are the usual pairs.
	Horizontal = Left | Right
	Vertical   = Top | Bottom
	// AllEdges is every side. It equals the zero value's behaviour and exists so
	// a caller can say so explicitly.
	AllEdges = Top | Bottom | Left | Right
)

// SafeArea insets its child by the platform's obstructed edges — the notch and
// Dynamic Island, the status bar, the home indicator, a curved display's corners.
//
// Without it a full-bleed layout paints under the hardware: a header disappears
// behind the Dynamic Island, a bottom bar sits under the home indicator. The
// insets arrive from the shell (Ctx.SafeInsets) and are zero on desktop, so
// wrapping a layout costs nothing on platforms that have no obstructions and does
// the right thing on the ones that do.
//
// A screen usually wants this once, around the root:
//
//	widget.SafeArea{Child: myScreen}
//
// Edges selects which sides to apply; the zero value applies all of them. Leave
// an edge out when the content should deliberately run under it — a background
// image, or a scrolling list whose last row should slide beneath the home
// indicator rather than stopping short of it.
type SafeArea struct {
	Edges Edge
	// Minimum is applied in addition to the platform inset on the selected edges,
	// so a layout can keep its own margin without adding a second Padding.
	Minimum geom.Insets
	Child   Widget
}

func (SafeArea) CreateState() State { return &safeAreaState{} }

type safeAreaState struct {
	StateBase[SafeArea]
}

// safeAreaApplied marks a subtree that has already been inset, so a nested
// SafeArea passes through instead of insetting a second time.
//
// The app runner wraps every root in one of these, because four of five
// examples in this tree forgot to and had their titles under the notch. An app
// that also wraps its own screen — which the documentation above recommends,
// and which was the right advice before the default existed — must not end up
// padded twice.
type safeAreaApplied struct{}

func (s *safeAreaState) Build(ctx Ctx) Widget {
	w := s.W()
	if _, done := Of[safeAreaApplied](ctx); done {
		return w.Child
	}
	return Provide[safeAreaApplied]{Value: safeAreaApplied{}, Child: Padding{
		Insets: resolveSafeInsets(ctx.SafeInsets(), w.Edges, w.Minimum),
		Child:  w.Child,
	}}
}

// resolveSafeInsets decides the padding for a set of platform insets. It is a
// pure function so the edge selection and the floor behaviour can be tested
// exhaustively without mounting a tree.
//
// Minimum is a floor rather than an addition: a layout keeps its own margin where
// the hardware needs none, and the hardware wins where it needs more. Adding the
// two would double-pad a notch.
func resolveSafeInsets(platform geom.Insets, edges Edge, min geom.Insets) geom.Insets {
	if edges == 0 {
		edges = AllEdges
	}
	pick := func(on Edge, p, m float32) float32 {
		if edges&on == 0 {
			return m
		}
		if p > m {
			return p
		}
		return m
	}
	return geom.Insets{
		Top:    pick(Top, platform.Top, min.Top),
		Bottom: pick(Bottom, platform.Bottom, min.Bottom),
		Left:   pick(Left, platform.Left, min.Left),
		Right:  pick(Right, platform.Right, min.Right),
	}
}
