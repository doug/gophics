package terminal

import (
	"io"
	"testing"
	"time"

	"github.com/doug/gophics/shell"
)

// events drains the parser's output for a fixed input, ignoring any tail.
func events(in string) []shell.Event {
	var out []shell.Event
	parse([]byte(in), func(e shell.Event) { out = append(out, e) }, 1)
	return out
}

// Every Ctrl+letter reaches the app as a key with ModCtrl — the editor's
// undo/redo and Emacs bindings need them — except the two that quit.
func TestParseCtrlLetters(t *testing.T) {
	cases := map[string]shell.KeyCode{
		"\x01": shell.KeyA, "\x02": shell.KeyB, "\x0b": shell.KeyK, "\x0e": shell.KeyN,
		"\x10": shell.KeyP, "\x16": shell.KeyV, "\x18": shell.KeyX, "\x19": shell.KeyY,
		"\x1a": shell.KeyZ,
	}
	for in, code := range cases {
		evs := events(in)
		if len(evs) != 1 {
			t.Errorf("%q: got %d events, want 1", in, len(evs))
			continue
		}
		k, ok := evs[0].(shell.Key)
		if !ok || k.Code != code || k.Mods != shell.ModCtrl {
			t.Errorf("%q = %+v, want Ctrl+%v", in, evs[0], code)
		}
	}
	for _, in := range []string{"\x03", "\x11"} {
		evs := events(in)
		if len(evs) != 1 {
			t.Fatalf("%q: got %d events", in, len(evs))
		}
		if _, ok := evs[0].(shell.Closed); !ok {
			t.Errorf("%q = %+v, want Closed: Ctrl-C and Ctrl-Q stay the way out", in, evs[0])
		}
	}
}

// ESC followed by a key in the same write is an Alt chord, not Escape plus a
// typed letter.
func TestParseAltChords(t *testing.T) {
	cases := []struct {
		in   string
		code shell.KeyCode
		mods shell.Mods
	}{
		{"\x1bx", shell.KeyX, shell.ModAlt},
		{"\x1bB", shell.KeyB, shell.ModAlt | shell.ModShift},
		{"\x1b\x7f", shell.KeyBackspace, shell.ModAlt},
		{"\x1b\r", shell.KeyEnter, shell.ModAlt},
	}
	for _, c := range cases {
		evs := events(c.in)
		if len(evs) != 1 {
			t.Errorf("%q: got %d events %+v, want 1", c.in, len(evs), evs)
			continue
		}
		k, ok := evs[0].(shell.Key)
		if !ok || k.Code != c.code || k.Mods != c.mods {
			t.Errorf("%q = %+v, want code=%v mods=%v", c.in, evs[0], c.code, c.mods)
		}
	}
	// A chord with nothing bound to it types nothing.
	if evs := events("\x1b/"); len(evs) != 0 {
		t.Errorf("Alt+/ produced %+v, want nothing", evs)
	}
	// ESC ESC is Escape, then whatever the second ESC introduces.
	evs := events("\x1b\x1b[A")
	if len(evs) != 2 {
		t.Fatalf("ESC ESC [A: got %+v", evs)
	}
	if k := evs[0].(shell.Key); k.Code != shell.KeyEscape {
		t.Errorf("first = %+v, want Escape", evs[0])
	}
	if k := evs[1].(shell.Key); k.Code != shell.KeyUp {
		t.Errorf("second = %+v, want Up", evs[1])
	}
}

// A bare ESC cannot be told from the start of a sequence by its bytes; the
// reader delivers it as Escape once nothing follows for escTimeout, instead
// of holding it until the next key.
func TestReadInputDeliversLoneEscape(t *testing.T) {
	pr, pw := io.Pipe()
	defer pw.Close()
	evs := make(chan shell.Event, 16)
	done := make(chan struct{})
	defer close(done)
	go readInput(pr, evs, func() float32 { return 1 }, done)

	pw.Write([]byte{esc})
	select {
	case e := <-evs:
		if k, ok := e.(shell.Key); !ok || k.Code != shell.KeyEscape {
			t.Fatalf("got %+v, want Escape", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lone ESC never delivered")
	}
	// And a sequence whose bytes arrive in two writes is still one key —
	// the timeout must not fire between them.
	pw.Write([]byte{esc, '['})
	pw.Write([]byte{'A'})
	select {
	case e := <-evs:
		if k, ok := e.(shell.Key); !ok || k.Code != shell.KeyUp {
			t.Fatalf("got %+v, want Up", e)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("split arrow never delivered")
	}
	select {
	case e := <-evs:
		t.Fatalf("unexpected extra event %+v", e)
	case <-time.After(3 * escTimeout):
	}
}

// Back and forward mouse buttons (8 and 9, encoded as 128+n) have no gophics
// button; cb&3 read them as primary and middle clicks.
func TestParseMouseExtraButtonsDropped(t *testing.T) {
	for _, in := range []string{"\x1b[<128;10;10M", "\x1b[<129;10;10m"} {
		if evs := events(in); len(evs) != 0 {
			t.Errorf("%q produced %+v, want nothing", in, evs)
		}
	}
	// Motion with such a button held is still motion.
	evs := events("\x1b[<160;10;10M")
	if len(evs) != 1 || evs[0].(shell.Pointer).Kind != shell.PointerMove {
		t.Errorf("motion with button 8 held = %+v, want PointerMove", evs)
	}
}

// A bracketed paste arrives as one Text with newlines, not as a Text and an
// Enter per line that would submit a form line by line.
func TestParseBracketedPaste(t *testing.T) {
	evs := events("\x1b[200~line one\rline two\r\nline three\x1b[201~x")
	if len(evs) != 2 {
		t.Fatalf("got %d events %+v, want paste then text", len(evs), evs)
	}
	if tx, ok := evs[0].(shell.Text); !ok || tx.S != "line one\nline two\nline three" {
		t.Errorf("paste = %+v", evs[0])
	}
	if tx, ok := evs[1].(shell.Text); !ok || tx.S != "x" {
		t.Errorf("text after paste = %+v", evs[1])
	}
	// A paste split across reads waits for its end marker.
	var got []shell.Event
	send := func(e shell.Event) { got = append(got, e) }
	tail := parse([]byte("\x1b[200~half"), send, 1)
	if len(got) != 0 || len(tail) == 0 {
		t.Fatalf("incomplete paste emitted %+v, tail %q", got, tail)
	}
	tail = parse(append(tail, "\r\x1b[201~"...), send, 1)
	if len(tail) != 0 || len(got) != 1 || got[0].(shell.Text).S != "half\n" {
		t.Fatalf("completed paste: tail=%q events=%+v", tail, got)
	}
}
