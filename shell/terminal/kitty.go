// Package terminal presents a gossamer app inside a terminal emulator that
// supports the kitty graphics protocol (kitty, Ghostty, WezTerm, Konsole). The
// core renders each frame to a CPU RGBA buffer (no GPU/window); this backend
// transmits that buffer to the terminal as an image and parses terminal input
// (SGR-pixel mouse, keyboard) into gossamer's event dispatch.
//
// Frames are transmitted incrementally: the first frame (and any frame that
// changes most of the screen) is sent whole with a=T; smaller changes send only
// the changed bounding box, composited onto the displayed image in place with
// a=f. Pixel data travels either through a temp file (t=t, for a local terminal
// sharing the filesystem) or inline as chunked base64 (for a remote transport
// such as SSH). See https://sw.kovidgoyal.net/kitty/graphics-protocol/.
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
	for y := 0; y < h; y++ {
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
	for y := 0; y < h; y++ {
		src := (r.Min.Y+y)*img.Stride + r.Min.X*4
		copy(out[y*rowLen:], img.Pix[src:src+rowLen])
	}
	return out
}

// fullFrameCmds builds the commands that transmit a w×h RGBA frame as image id
// and display it at the cursor (which the caller homes first). Placement id p=1
// is reused so re-transmits don't accumulate placements.
// fullFrameCmds transmits a w×h RGBA frame as image id and displays it at the
// cursor with placement id 1 (reused → in-place replace, no flicker). When cols
// and rows are given, the image is scaled to fill that many cells (c/r), so the
// renderer can transmit a capped-resolution image and let the terminal scale it
// to fill a large window.
func fullFrameCmds(id, w, h, cols, rows int, pixels []byte, dir string) [][]byte {
	ctrl := fmt.Sprintf("a=T,f=32,s=%d,v=%d,i=%d,p=1,C=1,q=2", w, h, id)
	if cols > 0 && rows > 0 {
		ctrl += fmt.Sprintf(",c=%d,r=%d", cols, rows)
	}
	return graphicsCmds(ctrl, pixels, dir)
}

// composeCmds builds the commands that overwrite the w×h region at (x,y) of
// image id's root frame (r=1) with pixels — an in-place partial update of the
// already-displayed image, transferring only the changed rectangle.
func composeCmds(id, x, y, w, h int, pixels []byte, dir string) [][]byte {
	ctrl := fmt.Sprintf("a=f,f=32,s=%d,v=%d,x=%d,y=%d,i=%d,r=1,q=2", w, h, x, y, id)
	return graphicsCmds(ctrl, pixels, dir)
}

// graphicsCmds packages control keys ctrl and a pixel payload into one or more
// kitty _G…\ commands. When dir is non-empty the payload is written to a temp
// file there and referenced by path (t=t); otherwise it is chunked inline as
// base64 (m=1 on every chunk but the last).
func graphicsCmds(ctrl string, payload []byte, dir string) [][]byte {
	if dir != "" {
		path, err := writeTemp(dir, payload)
		if err != nil {
			return nil
		}
		cmd := []byte{esc, '_', 'G'}
		cmd = append(cmd, ctrl...)
		cmd = append(cmd, ",t=t;"...)
		cmd = append(cmd, b64(path)...)
		cmd = append(cmd, esc, '\\')
		return [][]byte{cmd}
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
			head = fmt.Sprintf("%c_Gm=%d;", esc, m)
		}
		cmd := append([]byte(head), enc[i:end]...)
		cmd = append(cmd, esc, '\\')
		cmds = append(cmds, cmd)
	}
	return cmds
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

// writeTemp writes data to a fresh temp file in dir and returns its path. Kitty
// reads and (with t=t) deletes the file.
func writeTemp(dir string, data []byte) (string, error) {
	f, err := os.CreateTemp(dir, "gossamer-*.rgba")
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
