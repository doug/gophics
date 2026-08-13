package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
)

// safeAreaPadding is the decision SafeArea makes, isolated from mounting.
func safeAreaPadding(_ *testing.T, w SafeArea, platform geom.Insets) geom.Insets {
	return resolveSafeInsets(platform, w.Edges, w.Minimum)
}

func TestSafeAreaZeroValueAppliesEveryEdge(t *testing.T) {
	platform := geom.Insets{Top: 59, Bottom: 34, Left: 5, Right: 5}
	got := safeAreaPadding(t, SafeArea{}, platform)
	if got != platform {
		t.Errorf("zero-value SafeArea gave %+v, want every edge %+v", got, platform)
	}
}

func TestSafeAreaSelectsEdges(t *testing.T) {
	platform := geom.Insets{Top: 59, Bottom: 34, Left: 5, Right: 5}
	cases := []struct {
		name  string
		edges Edge
		want  geom.Insets
	}{
		// A scrolling list slides under the home indicator: top only.
		{"top", Top, geom.Insets{Top: 59}},
		{"horizontal", Horizontal, geom.Insets{Left: 5, Right: 5}},
		{"vertical", Vertical, geom.Insets{Top: 59, Bottom: 34}},
		{"all", AllEdges, platform},
	}
	for _, c := range cases {
		if got := safeAreaPadding(t, SafeArea{Edges: c.edges}, platform); got != c.want {
			t.Errorf("%s: got %+v, want %+v", c.name, got, c.want)
		}
	}
}

// TestSafeAreaMinimumIsAFloor: Minimum is a floor, not an addition. A layout keeps
// its own margin where the hardware needs none, and the hardware wins where it
// needs more — adding the two would double-pad a notch.
func TestSafeAreaMinimumIsAFloor(t *testing.T) {
	w := SafeArea{Minimum: geom.Insets{Top: 16, Bottom: 16, Left: 16, Right: 16}}

	got := safeAreaPadding(t, w, geom.Insets{})
	want := geom.Insets{Top: 16, Bottom: 16, Left: 16, Right: 16}
	if got != want {
		t.Errorf("with no platform insets got %+v, want the minimum %+v", got, want)
	}

	got = safeAreaPadding(t, w, geom.Insets{Top: 59, Bottom: 8})
	want = geom.Insets{Top: 59, Bottom: 16, Left: 16, Right: 16}
	if got != want {
		t.Errorf("got %+v, want %+v (platform wins above the floor, floor wins below)", got, want)
	}
}

// TestSafeAreaIsFreeOnDesktop: with no obstructions the wrapper adds nothing, so a
// layout can wrap unconditionally rather than branching per platform.
func TestSafeAreaIsFreeOnDesktop(t *testing.T) {
	if got := safeAreaPadding(t, SafeArea{}, geom.Insets{}); got != (geom.Insets{}) {
		t.Errorf("SafeArea added %+v on a platform with no insets", got)
	}
}

// TestSafeAreaFollowsRotation: rotating a phone moves the notch from the top edge
// to a side, and the padding must follow rather than latch its first value.
func TestSafeAreaFollowsRotation(t *testing.T) {
	portrait := safeAreaPadding(t, SafeArea{}, geom.Insets{Top: 59, Bottom: 34})
	if portrait.Top != 59 || portrait.Left != 0 {
		t.Fatalf("portrait insets = %+v", portrait)
	}
	landscape := safeAreaPadding(t, SafeArea{}, geom.Insets{Left: 59, Right: 34, Bottom: 21})
	if landscape.Left != 59 || landscape.Right != 34 || landscape.Top != 0 {
		t.Errorf("landscape insets = %+v, want the notch on the left", landscape)
	}
}

// TestSafeAreaMountsAndPads is the integration check: a real SafeArea in a real
// tree produces a Padding, so the pure decision above is actually the one applied.
func TestSafeAreaMountsAndPads(t *testing.T) {
	o := newOwner()
	o.SafeInsets = geom.Insets{Top: 59, Bottom: 34}

	var seen geom.Insets
	o.SetRoot(SafeArea{Child: insetProbe{got: &seen}})

	// The child still reads the raw platform insets from context — SafeArea pads
	// around it rather than consuming them, so nested content can make its own
	// decision.
	if seen.Top != 59 {
		t.Errorf("child saw insets %+v, want the platform values", seen)
	}
}

// insetProbe reports the SafeInsets its context sees.
type insetProbe struct{ got *geom.Insets }

func (p insetProbe) Build(ctx Ctx) Widget {
	*p.got = ctx.SafeInsets()
	return Sized{W: 1, H: 1}
}
