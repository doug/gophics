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
