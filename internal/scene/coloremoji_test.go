package scene_test

import (
	"testing"

	"github.com/doug/gophics/internal/scene"
	"github.com/doug/gophics/paint"
)

// TestRecorderAnswersTheEmojiCapability: a widget's Draw is handed a recorder,
// not the painter canvas, so a capability that only the painter answered would
// be unreachable from exactly the place it is meant to be used.
//
// The Latin-only measurer here is the whole assertion on the false side: it
// must answer false rather than panic or claim coverage it does not have.
func TestRecorderAnswersTheEmojiCapability(t *testing.T) {
	var list scene.List
	e := paint.Emoji(list.Recorder(measurer(t)))
	if e == nil {
		t.Fatal("paint.Emoji(recorder) is nil: a widget cannot ask at all")
	}
	if e.HasColorEmoji("", "\U0001F600", 32) {
		t.Error("a recorder over a Latin-only painter reported a colour emoji")
	}
}

// TestRecorderWithNoMeasurerDoesNotClaimEmoji covers the recorder the scene
// tests themselves build — Recorder(nil). The capability is still answerable,
// because the honest answer is available without a font chain: nothing will
// draw, so false.
func TestRecorderWithNoMeasurerDoesNotClaimEmoji(t *testing.T) {
	var list scene.List
	e := paint.Emoji(list.Recorder(nil))
	if e == nil {
		t.Fatal("paint.Emoji(recorder) is nil")
	}
	if e.HasColorEmoji("", "\U0001F600", 32) {
		t.Error("a recorder with no measurer claimed a colour emoji")
	}
}
