package terminal

import (
	"bytes"
	"io"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// wheelStep is the logical-pixel scroll distance per mouse-wheel notch.
const wheelStep = 48

// escTimeout is how long a lone ESC waits for the rest of a sequence before
// it is taken as the Escape key. A terminal writes a whole escape sequence in
// one go, so the bytes of a real sequence arrive together or within a few
// milliseconds over SSH; a human cannot follow Esc with another key this fast.
const escTimeout = 50 * time.Millisecond

// readInput reads raw terminal bytes, parses them into shell events, and sends
// them on events until r returns an error (the tty closes, or an SSH client
// disconnects). On end-of-input it emits a final Closed so the run loop exits.
// It runs on its own goroutine and only writes to the channel, never touching
// the core. Once done is closed nobody drains events any more, so from then
// on they are dropped rather than sent: a send that blocks forever would keep
// this goroutine alive past the run loop that owned it.
//
// The read itself runs on a further goroutine, so that the parser can wait on
// a timer as well as on bytes: a bare ESC is indistinguishable from the start
// of a sequence until either more bytes arrive or enough time passes that
// none will. Without the timer the Escape key was delivered only when the
// *next* key was pressed, together with it — so Esc to dismiss a dialog did
// nothing.
func readInput(r io.Reader, events chan<- shell.Event, scale func() float32, done <-chan struct{}) {
	send := func(e shell.Event) {
		select {
		case events <- e:
		case <-done:
		}
	}
	chunks := make(chan []byte)
	go func() {
		defer close(chunks)
		for {
			buf := make([]byte, 4096)
			n, err := r.Read(buf)
			if n > 0 {
				select {
				case chunks <- buf[:n]:
				case <-done:
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	var pending []byte
	timer := time.NewTimer(time.Hour)
	timer.Stop()
	for {
		var timeout <-chan time.Time
		if escAlone(pending) {
			timer.Reset(escTimeout)
			timeout = timer.C
		}
		select {
		case c, ok := <-chunks:
			if !ok {
				send(shell.Closed{})
				return
			}
			timer.Stop()
			pending = parse(append(pending, c...), send, scale())
		case <-timeout:
			// Nothing followed: it was the Escape key. An introducer left
			// behind it ("[" or "O") was a key of its own.
			send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyEscape})
			pending = parse(pending[1:], send, scale())
		case <-done:
			return
		}
	}
}

// escAlone reports a pending tail that is a lone ESC (or ESC and one
// introducer) — what the Escape key, and nothing else, leaves behind.
func escAlone(pending []byte) bool {
	switch string(pending) {
	case "\x1b", "\x1b[", "\x1bO":
		return true
	}
	return false
}

// parse consumes as many complete input sequences as it can from the front of
// b, emitting an event for each, and returns the unconsumed tail (a partial
// sequence awaiting more bytes). A scale of 0 is treated as 1.
func parse(b []byte, send func(shell.Event), scale float32) []byte {
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

// parseCtrl handles C0 control bytes (Ctrl-A..Ctrl-Z) as Ctrl+letter key
// events — the editor's bindings (Ctrl-Z undo, Ctrl-Y redo, the Emacs
// Ctrl-B/N/P/K/H set) are all reachable that way. Ctrl-C and Ctrl-Q quit:
// the parser cannot see whether a selection exists, so Ctrl-C stays the way
// out rather than a copy that sometimes kills the app. Ctrl-H/I/J/M are the
// Backspace, Tab and Enter bytes and never reach here.
func parseCtrl(c byte, b []byte, send func(shell.Event)) []byte {
	switch {
	case c == 0x03 || c == 0x11: // Ctrl-C / Ctrl-Q → quit (both, so users are never trapped)
		send(shell.Closed{})
	case c >= 0x01 && c <= 0x1a:
		if code := letterKey(rune('a' + c - 1)); code != shell.KeyUnknown {
			send(shell.Key{Kind: shell.KeyPress, Code: code, Mods: shell.ModCtrl})
		}
	}
	return b[1:]
}

// letterKey maps a letter or digit to its KeyCode, KeyUnknown otherwise. The
// codes are not contiguous (they were appended over time, ABI-pinned), hence a
// table rather than arithmetic.
func letterKey(r rune) shell.KeyCode {
	if r >= 'A' && r <= 'Z' {
		r += 'a' - 'A'
	}
	if r >= '0' && r <= '9' {
		return shell.Key0 + shell.KeyCode(r-'0')
	}
	return letterKeys[r]
}

var letterKeys = map[rune]shell.KeyCode{
	'a': shell.KeyA, 'b': shell.KeyB, 'c': shell.KeyC, 'd': shell.KeyD, 'e': shell.KeyE,
	'f': shell.KeyF, 'g': shell.KeyG, 'h': shell.KeyH, 'i': shell.KeyI, 'j': shell.KeyJ,
	'k': shell.KeyK, 'l': shell.KeyL, 'm': shell.KeyM, 'n': shell.KeyN, 'o': shell.KeyO,
	'p': shell.KeyP, 'q': shell.KeyQ, 'r': shell.KeyR, 's': shell.KeyS, 't': shell.KeyT,
	'u': shell.KeyU, 'v': shell.KeyV, 'w': shell.KeyW, 'x': shell.KeyX, 'y': shell.KeyY,
	'z': shell.KeyZ,
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

// Bracketed-paste markers: the terminal wraps pasted text in these when
// ?2004 is on, so a paste can be told from typing.
var (
	pasteStart = []byte("\x1b[200~")
	pasteEnd   = []byte("\x1b[201~")
)

// parseEsc handles ESC-prefixed sequences: SGR mouse reports, CSI/SS3 special
// keys, bracketed pastes, Alt chords, and a bare ESC. It returns the number of
// bytes consumed, or ok=false if the sequence is incomplete.
func parseEsc(b []byte, send func(shell.Event), scale float32) (consumed int, ok bool) {
	if len(b) < 2 {
		return 0, false // a lone ESC: readInput's timer decides what it was
	}
	switch b[1] {
	case '[':
		if bytes.HasPrefix(b, pasteStart) {
			return parsePaste(b, send)
		}
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
	case esc:
		// ESC ESC: the first is the Escape key on its own; the second is
		// parsed afresh (it may introduce a sequence of its own).
		send(shell.Key{Kind: shell.KeyPress, Code: shell.KeyEscape})
		return 1, true
	default:
		return parseAlt(b, send)
	}
}

// parseAlt handles ESC followed by a key in the same write: how a terminal
// sends an Alt (Meta) chord. Delivering it as Escape plus the key typed a
// letter for every Alt+letter. The chord becomes the key with ModAlt —
// Alt+Backspace is what deletes a word — and a chord with no key code of its
// own is swallowed rather than typed.
func parseAlt(b []byte, send func(shell.Event)) (consumed int, ok bool) {
	key := func(code shell.KeyCode, mods shell.Mods) {
		if code != shell.KeyUnknown {
			send(shell.Key{Kind: shell.KeyPress, Code: code, Mods: shell.ModAlt | mods})
		}
	}
	c := b[1]
	switch {
	case c == 0x0d || c == 0x0a:
		key(shell.KeyEnter, 0)
	case c == 0x7f || c == 0x08:
		key(shell.KeyBackspace, 0)
	case c == 0x09:
		key(shell.KeyTab, 0)
	case c < 0x20:
		// Alt+Ctrl+letter: nothing in gophics is bound to it.
	default:
		r, size := utf8.DecodeRune(b[1:])
		if r == utf8.RuneError && size == 1 && !utf8.FullRune(b[1:]) {
			return 0, false // the rune's tail is still on its way
		}
		var mods shell.Mods
		if r >= 'A' && r <= 'Z' {
			mods = shell.ModShift // the terminal folds Shift into the case
		}
		key(letterKey(r), mods)
		return 1 + size, true
	}
	return 2, true
}

// parsePaste consumes a bracketed paste and delivers it as one Text event.
// Without the brackets a multi-line paste arrived as Text and KeyEnter per
// line, and pasting into a form submitted it line by line. Line endings are
// normalised to \n: a terminal sends CR for the newlines in a paste.
func parsePaste(b []byte, send func(shell.Event)) (consumed int, ok bool) {
	body := b[len(pasteStart):]
	end := bytes.Index(body, pasteEnd)
	if end < 0 {
		return 0, false // the rest of the paste is still on its way
	}
	text := strings.ReplaceAll(string(body[:end]), "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	if text != "" {
		send(shell.Text{S: text})
	}
	return len(pasteStart) + end + len(pasteEnd), true
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
	// xterm encodes modifiers as a second parameter: ESC [ 1 ; 2 C is
	// Shift+Right, ESC [ 3 ; 5 ~ is Ctrl+Delete. Without them shift-selection
	// and word movement — which the editor implements — are unreachable.
	params, mod, _ := strings.Cut(string(b[2:i]), ";")
	var code shell.KeyCode
	switch final {
	case 'A', 'B', 'C', 'D', 'H', 'F':
		code = csiFinalCode(final)
	case '~':
		code = tildeCode(params)
	}
	if code != shell.KeyUnknown {
		send(shell.Key{Kind: shell.KeyPress, Code: code, Mods: csiMods(mod)})
	}
	return i + 1, true
}

// csiMods decodes xterm's modifier parameter: 1 plus a bitmask of shift (1),
// alt (2), ctrl (4) and meta (8). Absent or 1 means none.
func csiMods(param string) shell.Mods {
	param, _, _ = strings.Cut(param, ";") // a third parameter, if any, is not a modifier
	n, err := strconv.Atoi(param)
	if err != nil || n < 2 {
		return 0
	}
	n--
	var m shell.Mods
	if n&1 != 0 {
		m |= shell.ModShift
	}
	if n&2 != 0 {
		m |= shell.ModAlt
	}
	if n&4 != 0 {
		m |= shell.ModCtrl
	}
	if n&8 != 0 {
		m |= shell.ModSuper
	}
	return m
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
	case cb&128 != 0 && cb&32 == 0:
		// Buttons 8-11 (back, forward) are encoded as 128+n; cb&3 would
		// read back as a primary click and forward as middle. gophics has no
		// button for them, so the press and release are dropped.
		return i + 1, true
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
