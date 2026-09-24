//go:build darwin || linux

package terminal

import (
	"bytes"
	"image"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// collect drains the parser's output for a fixed input into a slice.
func collect(t *testing.T, in []byte, scale float32) []shell.Event {
	t.Helper()
	var out []shell.Event
	rest := parse(in, func(e shell.Event) { out = append(out, e) }, scale)
	if len(rest) != 0 {
		t.Fatalf("unconsumed tail: %q", rest)
	}
	return out
}

func TestParseMousePixels(t *testing.T) {
	// SGR-pixel press of the left button at pixel (100,50), scale 2 → logical (49.5,24.5).
	evs := collect(t, []byte("\x1b[<0;100;50M"), 2)
	if len(evs) != 1 {
		t.Fatalf("got %d events, want 1: %+v", len(evs), evs)
	}
	p, ok := evs[0].(shell.Pointer)
	if !ok || p.Kind != shell.PointerDown || p.Button != 0 {
		t.Fatalf("want primary PointerDown, got %+v", evs[0])
	}
	if p.Pos.X != 49.5 || p.Pos.Y != 24.5 {
		t.Errorf("pos = %v, want (49.5,24.5) — pixels/scale, 1-based", p.Pos)
	}
}

func TestParseMouseKinds(t *testing.T) {
	cases := []struct {
		in   string
		kind shell.PointerKind
		btn  uint8
	}{
		{"\x1b[<2;10;10M", shell.PointerDown, 1},    // right button → secondary
		{"\x1b[<0;10;10m", shell.PointerUp, 0},      // release
		{"\x1b[<35;10;10M", shell.PointerMove, 0},   // motion bit (32) + button
		{"\x1b[<64;10;10M", shell.PointerScroll, 0}, // wheel up
	}
	for _, c := range cases {
		evs := collect(t, []byte(c.in), 1)
		if len(evs) != 1 {
			t.Fatalf("%q: got %d events", c.in, len(evs))
		}
		p := evs[0].(shell.Pointer)
		if p.Kind != c.kind {
			t.Errorf("%q: kind = %d, want %d", c.in, p.Kind, c.kind)
		}
	}
	// Wheel up should scroll up (positive Y in gophics's convention).
	up := collect(t, []byte("\x1b[<64;5;5M"), 1)[0].(shell.Pointer)
	if up.Scroll.Y <= 0 {
		t.Errorf("wheel-up scroll = %v, want positive Y", up.Scroll)
	}
}

func TestParseKeysAndText(t *testing.T) {
	evs := collect(t, []byte("hi\x1b[A\r\x1b[3~\x11"), 1)
	// "hi" (text), Up, Enter, Delete, then Ctrl-Q → Closed.
	if len(evs) != 5 {
		t.Fatalf("got %d events: %+v", len(evs), evs)
	}
	if tx, ok := evs[0].(shell.Text); !ok || tx.S != "hi" {
		t.Errorf("evs[0] = %+v, want Text{hi}", evs[0])
	}
	if k := evs[1].(shell.Key); k.Code != shell.KeyUp {
		t.Errorf("evs[1] = %+v, want KeyUp", evs[1])
	}
	if k := evs[2].(shell.Key); k.Code != shell.KeyEnter {
		t.Errorf("evs[2] = %+v, want KeyEnter", evs[2])
	}
	if k := evs[3].(shell.Key); k.Code != shell.KeyDelete {
		t.Errorf("evs[3] = %+v, want KeyDelete", evs[3])
	}
	if _, ok := evs[4].(shell.Closed); !ok {
		t.Errorf("evs[4] = %+v, want Closed (Ctrl-Q)", evs[4])
	}
}

func TestParseHandlesSplitSequence(t *testing.T) {
	// A mouse report split across two reads: the first half yields nothing and
	// is returned as the tail; feeding the rest completes it.
	var got []shell.Event
	send := func(e shell.Event) { got = append(got, e) }
	tail := parse([]byte("\x1b[<0;10"), send, 1)
	if len(got) != 0 {
		t.Fatalf("partial sequence emitted an event")
	}
	tail = parse(append(tail, []byte(";10M")...), send, 1)
	if len(tail) != 0 || len(got) != 1 {
		t.Fatalf("completed sequence not parsed: tail=%q events=%d", tail, len(got))
	}
}

// xterm carries modifiers as a second CSI parameter. Without them Shift+Arrow
// selection and Ctrl/Alt word movement never reach the editor from a
// terminal.
func TestParseCSIModifiers(t *testing.T) {
	cases := []struct {
		in   string
		code shell.KeyCode
		mods shell.Mods
	}{
		{"\x1b[1;2C", shell.KeyRight, shell.ModShift},
		{"\x1b[1;5D", shell.KeyLeft, shell.ModCtrl},
		{"\x1b[1;3D", shell.KeyLeft, shell.ModAlt},
		{"\x1b[1;6C", shell.KeyRight, shell.ModShift | shell.ModCtrl},
		{"\x1b[3;5~", shell.KeyDelete, shell.ModCtrl},
		{"\x1b[1;2H", shell.KeyHome, shell.ModShift},
		{"\x1b[1;2F", shell.KeyEnd, shell.ModShift},
		{"\x1b[C", shell.KeyRight, 0},
		{"\x1b[3~", shell.KeyDelete, 0},
	}
	for _, c := range cases {
		evs := collect(t, []byte(c.in), 1)
		if len(evs) != 1 {
			t.Errorf("%q: got %d events, want 1", c.in, len(evs))
			continue
		}
		k, ok := evs[0].(shell.Key)
		if !ok || k.Kind != shell.KeyPress || k.Code != c.code || k.Mods != c.mods {
			t.Errorf("%q = %+v, want KeyPress code=%v mods=%v", c.in, evs[0], c.code, c.mods)
		}
	}
}

// eofTTY is a transport whose input has already ended: RunTTY must unwind.
type eofTTY struct{ io.Writer }

func (eofTTY) Read([]byte) (int, error) { return 0, io.EOF }
func (eofTTY) Size() (int, int)         { return 320, 200 }
func (eofTTY) Resize() <-chan struct{}  { return nil }

// recordingHandler keeps the events it is given.
type recordingHandler struct{ events []shell.Event }

func (*recordingHandler) Frame(shell.Window, shell.Frame, float64) {}
func (h *recordingHandler) Event(_ shell.Window, e shell.Event)    { h.events = append(h.events, e) }

// shell.Window.Close promises the handler receives Closed; an app saves on
// quit there, and every way out of RunTTY — transport EOF included — has to
// deliver it before returning.
func TestRunTTYDeliversClosed(t *testing.T) {
	h := &recordingHandler{}
	if err := RunTTY(h, shell.Config{}, eofTTY{io.Discard}); err != nil {
		t.Fatal(err)
	}
	var closed int
	for _, e := range h.events {
		if _, ok := e.(shell.Closed); ok {
			closed++
		}
	}
	if closed != 1 {
		t.Fatalf("handler received Closed %d times, want once (events: %+v)", closed, h.events)
	}
}

// After the run loop has returned nobody drains the event channel; the reader
// must drop events rather than block on them forever, and still hit EOF.
func TestReadInputDoesNotBlockAfterDone(t *testing.T) {
	events := make(chan shell.Event) // unbuffered: any send without a receiver blocks
	done := make(chan struct{})
	close(done)
	finished := make(chan struct{})
	go func() {
		readInput(strings.NewReader(strings.Repeat("\x1b[<35;10;10M", 300)), events, func() float32 { return 1 }, done)
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("readInput is still blocked on a send nobody will receive")
	}
}

func TestPresentEmitsKitty(t *testing.T) {
	var buf bytes.Buffer
	ts := &termState{out: &buf, scale: 1, imageID: 1}
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	ts.present(img)
	out := buf.String()
	if !strings.Contains(out, "\x1b[H") {
		t.Error("present should home the cursor before placing the image")
	}
	if !strings.Contains(out, "\x1b_G") || !strings.Contains(out, "s=8,v=4") {
		t.Errorf("present output missing kitty image for 8x4: %q", out)
	}
	// An identical second frame is deduped (no new output).
	before := buf.Len()
	ts.present(img)
	if buf.Len() != before {
		t.Error("identical frame should be deduped, not re-sent")
	}
}

// TestEndToEndFrame drives a real app through one frame and confirms the
// terminal backend turns the rendered pixels into a kitty image — the full
// seam from widget tree → CPU raster → PixelTarget → kitty.
func TestEndToEndFrame(t *testing.T) {
	root := widget.Canvas{Draw: func(c paint.Canvas, size geom.Size) {
		c.Clear(paint.RGB(0.1, 0.2, 0.3))
	}}
	h, err := app.NewHandler(root, app.Config{Size: geom.Size{W: 120, H: 80}})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	ts := &termState{out: &buf, scale: 1, imageID: 1, pw: 120, ph: 80}
	win := &window{ts: ts}
	fr := &frame{ts: ts}

	h.Frame(win, fr, 0)

	out := buf.String()
	if !strings.Contains(out, "\x1b_G") || !strings.Contains(out, "s=120,v=80") {
		t.Fatalf("end-to-end frame did not emit a 120x80 kitty image: %q", truncate(out))
	}
}

func truncate(s string) string {
	if len(s) > 120 {
		return s[:120] + "..."
	}
	return s
}
