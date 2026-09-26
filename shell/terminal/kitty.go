// Kitty graphics protocol encoding. Frames are transmitted incrementally: the
// first frame (and any frame that changes most of the screen) is sent whole
// with a=T; smaller changes send only the changed bounding box, composited
// onto the displayed image in place with a=f. Pixel data travels either
// through a temp file (t=t, for a local terminal sharing the filesystem) or
// inline as chunked base64 (for a remote transport such as SSH). See
// https://sw.kovidgoyal.net/kitty/graphics-protocol/.

package terminal

import (
	"bytes"
	"compress/zlib"
	"encoding/base64"
	"fmt"
	"image"
	"os"
)

// esc is the ASCII escape byte that begins every control sequence.
const esc = 0x1b

// chunkSize is the maximum base64 payload per kitty command (protocol limit).
const chunkSize = 4096

// tightPixels returns img's pixels as a tight, top-to-bottom RGBA buffer
// (width*height*4, no row padding), copying only if the source has padding.
func tightPixels(img *image.RGBA) []byte {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rowLen := w * 4
	if img.Stride == rowLen && b.Min.X == 0 && b.Min.Y == 0 {
		return img.Pix[:rowLen*h]
	}
	out := make([]byte, rowLen*h)
	for y := range h {
		src := (b.Min.Y+y)*img.Stride + b.Min.X*4
		copy(out[y*rowLen:], img.Pix[src:src+rowLen])
	}
	return out
}

// subRect returns the pixels of rectangle r within img as a tight RGBA buffer.
func subRect(img *image.RGBA, r image.Rectangle) []byte {
	w, h := r.Dx(), r.Dy()
	rowLen := w * 4
	out := make([]byte, rowLen*h)
	for y := range h {
		src := (r.Min.Y+y)*img.Stride + r.Min.X*4
		copy(out[y*rowLen:], img.Pix[src:src+rowLen])
	}
	return out
}

// encoder builds the kitty commands for one transport. With dir set the
// pixels travel through temp files there (t=t); otherwise inline as chunked
// base64. It remembers the files it wrote so the previous frame's can be
// unlinked when the next one is sent.
type encoder struct {
	dir   string   // temp-file transfer dir; "" → inline base64
	temps []string // temp files written since the last reap
}

// fullFrame builds the commands that transmit a w×h RGBA frame as image id
// and display it at the cursor (which the caller homes first). Placement id
// p=1 is reused so re-transmits replace in place instead of accumulating
// placements — no flicker. When cols and rows are given, the image is scaled
// to fill that many cells (c/r), so the renderer can transmit a
// capped-resolution image and let the terminal scale it to fill a large
// window.
func (e *encoder) fullFrame(id, w, h, cols, rows int, pixels []byte) [][]byte {
	ctrl := fmt.Sprintf("a=T,f=32,s=%d,v=%d,i=%d,p=1,C=1,q=2", w, h, id)
	if cols > 0 && rows > 0 {
		ctrl += fmt.Sprintf(",c=%d,r=%d", cols, rows)
	}
	return e.cmds(ctrl, "", pixels)
}

// compose builds the commands that overwrite the w×h region at (x,y) of image
// id's root frame (r=1) with pixels — an in-place partial update of the
// already-displayed image, transferring only the changed rectangle.
//
// Every chunk repeats a=f: the protocol requires it on each chunk of
// animation-frame data, and a terminal that follows the spec takes a bare
// m= continuation for part of an ordinary transmission instead.
func (e *encoder) compose(id, x, y, w, h int, pixels []byte) [][]byte {
	ctrl := fmt.Sprintf("a=f,f=32,s=%d,v=%d,x=%d,y=%d,i=%d,r=1,q=2", w, h, x, y, id)
	return e.cmds(ctrl, "a=f,", pixels)
}

// cmds packages control keys ctrl and a pixel payload into one or more kitty
// _G…\ commands: one command referencing a temp file (t=t) when the encoder
// has a dir, else inline base64 chunks (m=1 on every chunk but the last),
// each continuation chunk prefixed with cont.
//
// A temp file that cannot be written (the dir gone, the disk full) falls back
// to inline for that frame rather than sending nothing: a frame that was
// never sent must not become the baseline the next diff is taken against, or
// the terminal keeps stale pixels until they happen to change again.
func (e *encoder) cmds(ctrl, cont string, payload []byte) [][]byte {
	if e.dir != "" {
		path, err := writeTemp(e.dir, payload)
		if err == nil {
			e.temps = append(e.temps, path)
			cmd := []byte{esc, '_', 'G'}
			cmd = append(cmd, ctrl...)
			cmd = append(cmd, ",t=t;"...)
			cmd = append(cmd, b64(path)...)
			cmd = append(cmd, esc, '\\')
			return [][]byte{cmd}
		}
		dbg("temp file transfer failed, sending inline: %v", err)
	}

	// Inline transfer goes over the PTY (or an SSH channel), so zlib-compress the
	// pixels (o=z) first — UI frames are mostly flat color and shrink 10–40×,
	// which is what makes scrolling (a whole new frame each time) usable.
	ctrl += ",o=z"
	enc := base64.StdEncoding.EncodeToString(zlibCompress(payload))
	var cmds [][]byte
	for i := 0; i < len(enc); i += chunkSize {
		end := min(i+chunkSize, len(enc))
		last := end == len(enc)
		m := 1
		if last {
			m = 0
		}
		var head string
		if i == 0 {
			head = fmt.Sprintf("%c_G%s,m=%d;", esc, ctrl, m)
		} else {
			head = fmt.Sprintf("%c_G%sm=%d;", esc, cont, m)
		}
		cmd := append([]byte(head), enc[i:end]...)
		cmd = append(cmd, esc, '\\')
		cmds = append(cmds, cmd)
	}
	return cmds
}

// reap unlinks the temp files written before this call. The terminal deletes
// a t=t file itself only when its name carries the tty-graphics-protocol
// marker (writeTemp's does), and a terminal that ignores t=t deletes nothing;
// either way, once the next frame is on the wire the previous frame's files
// are done with. Called before each frame is sent and on teardown.
func (e *encoder) reap() {
	for _, p := range e.temps {
		_ = os.Remove(p)
	}
	e.temps = e.temps[:0]
}

// zlibCompress zlib-compresses data at the fastest level (UI frames are highly
// compressible, so speed matters more than ratio here).
func zlibCompress(data []byte) []byte {
	var buf bytes.Buffer
	w, _ := zlib.NewWriterLevel(&buf, zlib.BestSpeed)
	_, _ = w.Write(data)
	_ = w.Close()
	return buf.Bytes()
}

// writeTemp writes data to a fresh temp file in dir and returns its path.
//
// The name carries "tty-graphics-protocol": the terminal deletes a t=t file
// after reading it only when the path contains that marker and sits in a
// known temp dir — a safety rule, so a hostile program cannot make it unlink
// arbitrary files. Without the marker every frame left a file behind, up to
// sixty a second.
func writeTemp(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, "tty-graphics-protocol-gophics-*.rgba")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return f.Name(), nil
}

// deleteImageCmd removes image id and all its placements (a=d, d=I). Sent on
// teardown so the terminal frees the pixels.
func deleteImageCmd(id int) []byte {
	return fmt.Appendf(nil, "%c_Ga=d,d=I,i=%d,q=2%c\\", esc, id, esc)
}

// homeCursor returns the sequence that moves the cursor to the top-left, so a
// full-frame a=T places the image over the whole screen.
func homeCursor() []byte { return []byte{esc, '[', 'H'} }

// b64 standard-base64-encodes s (kitty payloads and OSC 52 clipboard).
func b64(s string) string { return base64.StdEncoding.EncodeToString([]byte(s)) }
