package mobile

import "github.com/doug/gophics/shell"

// TextInput makes the Bridge a shell.TextInputWindow.
//
// The keyboard itself already worked before this existed: the host polls
// TextInputActive every frame and raises or dismisses the platform keyboard on
// the transition, and committed text, editing keys and IME composition come
// back in through Text, Key and Composition. What was missing is the other
// direction — telling the IME what the field currently holds.
//
// So this implementation deliberately does not invoke the handler's OnText,
// OnComposing or OnEditKey. Those already reach the editor through
// handler.Event, and calling the handler as well would insert every keystroke
// twice. OnReplace is the exception, and the reason the handler is retained at
// all: replacing a span is what autocorrect *is*, and no existing Bridge event
// can express it. What this adds is:
//
//   - Show/Hide, so the keyboard is raised because a field asked for it rather
//     than inferred from focus (TextInputActive keeps the inference as a
//     fallback, so hosts and widgets that predate this keep working).
//   - The keyboard type and autocorrect hint, which the host had no way to read.
//   - SetText, the surrounding text and selection. Without it autocorrect
//     replaces the wrong word, predictive text has nothing to predict from, and
//     the IME believes nothing is ever selected.
//   - ReplaceText, which the host calls when the IME corrects a span.
//
// The host reads all of it through the accessors below. Per the Bridge
// contract, every one of these runs on the host UI thread, so the state needs
// no lock — the same reason a11y and clipboard state carry none.
func (b *Bridge) TextInput() shell.TextInput { return mobileTextInput{b} }

type mobileTextInput struct{ b *Bridge }

// Show records that a field wants the keyboard, with the layout it asked for.
// Only the handler's OnReplace is kept; see the note on Bridge.TextInput.
func (t mobileTextInput) Show(opts shell.TextInputOptions, h shell.TextInputHandler) {
	b := t.b
	b.tiReplace = h.OnReplace
	if b.tiActive && b.tiType == opts.Type && b.tiAutocorrect == opts.Autocorrect {
		return
	}
	b.tiActive = true
	b.tiType = opts.Type
	b.tiAutocorrect = opts.Autocorrect
	b.tiRev++
}

// Hide records that no field wants the keyboard, and drops the stale editing
// context so the next field does not inherit the last one's text.
func (t mobileTextInput) Hide() {
	b := t.b
	if !b.tiActive {
		return
	}
	b.tiActive = false
	b.tiText, b.tiSelStart, b.tiSelEnd = "", 0, 0
	b.tiReplace = nil
	b.tiRev++
}

// SetText publishes the editing context. Rune offsets, matching the capability
// contract; hosts that need UTF-16 offsets (iOS) convert on their side, because
// only they know which encoding their IME speaks.
func (t mobileTextInput) SetText(text string, selStart, selEnd int) {
	b := t.b
	if b.tiText == text && b.tiSelStart == selStart && b.tiSelEnd == selEnd {
		return
	}
	b.tiText, b.tiSelStart, b.tiSelEnd = text, selStart, selEnd
	b.tiRev++
}

// TextInputRevision changes whenever any of the state below does. Hosts poll it
// next to NeedsFrame and only re-read — and only tell their IME the document
// changed — when it moves, which keeps a per-frame poll from restarting
// composition on every frame.
func (b *Bridge) TextInputRevision() int { return b.tiRev }

// TextInputKind reports the requested keyboard layout: 0 default, 1 email,
// 2 number, 3 URL, 4 search (shell.TextInputType).
func (b *Bridge) TextInputKind() int { return int(b.tiType) }

// TextInputAutocorrect reports whether autocorrect and predictive text were
// asked for.
func (b *Bridge) TextInputAutocorrect() bool { return b.tiAutocorrect }

// TextInputText is the full text of the focused field, "" when none is focused.
func (b *Bridge) TextInputText() string { return b.tiText }

// TextInputSelStart / TextInputSelEnd are the selection in rune offsets. They
// are equal when the selection is a caret.
func (b *Bridge) TextInputSelStart() int { return b.tiSelStart }
func (b *Bridge) TextInputSelEnd() int   { return b.tiSelEnd }

// ReplaceText applies an IME correction: the text between two rune offsets is
// replaced by s.
//
// Hosts call it for autocorrect and predictive replacement — iOS from
// UITextInput.replace(_:withText:), Android from an InputConnection that
// deletes a span and commits over it. Out-of-range offsets are clamped rather
// than rejected, because the IME's idea of the document can lag a frame behind
// the editor's and dropping the correction is worse than applying it to the
// nearest valid span.
//
// A no-op when no field is focused, or when the field's handler offers no
// OnReplace.
func (b *Bridge) ReplaceText(startRune, endRune int, s string) {
	if b.tiReplace == nil {
		return
	}
	n := len([]rune(b.tiText))
	clamp := func(v int) int {
		if v < 0 {
			return 0
		}
		if v > n {
			return n
		}
		return v
	}
	a, z := clamp(startRune), clamp(endRune)
	if a > z {
		a, z = z, a
	}
	b.tiReplace(a, z, s)
}
