package shell

// Cursor sets the mouse pointer's shape. It is an optional platform
// capability: a Window exposes it by implementing CursorWindow, and callers
// reach it through the widget layer, which returns nil when the running
// platform has no pointer to shape.
//
// Desktop and web implement it. Mobile and the terminal do not: a finger has
// no cursor, and a terminal's is the shell's to place, so Cursor() is nil
// there rather than a call that quietly does nothing.
//
// Widgets do not normally call this directly — widget.Gestures.Cursor declares
// the shape for a region and the app runner applies whichever region the
// pointer is over.
type Cursor interface {
	// Set changes the pointer's shape. Calling it with the shape already in
	// effect is cheap; the runner relies on that and sets the shape for
	// whatever is under the pointer on every hover change.
	Set(CursorShape)
}

// CursorShape names the pointer shapes an app can ask for. The set is
// deliberately small — the shapes every desktop and every browser has — so a
// widget can name one without asking which platform it is on.
type CursorShape uint8

const (
	// CursorDefault is the ordinary arrow, and the shape a region gets when
	// it asks for nothing. The zero value, so an unset Gestures.Cursor means
	// "whatever the platform's default is" rather than a forced arrow.
	CursorDefault CursorShape = iota
	// CursorText is the I-beam, for text the user can select or type into.
	CursorText
	// CursorPointer is the hand, for links.
	CursorPointer
	// CursorCrosshair is for precise picking — a canvas, a colour dropper.
	CursorCrosshair
	// CursorMove is the four-arrow, for something being dragged bodily.
	CursorMove
	// CursorNotAllowed is the barred circle, for a drop that will be refused.
	CursorNotAllowed
	// CursorWait is the busy pointer.
	CursorWait
)

// String names the shape, for logs and tests.
func (c CursorShape) String() string {
	switch c {
	case CursorText:
		return "text"
	case CursorPointer:
		return "pointer"
	case CursorCrosshair:
		return "crosshair"
	case CursorMove:
		return "move"
	case CursorNotAllowed:
		return "not-allowed"
	case CursorWait:
		return "wait"
	}
	return "default"
}

// CursorWindow is implemented by a Window that can shape the pointer. The app
// runner type-asserts the Window to it and, when present, publishes Cursor()
// to the widget tree.
type CursorWindow interface {
	// Cursor returns the pointer-shaping capability, or nil if unavailable.
	Cursor() Cursor
}
