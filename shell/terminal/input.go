package terminal

import (
	"io"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// wheelStep is the logical-pixel scroll distance per mouse-wheel notch.
const wheelStep = 48

// readInput reads raw terminal bytes, parses them into shell events, and sends
// them on events until r returns an error (the tty closes, or an SSH client
// disconnects). On end-of-input it emits a final Closed so the run loop exits.
// It runs on its own goroutine and only writes to the channel, never touching
// the core.
func readInput(r io.Reader, events chan<- shell.Event, scale func() float32) {
	buf := make([]byte, 4096)
	var pending []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			pending = parse(append(pending, buf[:n]...), events, scale())
		}
		if err != nil {
			events <- shell.Closed{}
			return
		}
	}
}

// parse consumes as many complete input sequences as it can from the front of
// b, emitting an event for each, and returns the unconsumed tail (a partial
// sequence awaiting more bytes). A scale of 0 is treated as 1.
func parse(b []byte, events chan<- shell.Event, scale float32) []byte {
	send := func(e shell.Event) { events <- e }
	for len(b) > 0 {
		c := b[0]
		switch {
		case c == esc:
			consumed, ok := parseEsc(b, send, scale)
			if !ok {
				return b // incomplete escape sequence; wait for more bytes
			}
			b = b[consumed:]
		case c == 0x0d || c == 0x0a: // CR / LF
			send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyEnter})
			b = b[1:]
		case c == 0x7f || c == 0x08: // DEL / BS
			send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyBackspace})
			b = b[1:]
		case c == 0x09: // Tab
			send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyTab})
			b = b[1:]
		case c < 0x20: // other control chars: Ctrl-<letter>
			b = parseCtrl(c, b, send)
		default: // printable UTF-8 → text
			consumed, ok := parseText(b, send)
			if !ok {
				return b // partial UTF-8 rune at the boundary
			}
			b = b[consumed:]
		}
	}
	return nil
}

// parseCtrl handles C0 control bytes (Ctrl-A..Ctrl-Z). Ctrl-Q quits; the
// clipboard shortcuts are surfaced as key events; the rest are ignored.
func parseCtrl(c byte, b []byte, send func(shell.Event)) []byte {
	switch c {
	case 0x03, 0x11: // Ctrl-C / Ctrl-Q → quit (both, so users are never trapped)
		send(shell.Closed{})
	case 0x01: // Ctrl-A
		send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyA, Mods: shell.ModCtrl})
	case 0x16: // Ctrl-V
		send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyV, Mods: shell.ModCtrl})
	case 0x18: // Ctrl-X
		send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyX, Mods: shell.ModCtrl})
	}
	return b[1:]
}

// parseText batches a run of printable bytes into one Text event, stopping at
// the next control/escape byte. It returns ok=false when the tail holds only a
// partial multi-byte rune, so the caller waits for the rest.
func parseText(b []byte, send func(shell.Event)) (consumed int, ok bool) {
	i := 0
	for i < len(b) && b[i] >= 0x20 && b[i] != esc && b[i] != 0x7f {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 && !utf8.FullRune(b[i:]) {
			break // partial rune at the buffer boundary
		}
		i += size
	}
	if i == 0 {
		return 0, false
	}
	send(shell.Text{S: string(b[:i])})
	return i, true
}

// parseEsc handles ESC-prefixed sequences: SGR mouse reports, CSI/SS3 special
// keys, and a bare ESC. It returns the number of bytes consumed, or ok=false if
// the sequence is incomplete.
func parseEsc(b []byte, send func(shell.Event), scale float32) (consumed int, ok bool) {
	if len(b) < 2 {
		return 0, false
	}
	switch b[1] {
	case '[':
		if len(b) >= 3 && b[2] == '<' {
			return parseMouse(b, send, scale)
		}
		return parseCSI(b, send)
	case 'O': // SS3 (application-cursor arrows)
		if len(b) < 3 {
			return 0, false
		}
		if code := arrowCode(b[2]); code != shell.KeyUnknown {
			send(shell.Key{Kind: shell.KeyPress, Code: code})
		}
		return 3, true
	default:
		// Bare ESC key press (or an Alt-modified key we don't decode).
		send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyEscape})
		return 1, true
	}
}

// parseCSI handles "ESC [ ... <final>" sequences for special keys.
func parseCSI(b []byte, send func(shell.Event)) (consumed int, ok bool) {
	// Find the final byte (0x40..0x7e) that terminates the CSI sequence.
	i := 2
	for i < len(b) && (b[i] < 0x40 || b[i] > 0x7e) {
		i++
	}
	if i >= len(b) {
		return 0, false // not yet complete
	}
	final := b[i]
	params := string(b[2:i])
	var code shell.KeyCode
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		code = csiFinalCode(final)
	case '~':
		code = tildeCode(params)
	}
	if code != shell.KeyUnknown {
		send(shell.Key{Kind: shell.KeyPress, Code: code})
	}
	return i + 1, true
}

func arrowCode(final byte) shell.KeyCode { return csiFinalCode(final) }

func csiFinalCode(final byte) shell.KeyCode {
	switch final {
	case 'A':
		return shell.KeyUp
	case 'B':
		return shell.KeyDown
	case 'C':
		return shell.KeyRight
	case 'D':
		return shell.KeyLeft
	case 'H':
		return shell.KeyHome
	case 'F':
		return shell.KeyEnd
	}
	return shell.KeyUnknown
}

func tildeCode(params string) shell.KeyCode {
	switch params {
	case "1", "7":
		return shell.KeyHome
	case "4", "8":
		return shell.KeyEnd
	case "3":
		return shell.KeyDelete
	}
	return shell.KeyUnknown
}

// parseMouse decodes an SGR mouse report: ESC [ < Cb ; Cx ; Cy (M|m), where
// under SGR-Pixels mode Cx,Cy are 1-based pixel coordinates. It emits the
// corresponding pointer move/down/up/scroll event, converting pixels to logical
// coordinates by scale.
func parseMouse(b []byte, send func(shell.Event), scale float32) (consumed int, ok bool) {
	// Sequence: ESC [ < params (M|m)
	i := 3
	for i < len(b) && b[i] != 'M' && b[i] != 'm' {
		i++
	}
	if i >= len(b) {
		return 0, false
	}
	final := b[i]
	fields := strings.Split(string(b[3:i]), ";")
	if len(fields) != 3 {
		return i + 1, true // malformed; skip
	}
	cb, e1 := strconv.Atoi(fields[0])
	cx, e2 := strconv.Atoi(fields[1])
	cy, e3 := strconv.Atoi(fields[2])
	if e1 != nil || e2 != nil || e3 != nil {
		return i + 1, true
	}
	s := scale
	if s <= 0 {
		s = 1
	}
	pos := geom.Pt{X: float32(cx-1) / s, Y: float32(cy-1) / s}

	switch {
	case cb&64 != 0: // wheel
		var d geom.Pt
		switch cb & 3 {
		case 0:
			d = geom.Pt{Y: wheelStep} // wheel up
		case 1:
			d = geom.Pt{Y: -wheelStep} // wheel down
		case 2:
			d = geom.Pt{X: wheelStep}
		case 3:
			d = geom.Pt{X: -wheelStep}
		}
		send(shell.Pointer{Kind: shell.PointerScroll, Pos: pos, Scroll: d})
	case cb&32 != 0: // motion
		send(shell.Pointer{Kind: shell.PointerMove, Pos: pos})
	case final == 'M': // button press
		send(shell.Pointer{Kind: shell.PointerDown, Pos: pos, Button: sgrButton(cb)})
	default: // final == 'm', button release
		send(shell.Pointer{Kind: shell.PointerUp, Pos: pos, Button: sgrButton(cb)})
	}
	return i + 1, true
}

// sgrButton maps SGR button codes (0=left,1=middle,2=right) to gophics's
// button numbering (0=primary, 1=secondary, 2=middle).
func sgrButton(cb int) uint8 {
	switch cb & 3 {
	case 0:
		return 0 // left → primary
	case 1:
		return 2 // middle
	case 2:
		return 1 // right → secondary
	}
	return 0
}
