package shell

// On-screen text-input / IME capability. Because gophics draws its own text
// editor (there is no native text field to focus), the platform soft keyboard
// and IME must be driven explicitly: Show raises the keyboard and routes
// composition + committed text + editing keys to the handler; Hide dismisses it.
// A Window exposes it by implementing TextInputWindow; widgets reach it via
// ctx.TextInput(), nil where unsupported (desktop has a hardware keyboard, so
// key events already flow through Handler.OnKey and TextInput is typically nil).
// All callbacks fire on the UI goroutine.
//
// STATUS: the web implementation (a focus-driven hidden input that raises the
// mobile keyboard and forwards input/composition/edit-key events) is complete.
// The native mobile piece — routing platform IME composition into the gophics
// editor — is the larger remaining work; see docs/design-capabilities.md.

// TextInputWindow is implemented by a Window that can present a soft keyboard.
type TextInputWindow interface {
	TextInput() TextInput
}

// TextInput controls the platform on-screen keyboard and IME.
type TextInput interface {
	// Show raises the soft keyboard and begins routing editing events to h.
	Show(TextInputOptions, TextInputHandler)
	// Hide dismisses the keyboard and stops routing.
	Hide()
	// SetText informs the IME of the current editing context (the surrounding
	// text and selection range, rune offsets) so composition, autocorrect, and
	// predictive suggestions have something to work against.
	SetText(text string, selStart, selEnd int)
}

// TextInputType hints the keyboard layout the platform should present.
type TextInputType uint8

const (
	TextInputDefault TextInputType = iota
	TextInputEmail
	TextInputNumber
	TextInputURL
	TextInputSearch
)

// TextInputOptions configures the keyboard.
type TextInputOptions struct {
	Type        TextInputType
	Autocorrect bool // enable autocorrect/predictive text
}

// TextInputHandler receives editing events from the platform IME.
type TextInputHandler struct {
	// OnText commits final text — an insertion of committed characters (a
	// completed composition, an autocorrect acceptance, a typed key).
	OnText func(string)
	// OnComposing reports in-progress composition ("marked" text) for CJK,
	// emoji, and accent input; it is replaced by a later OnComposing or OnText.
	OnComposing func(string)
	// OnEditKey reports editing keys the IME sends (backspace, enter, arrows).
	OnEditKey func(EditKey)
}

// EditKey is a non-text editing action from the IME.
type EditKey uint8

const (
	EditBackspace EditKey = iota
	EditEnter
	EditLeft
	EditRight
)
