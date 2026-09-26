package widget_test

import (
	"testing"

	"github.com/doug/gophics/widget"
)

// IME input goes through the same gate as typed text. A read-only field is
// focusable, and a desktop IME composes into whatever is focused, so without
// the gate a Japanese or Chinese input method could edit a ReadOnly field.
func TestIMECommitDeclinedByReadOnlyField(t *testing.T) {
	var changes []string
	h := headless(t, widget.Sized{W: 300, Child: widget.TextField{
		Value: "ab", ReadOnly: true, Autofocus: true,
		OnChange: func(s string) { changes = append(changes, s) },
	}}, 320, 240)
	h.Render()
	h.Compose("か", 1)
	h.CommitComposition("漢")
	h.Render()
	if len(changes) != 0 {
		t.Errorf("a ReadOnly field accepted an IME commit: %q", changes)
	}
}

// MaxLength promises that IME commits which would exceed it are truncated,
// the way typing and pasting are.
func TestIMECommitCappedByMaxLength(t *testing.T) {
	var changes []string
	h := headless(t, widget.Sized{W: 300, Child: widget.TextField{
		Value: "ab", MaxLength: 3, Autofocus: true,
		OnChange: func(s string) { changes = append(changes, s) },
	}}, 320, 240)
	h.Render()
	h.Type("zz")
	if len(changes) != 1 || changes[0] != "abz" {
		t.Fatalf("typing into a 3-rune field gave %q, want [abz]", changes)
	}
	h.CommitComposition("漢字")
	h.Render()
	for _, c := range changes {
		if n := len([]rune(c)); n > 3 {
			t.Errorf("MaxLength 3 exceeded by an IME commit: %q (%d runes)", c, n)
		}
	}
}
