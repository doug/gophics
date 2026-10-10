package widget

import "testing"

// countingClipboard records reads. A read is what raises the system paste
// prompt on iOS and the paste toast on Android, so "how many times did the menu
// read" is the thing worth asserting.
type countingClipboard struct {
	text  string
	reads int
}

func (c *countingClipboard) ClipboardRead() (string, error) { c.reads++; return c.text, nil }
func (c *countingClipboard) ClipboardWrite(string) error    { return nil }

// peekingClipboard also answers without reading, the way a mobile bridge does.
type peekingClipboard struct {
	countingClipboard
	has bool
}

func (c *peekingClipboard) ClipboardHasText() bool { return c.has }

// Building the menu must not read the clipboard when the backend can peek.
//
// editActionsFor runs every time the menu is built, and it used to call
// ClipboardRead to decide whether Paste had anything to offer. On iOS that read
// is what shows "would like to paste from…", so the menu asked the system to
// nag the user about a paste they had not requested.
func TestEditMenuPeeksRatherThanReadingTheClipboard(t *testing.T) {
	cb := &peekingClipboard{has: true}
	if !clipboardHasText(cb) {
		t.Error("clipboardHasText = false though the peek says there is text")
	}
	if cb.reads != 0 {
		t.Errorf("peeking clipboard was read %d times; a peek must not read", cb.reads)
	}

	cb.has = false
	if clipboardHasText(cb) {
		t.Error("clipboardHasText = true though the peek says there is none")
	}
	if cb.reads != 0 {
		t.Errorf("peeking clipboard was read %d times", cb.reads)
	}
}

// A backend that cannot peek keeps the old behaviour rather than losing Paste:
// desktop, terminal and web read the clipboard for free.
func TestClipboardWithoutPeekFallsBackToReading(t *testing.T) {
	cb := &countingClipboard{text: "from Safari"}
	if !clipboardHasText(cb) {
		t.Error("clipboardHasText = false for a non-peeking clipboard holding text")
	}
	if cb.reads != 1 {
		t.Errorf("read %d times, want 1", cb.reads)
	}

	empty := &countingClipboard{}
	if clipboardHasText(empty) {
		t.Error("clipboardHasText = true for an empty clipboard")
	}
}

// asyncPeekingClipboard is the web's shape: it can only be read
// asynchronously, and it can say up front whether a read could ever succeed.
type asyncPeekingClipboard struct {
	countingClipboard
	allowed bool
}

func (c *asyncPeekingClipboard) ClipboardHasText() bool                 { return c.allowed }
func (c *asyncPeekingClipboard) ClipboardReadAsync(func(string, error)) {}

// An async clipboard is offered optimistically — until the browser refuses.
//
// The web cannot know whether the clipboard holds text without reading it, and
// the read is asynchronous and asks the user's permission, so Paste is offered
// on the chance that it works. But once the user denies that permission, the
// read can never succeed again, and the menu went on offering a Paste that was
// guaranteed to do nothing every time it was built. A peek that reports the
// refusal takes the item away.
func TestAsyncClipboardStopsOfferingPasteOnceRefused(t *testing.T) {
	cb := &asyncPeekingClipboard{allowed: true}
	if !clipboardHasText(cb) {
		t.Error("Paste withheld from an async clipboard that has not been refused")
	}

	cb.allowed = false
	if clipboardHasText(cb) {
		t.Error("Paste still offered after the browser refused the clipboard; " +
			"the item cannot do anything and must not be shown")
	}
	if cb.reads != 0 {
		t.Errorf("the async clipboard was read %d times by the peek", cb.reads)
	}
}
