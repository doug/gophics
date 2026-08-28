package mobile_test

import (
	"testing"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/mobile"
	"github.com/doug/gophics/widget"
)

// fieldApp is a single focusable text field, which is the smallest tree that
// exercises the IME contract end to end. It takes focus as soon as it mounts —
// a focusable widget mounted while nothing has focus takes it (see
// shellHandler.TextInputActive) — so the keyboard is wanted from the first
// frame, with no tap needed.
type fieldApp struct{}

func (fieldApp) CreateState() widget.State { return &fieldState{} }

type fieldState struct {
	widget.StateBase[fieldApp]
	text string
}

func (s *fieldState) Build(widget.Ctx) widget.Widget {
	return widget.TextField{
		Value:    s.text,
		OnChange: func(v string) { s.SetState(func() { s.text = v }) },
	}
}

// The keyboard came up on mobile long before the TextInput capability existed,
// so the interesting assertion is not "typing works" — it did. It is that the
// IME is now *told what the field holds*.
//
// Before this, ctx.TextInput() was nil on mobile, so textFieldState.syncIME
// returned at its first line and the platform keyboard ran with no surrounding
// text and no selection: autocorrect replaced the wrong word, predictive text
// had nothing to read, and the IME believed nothing was ever selected. Every
// piece of that was invisible from Go, which is why it survived — nothing was
// broken enough to fail, and no test could see it.
func TestFocusedFieldPublishesIMEContext(t *testing.T) {
	h, err := app.NewHandler(fieldApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.Resize(400, 600, 2)
	b.Snapshot(0.016) // mount + first layout; the field takes focus here

	if !b.TextInputActive() {
		t.Fatal("a focused field must ask for the keyboard")
	}
	if got := b.TextInputText(); got != "" {
		t.Errorf("IME text = %q before anything was typed, want empty", got)
	}
	rev := b.TextInputRevision()

	// Type through the same path a host uses.
	b.Text("hello")
	b.Snapshot(0.016)

	if got := b.TextInputText(); got != "hello" {
		t.Errorf("IME text = %q, want %q — the IME cannot autocorrect text it "+
			"has not been given", got, "hello")
	}
	if a, e := b.TextInputSelStart(), b.TextInputSelEnd(); a != 5 || e != 5 {
		t.Errorf("IME selection = (%d,%d), want (5,5) — the caret sits after "+
			"the typed text", a, e)
	}
	if b.TextInputRevision() == rev {
		t.Error("revision must move when the text changes, or a host polling it " +
			"never re-reads")
	}
}

// The revision is what makes a per-frame poll affordable: a host that re-read
// the text every frame would tell its IME the document changed 60 times a
// second, which restarts composition and breaks CJK input.
func TestIMERevisionIsStableWhileNothingChanges(t *testing.T) {
	h, err := app.NewHandler(fieldApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.Resize(400, 600, 2)
	b.Snapshot(0.016)
	b.Text("hi")
	b.Snapshot(0.016)

	rev := b.TextInputRevision()
	for range 5 {
		b.Snapshot(0.016)
	}
	if got := b.TextInputRevision(); got != rev {
		t.Errorf("revision moved from %d to %d across idle frames", rev, got)
	}
}

// Hide drops the editing context as well as lowering the keyboard, so the next
// field to be focused cannot inherit the last one's text — which would offer
// the user autocorrect suggestions from a field they had already left.
func TestHideClearsTheEditingContext(t *testing.T) {
	b := mobile.NewBridge(nil)
	ti := b.TextInput()
	if ti == nil {
		t.Fatal("mobile must publish the TextInput capability")
	}

	ti.Show(shell.TextInputOptions{Type: shell.TextInputEmail, Autocorrect: true}, shell.TextInputHandler{})
	ti.SetText("user@example.com", 4, 16)

	if !b.TextInputActive() {
		t.Error("Show must raise the keyboard")
	}
	if got := b.TextInputKind(); got != int(shell.TextInputEmail) {
		t.Errorf("keyboard kind = %d, want %d", got, shell.TextInputEmail)
	}
	if !b.TextInputAutocorrect() {
		t.Error("autocorrect hint was dropped")
	}
	if s, e := b.TextInputSelStart(), b.TextInputSelEnd(); s != 4 || e != 16 {
		t.Errorf("selection = (%d,%d), want (4,16) — a range, not a caret", s, e)
	}

	ti.Hide()

	if b.TextInputActive() {
		t.Error("Hide must lower the keyboard")
	}
	if got := b.TextInputText(); got != "" {
		t.Errorf("stale IME text %q survived Hide", got)
	}
}

// Announce is the half of accessibility the pull model cannot carry: the host
// asks for the tree when its focus moves, but nothing asks "did anything need
// saying?" — so a live-region message had nowhere to go on a phone.
func TestAnnouncementsQueueForTheHost(t *testing.T) {
	b := mobile.NewBridge(nil)
	a := b.Accessibility()
	if a == nil {
		t.Fatal("mobile must publish the Accessibility capability")
	}

	if got := b.TakeAnnouncement(); got != "" {
		t.Errorf("fresh bridge returned announcement %q", got)
	}

	a.Announce("3 results", false)
	a.Announce("upload failed", true)

	if got := b.TakeAnnouncement(); got != "3 results" {
		t.Errorf("first announcement = %q, want %q", got, "3 results")
	}
	if b.AnnouncementAssertive() {
		t.Error("a polite announcement must not be reported as assertive")
	}
	if got := b.TakeAnnouncement(); got != "upload failed" {
		t.Errorf("second announcement = %q, want %q", got, "upload failed")
	}
	if !b.AnnouncementAssertive() {
		t.Error("an error announcement must interrupt speech")
	}
	if got := b.TakeAnnouncement(); got != "" {
		t.Errorf("queue should be drained, got %q", got)
	}
}

// Copy and paste reached other apps only if a host wired both directions; the
// bridge holds the cache because ClipboardRead answers synchronously and a bound
// host call cannot.
func TestClipboardRoundTripsThroughTheHost(t *testing.T) {
	b := mobile.NewBridge(nil)

	// Host reports what the system pasteboard holds.
	b.SetClipboardText("from Safari")
	if got, _ := b.ClipboardRead(); got != "from Safari" {
		t.Errorf("ClipboardRead = %q, want %q", got, "from Safari")
	}
	if got := b.TakeClipboardWrite(); got != "" {
		t.Errorf("a host-originated update must not be echoed back, got %q", got)
	}

	// App copies; the host drains it and writes it to the pasteboard.
	if err := b.ClipboardWrite("copied in app"); err != nil {
		t.Fatal(err)
	}
	if got := b.TakeClipboardWrite(); got != "copied in app" {
		t.Errorf("pending write = %q, want %q", got, "copied in app")
	}
	if got := b.TakeClipboardWrite(); got != "" {
		t.Errorf("write drained twice, got %q", got)
	}
}

// Autocorrect is a replacement, not an insertion, and the bridge has to carry
// it as one. Without this an IME can see the document (SetText) and still have
// no way to fix a word in it.
func TestReplaceTextCorrectsASpan(t *testing.T) {
	h, err := app.NewHandler(fieldApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.Resize(400, 600, 2)
	b.Snapshot(0.016)

	b.Text("teh cat")
	b.Snapshot(0.016)
	if got := b.TextInputText(); got != "teh cat" {
		t.Fatalf("setup: IME text = %q", got)
	}

	// The IME corrects the first word, the way both platforms express it.
	b.ReplaceText(0, 3, "the")
	b.Snapshot(0.016)

	if got := b.TextInputText(); got != "the cat" {
		t.Errorf("after correction the field holds %q, want %q", got, "the cat")
	}
}

// A correction whose offsets no longer fit the document is clamped rather than
// dropped: the IME's copy can lag an edit by a frame, and applying it to the
// nearest valid span beats losing the user's correction.
func TestReplaceTextClampsStaleOffsets(t *testing.T) {
	h, err := app.NewHandler(fieldApp{}, app.Config{Font: goregular.TTF})
	if err != nil {
		t.Fatal(err)
	}
	b := mobile.NewBridge(h)
	b.Resize(400, 600, 2)
	b.Snapshot(0.016)
	b.Text("hi")
	b.Snapshot(0.016)

	// Far past the end of a two-rune document.
	b.ReplaceText(0, 99, "bye")
	b.Snapshot(0.016)

	if got := b.TextInputText(); got != "bye" {
		t.Errorf("field holds %q after a clamped replace, want %q", got, "bye")
	}
}

// With no field focused there is nothing to correct, and a stray call from a
// host that has not noticed the blur must not panic or edit anything.
func TestReplaceTextIsInertWithoutAField(t *testing.T) {
	b := mobile.NewBridge(nil)
	b.ReplaceText(0, 3, "nope") // must not panic
	if got := b.TextInputText(); got != "" {
		t.Errorf("IME text = %q with no field focused", got)
	}
}
