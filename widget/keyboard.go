package widget

import (
	"github.com/doug/gophics/geom"
)

// KeyboardAvoiding keeps its child clear of the on-screen keyboard by padding the
// bottom with the keyboard's height.
//
// Without it a phone form is unusable in the ordinary case: tapping the last
// field raises a keyboard over the bottom half of the screen, and the field the
// user is typing into — along with the button they need next — is behind it. The
// platform does not move anything; an app that draws its own pixels has to.
//
// Padding rather than translating is the composable choice. Inside a Scroll it
// makes the content taller than the visible area, so the user (or a controller)
// can bring any field above the keyboard and nothing is ever pushed off the top —
// which is what shifting the whole layout up does to a tall form.
//
//	widget.KeyboardAvoiding{Child: widget.Scroll{Child: myForm}}
//
// It is inert wherever no keyboard is reported: desktop, web, and mobile while the
// keyboard is down. Wrapping unconditionally is the intended use.
type KeyboardAvoiding struct {
	// Extra is added on top of the keyboard's height when the keyboard is up, so a
	// form can keep a gap between the focused field and the keys.
	Extra float32
	// Consume caps how much of the keyboard is compensated for. Zero means all of
	// it. A layout that already sits above the keyboard (a bottom bar the host
	// moves itself) can use this to opt out without losing the wrapper.
	Consume float32
	Child   Widget
}

func (KeyboardAvoiding) CreateState() State { return &keyboardAvoidingState{} }

type keyboardAvoidingState struct {
	StateBase[KeyboardAvoiding]
}

func (s *keyboardAvoidingState) Build(ctx Ctx) Widget {
	w := s.W()
	return Padding{
		Insets: geom.Insets{Bottom: keyboardPadding(ctx.KeyboardInset(), w.Extra, w.Consume)},
		Child:  w.Child,
	}
}

// keyboardPadding is the bottom inset for a given keyboard height. It is a pure
// function so the caps and the hidden case can be tested without a keyboard.
//
// Extra is only applied when the keyboard is actually up: a permanent gap at the
// bottom of every screen is not what a caller asking to avoid the keyboard means.
func keyboardPadding(keyboard, extra, consume float32) float32 {
	if keyboard <= 0 {
		return 0
	}
	if consume > 0 && keyboard > consume {
		keyboard = consume
	}
	return keyboard + extra
}
