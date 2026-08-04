//go:build darwin || linux

package terminal

import (
	"bytes"
	"image"
	"strings"
	"testing"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// collect drains the parser's output for a fixed input into a slice.
func collect(t *testing.T, in []byte, scale float32) []shell.Event {
	t.Helper()
	ch := make(chan shell.Event, 64)
	rest := parse(in, ch, scale)
	if len(rest) != 0 {
		t.Fatalf("unconsumed tail: %q", rest)
	}
	close(ch)
	var out []shell.Event
	for e := range ch {
		out = append(out, e)
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
	ch := make(chan shell.Event, 4)
	tail := parse([]byte("\x1b[<0;10"), ch, 1)
	if len(ch) != 0 {
		t.Fatalf("partial sequence emitted an event")
	}
	tail = parse(append(tail, []byte(";10M")...), ch, 1)
	if len(tail) != 0 || len(ch) != 1 {
		t.Fatalf("completed sequence not parsed: tail=%q events=%d", tail, len(ch))
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
