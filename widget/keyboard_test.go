package widget

import (
	"testing"

	"github.com/doug/gophics/geom"
)

func TestKeyboardPadding(t *testing.T) {
	cases := []struct {
		name                     string
		keyboard, extra, consume float32
		want                     float32
	}{
		{"hidden", 0, 0, 0, 0},
		{"hidden with extra keeps no gap", 0, 24, 0, 0},
		{"up", 336, 0, 0, 336},
		{"up with extra", 336, 16, 0, 352},
		{"capped", 336, 0, 100, 100},
		{"cap above the keyboard is a no-op", 200, 0, 500, 200},
		{"capped plus extra", 336, 16, 100, 116},
	}
	for _, c := range cases {
		if got := keyboardPadding(c.keyboard, c.extra, c.consume); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}

// TestKeyboardAvoidingIsInertWhenHidden is the property that lets a layout wrap
// unconditionally: on desktop, on the web, and whenever the keyboard is down, the
// wrapper must add nothing at all.
func TestKeyboardAvoidingIsInertWhenHidden(t *testing.T) {
	o := newOwner()
	o.KeyboardInset = 0

	var seen float32 = -1
	o.SetRoot(KeyboardAvoiding{Extra: 20, Child: keyboardProbe{got: &seen}})
	if seen != 0 {
		t.Errorf("child saw keyboard inset %v, want 0", seen)
	}
	if got := keyboardPadding(o.KeyboardInset, 20, 0); got != 0 {
		t.Errorf("padding with the keyboard hidden = %v, want 0", got)
	}
}

// TestKeyboardAvoidingRespondsToTheKeyboard checks the value reaches the tree at
// all, which is the part the plumbing has to get right.
func TestKeyboardAvoidingRespondsToTheKeyboard(t *testing.T) {
	o := newOwner()
	o.KeyboardInset = 336

	var seen float32
	o.SetRoot(KeyboardAvoiding{Child: keyboardProbe{got: &seen}})
	if seen != 336 {
		t.Errorf("child saw keyboard inset %v, want 336", seen)
	}
}

// TestKeyboardIsSeparateFromSafeInsets pins the design choice: a screen that pads
// for the home indicator must not also pad for a keyboard that is down, and vice
// versa. Folding them together would make both wrong.
func TestKeyboardIsSeparateFromSafeInsets(t *testing.T) {
	o := newOwner()
	o.SafeInsets = geom.Insets{Bottom: 34}
	o.KeyboardInset = 0

	if got := resolveSafeInsets(o.SafeInsets, 0, geom.Insets{}); got.Bottom != 34 {
		t.Errorf("safe bottom inset = %v, want the home indicator's 34", got.Bottom)
	}
	if got := keyboardPadding(o.KeyboardInset, 0, 0); got != 0 {
		t.Errorf("keyboard padding = %v with the keyboard down, want 0", got)
	}

	// Keyboard up: the keyboard pads, the safe area is unchanged.
	o.KeyboardInset = 336
	if got := resolveSafeInsets(o.SafeInsets, 0, geom.Insets{}); got.Bottom != 34 {
		t.Errorf("raising the keyboard changed the safe inset to %v", got.Bottom)
	}
	if got := keyboardPadding(o.KeyboardInset, 0, 0); got != 336 {
		t.Errorf("keyboard padding = %v, want 336", got)
	}
}

// keyboardProbe reports the keyboard inset its context sees.
type keyboardProbe struct{ got *float32 }

func (p keyboardProbe) Build(ctx Ctx) Widget {
	*p.got = ctx.KeyboardInset()
	return Sized{W: 1, H: 1}
}
