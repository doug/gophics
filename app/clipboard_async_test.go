package app

import (
	"errors"
	"testing"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// asyncClip is a clipboard that can only answer asynchronously, as the web's
// can: the synchronous read fails and the result arrives through a callback
// the test releases when it chooses.
type asyncClip struct {
	text    string
	pending func(string, error)
	reads   int
}

func (c *asyncClip) ClipboardRead() (string, error) {
	return "", errors.New("synchronous read unsupported")
}
func (c *asyncClip) ClipboardWrite(string) error { return nil }
func (c *asyncClip) ClipboardReadAsync(done func(string, error)) {
	c.reads++
	c.pending = done
}

// deliver hands the clipboard text to the waiting caller.
func (c *asyncClip) deliver() {
	if c.pending != nil {
		p := c.pending
		c.pending = nil
		p(c.text, nil)
	}
}

func asyncClipField(t *testing.T, cb *asyncClip) (*Headless, *nativeFieldState) {
	t.Helper()
	h, st := nativeField(t, widget.TextField{Value: ""}, pcKeys)
	h.core.Owner.Clipboard = cb
	h.Render()
	return h, st
}

// The edit menu offers Paste on a platform that cannot peek at the clipboard.
// Hiding it there made a working action unreachable, which is what the web
// menu did: the peek failed, so Paste was never listed.
func TestAsyncClipboardMenuOffersPaste(t *testing.T) {
	cb := &asyncClip{text: "from the clipboard"}
	h, _ := asyncClipField(t, cb)

	h.core.Pointer(shell.Pointer{Kind: shell.PointerDown, Pos: geom.Pt{X: 20, Y: 30}, Button: 1})
	h.Render()
	if !hasMenuItem(h, "Paste") {
		t.Fatal("no Paste in the edit menu for a clipboard that cannot be peeked")
	}
}

// And activating it reads asynchronously and inserts what comes back.
func TestAsyncClipboardPasteInsertsOnDelivery(t *testing.T) {
	cb := &asyncClip{text: "hello"}
	h, st := asyncClipField(t, cb)

	h.KeyMod(shell.KeyV, shell.ModCtrl)
	h.Render()
	if cb.reads != 1 {
		t.Fatalf("asked the clipboard %d times, want 1", cb.reads)
	}
	if st.value != "" {
		t.Fatalf("field changed before the clipboard answered: %q", st.value)
	}
	cb.deliver()
	h.Step(0.016)
	h.Render()
	if st.value != "hello" {
		t.Fatalf("after delivery the field holds %q, want \"hello\"", st.value)
	}
}

// A read that comes back empty, or refused, leaves the field alone.
func TestAsyncClipboardEmptyLeavesTheFieldAlone(t *testing.T) {
	cb := &asyncClip{text: ""}
	h, st := asyncClipField(t, cb)
	h.KeyMod(shell.KeyV, shell.ModCtrl)
	cb.deliver()
	h.Step(0.016)
	h.Render()
	if st.value != "" {
		t.Fatalf("an empty clipboard wrote %q into the field", st.value)
	}
}

func hasMenuItem(h *Headless, label string) bool {
	for _, n := range h.Semantics() {
		if n.Label == label {
			return true
		}
	}
	return false
}
