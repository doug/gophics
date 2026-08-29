// Package terminal presents a gophics app inside a terminal emulator that
// supports the kitty graphics protocol (kitty, Ghostty, WezTerm, Konsole). The
// core renders each frame to a CPU RGBA buffer (no GPU/window); this backend
// transmits that buffer to the terminal as an image and parses terminal input
// (SGR-pixel mouse, keyboard) into gophics's event dispatch.
//
// The backend is transport-agnostic. Run drives the local process terminal
// (os.Stdin/os.Stdout). RunTTY drives any TTY — the same renderer serves a
// gophics app over an SSH session (e.g. behind charmbracelet/wish or
// gliderlabs/ssh): the SSH server is the caller's concern, gophics only needs
// the session's byte stream, pixel size, and resize notifications.
//
// See https://sw.kovidgoyal.net/kitty/graphics-protocol/ for the wire format.
package terminal

import (
	"errors"
	"fmt"
	"image"
	"io"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/shell"
)

// errNotATerminal is returned when the local stdin is not a tty.
var errNotATerminal = errors.New("terminal: stdin is not a terminal")

// dbgW is the debug sink: set GOPHICS_TERM_DEBUG to a path (or "1" for
// /tmp/gophics-term.log) to trace sizing and per-frame transmits to a file —
// the on-screen graphics can't be inspected, so this is how terminal issues get
// diagnosed.
var dbgW io.Writer

func init() {
	p := os.Getenv("GOPHICS_TERM_DEBUG")
	if p == "" {
		return
	}
	if p == "1" {
		p = "/tmp/gophics-term.log"
	}
	if f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		dbgW = f
	}
}

func dbg(format string, args ...any) {
	if dbgW != nil {
		fmt.Fprintf(dbgW, format+"\n", args...)
	}
}

// TTY is a terminal transport: a duplex byte stream plus its drawable pixel
// size and resize notifications. Both the local process terminal and an SSH
// session satisfy it, so the same renderer serves either.
type TTY interface {
	io.Reader // terminal input: keyboard, mouse, and query replies
	io.Writer // terminal output: graphics and control sequences
	// Size reports the drawable area in pixels; (0,0) if unknown (the backend
	// then falls back to a default so it can still render).
	Size() (w, h int)
	// Resize returns a channel that fires when Size may have changed. It may
	// be nil for a transport that never resizes.
	Resize() <-chan struct{}
}

// FileTransport is an optional TTY capability: a filesystem directory the
// terminal can read pixel data from, enabling kitty's temp-file transfer (t=t)
// rather than streaming pixels inline. A local terminal implements it (its temp
// dir); a remote transport such as SSH does not, so the backend falls back to
// inline base64 — correct in both cases, just cheaper locally.
type FileTransport interface{ TempDir() string }

// CellGrid is an optional TTY capability reporting the terminal's cell grid
// (columns, rows). The backend uses it to scale one rendered image to fill the
// whole terminal (kitty c=/r=), so it can render at a capped resolution rather
// than the terminal's full — often enormous — pixel count.
type CellGrid interface{ CellGrid() (cols, rows int) }

// targetLogicalW aims the UI at a comfortable logical width on hidpi terminals.
const targetLogicalW = 1400

// maxImageLong caps the transmitted image's long edge so a huge (e.g. 4K)
// terminal doesn't re-render/encode tens of megapixels per frame — the image is
// scaled up to fill. Lower it (GOPHICS_TERM_MAXRES) to trade sharpness for
// faster scrolling; the whole per-frame cost scales with pixel count.
func maxImageLong() int {
	if v := os.Getenv("GOPHICS_TERM_MAXRES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 400 {
			return n
		}
	}
	return 2000
}

// contentScaleFor returns the logical→physical content scale: the
// GOPHICS_TERM_SCALE override if set, else one that targets ~targetLogicalW
// logical pixels wide (so hidpi terminals don't render microscopically).
func contentScaleFor(pw int) float32 {
	if v := os.Getenv("GOPHICS_TERM_SCALE"); v != "" {
		if f, err := strconv.ParseFloat(v, 32); err == nil && f > 0 {
			return float32(f)
		}
	}
	s := float32(pw) / targetLogicalW
	if s < 1 {
		s = 1
	}
	return s
}

// RunTTY presents handler h over the terminal transport tty, blocking until the
// app exits (Ctrl-Q, a Closed event, or the transport reaching EOF — e.g. an
// SSH client disconnecting). tty must be a terminal that supports the kitty
// graphics protocol and SGR-pixel mouse reporting.
//
// The app renders at the transport's full pixel resolution; UI is laid out in
// logical pixels = physical / scale, where scale defaults to 1 and can be
// overridden with GOPHICS_TERM_SCALE.
func RunTTY(h shell.Handler, cfg shell.Config, tty TTY) error {
	ts := &termState{out: tty, imageID: 1, done: make(chan struct{})}
	// Default to inline base64 transfer: temp-file transfer (t=t) is not honored
	// by every terminal (e.g. Ghostty renders nothing). Opt in with
	// GOPHICS_TERM_TMPFILE=1 on terminals that support it (kitty).
	if ft, ok := tty.(FileTransport); ok && os.Getenv("GOPHICS_TERM_TMPFILE") != "" {
		ts.dir = ft.TempDir()
	}
	// Default to whole-frame transmits (a=T), which every kitty-graphics terminal
	// supports. a=f region-compose partial updates are opt-in (GOPHICS_TERM_COMPOSE=1):
	// some terminals don't implement the animation/frame protocol.
	ts.full = os.Getenv("GOPHICS_TERM_COMPOSE") == "" || os.Getenv("GOPHICS_TERM_FULLFRAME") != ""
	ts.dirty.Store(true) // force the first frame (also binds h.window)

	ts.applySize(tty)
	dbg("start phys=%dx%d cells=%dx%d content=%.2f render=%.2f tempfile=%v compose=%v",
		ts.pw, ts.ph, ts.cols, ts.rows, ts.scale, ts.renderFull, ts.dir != "", !ts.full)

	setup(tty)
	defer teardown(tty, ts.imageID)
	if cfg.Title != "" {
		ts.setTitle(cfg.Title)
	}

	win := &window{ts: ts}
	fr := &frame{ts: ts}

	events := make(chan shell.Event, 128)
	go readInput(tty, events, ts.contentScale)

	resize := tty.Resize()
	ticker := time.NewTicker(time.Second / 60)
	defer ticker.Stop()

	// Dynamic resolution (video-encoding style): frames arriving faster than
	// motionGap are "inter frames" rendered at reduced resolution; when motion
	// stops, settle fires a full-resolution "keyframe". Disable with
	// GOPHICS_TERM_NODYNAMIC.
	dynamic := os.Getenv("GOPHICS_TERM_NODYNAMIC") == ""
	const motionGap = 130 * time.Millisecond
	const settleDelay = 180 * time.Millisecond
	settle := time.NewTimer(time.Hour)
	settle.Stop()
	defer settle.Stop()
	last := time.Now()
	var motionLast time.Time

	for {
		select {
		case <-ts.done:
			return nil
		case <-resize:
			ts.applySize(tty)
			h.Event(win, shell.Resize{Size: ts.logicalSize(), Scale: ts.scale})
			ts.dirty.Store(true)
		case e := <-events:
			if _, ok := e.(shell.Closed); ok {
				return nil // Ctrl-Q or transport EOF
			}
			h.Event(win, e)
			ts.dirty.Store(true) // repaint after input (present dedups no-ops)
		case <-settle.C:
			ts.setMotion(false) // motion settled → render a crisp keyframe
			ts.dirty.Store(true)
		case <-ticker.C:
			if ts.dirty.Swap(false) {
				now := time.Now()
				dt := now.Sub(last).Seconds()
				last = now
				moving := dynamic && !motionLast.IsZero() && now.Sub(motionLast) < motionGap
				motionLast = now
				ts.setMotion(moving)
				t0 := time.Now()
				h.Frame(win, fr, dt)
				dbg("frame %.1fms moving=%v", float64(time.Since(t0).Microseconds())/1000, moving)
				if moving {
					settle.Reset(settleDelay)
				}
			}
		}
	}
}

// termState is the shared backend state: output sink, current pixel size, the
// repaint flag, and the last transmitted frame (for diffing). dirty is set from
// any goroutine via Invalidate; done is closed once.
type termState struct {
	out     io.Writer
	scale   float32 // content scale: logical = physical/scale; pointer coords /scale
	imageID int
	dir     string // temp-file transfer dir; "" → inline base64
	full    bool   // GOPHICS_TERM_FULLFRAME: always send whole frames

	dirty atomic.Bool

	mu sync.Mutex
	// Render scales (image px = logical * scale). full is the crisp keyframe
	// resolution; motion is the reduced resolution used while the view is
	// actively changing (scrolling); cur is what the next frame renders at.
	renderFull, renderMotion, renderCur float32
	pw, ph                              int // physical pixels
	cols, rows                          int // cell grid (kitty scale-to-fill)

	lastFrame                []byte // previous frame's pixels, for diffing
	lastW, lastH, lastStride int

	done     chan struct{}
	doneOnce sync.Once
}

func (ts *termState) finish() { ts.doneOnce.Do(func() { close(ts.done) }) }

// applySize refreshes the physical size, cell grid, and derived content/render
// scales from the transport. The content scale keeps the UI a comfortable
// logical size on hidpi terminals; the render scale caps the transmitted image
// so a huge terminal doesn't cost tens of MB per frame (kitty scales it to fill).
func (ts *termState) applySize(tty TTY) {
	pw, ph := tty.Size()
	if pw <= 0 || ph <= 0 {
		pw, ph = 800, 480
	}
	cols, rows := 0, 0
	if cg, ok := tty.(CellGrid); ok {
		cols, rows = cg.CellGrid()
	}
	cs := contentScaleFor(pw)
	long := float32(max(pw, ph))
	logicalLong := long / cs
	full := clampScale(min(long, float32(maxImageLong()))/logicalLong, 1, 8)
	// Motion frames render at ~half the long edge (¼ the pixels) — the
	// low-detail "inter frame"; the keyframe restores full when motion settles.
	motion := clampScale(min(long, float32(maxImageLong())/2)/logicalLong, 0.5, 8)

	ts.mu.Lock()
	ts.pw, ts.ph, ts.cols, ts.rows = pw, ph, cols, rows
	ts.scale = cs
	ts.renderFull, ts.renderMotion, ts.renderCur = full, motion, full
	ts.mu.Unlock()
}

// setMotion selects the resolution for the next frame: reduced while the view is
// actively changing, full when it has settled.
func (ts *termState) setMotion(moving bool) {
	ts.mu.Lock()
	if moving {
		ts.renderCur = ts.renderMotion
	} else {
		ts.renderCur = ts.renderFull
	}
	ts.mu.Unlock()
}

func clampScale(v, lo, hi float32) float32 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func (ts *termState) logicalSize() geom.Size {
	ts.mu.Lock()
	pw, ph, s := ts.pw, ts.ph, ts.scale
	ts.mu.Unlock()
	if s <= 0 {
		s = 1
	}
	return geom.Size{W: float32(pw) / s, H: float32(ph) / s}
}

// contentScale returns the current logical→physical scale (for pointer mapping).
func (ts *termState) contentScale() float32 {
	ts.mu.Lock()
	s := ts.scale
	ts.mu.Unlock()
	if s <= 0 {
		s = 1
	}
	return s
}

func (ts *termState) setTitle(title string) {
	fmt.Fprintf(ts.out, "%c]2;%s%c", esc, title, 0x07) // OSC 2 ; title BEL
}

// fullFramePct is the damage-coverage above which a partial update is not worth
// it and the whole frame is re-sent instead.
const fullFramePct = 60

// present transmits a rendered frame to the terminal. The first frame, a resize,
// or a change covering most of the screen is sent whole (a=T); a smaller change
// sends only its bounding box, composited in place (a=f). Unchanged frames emit
// nothing.
func (ts *termState) present(img *image.RGBA) {
	b := img.Bounds()
	w, h, stride := b.Dx(), b.Dy(), img.Stride

	full := ts.full || ts.lastFrame == nil ||
		ts.lastW != w || ts.lastH != h || ts.lastStride != stride
	if !full {
		tiles, area := diffTiles(ts.lastFrame, img.Pix, w, h, stride)
		if len(tiles) == 0 {
			return // dedup: nothing moved this frame
		}
		if area*100 >= w*h*fullFramePct {
			full = true // change is widespread; a whole-frame send is cheaper
		} else {
			var cmds [][]byte
			for _, r := range tiles {
				cmds = append(cmds, composeCmds(ts.imageID, r.Min.X, r.Min.Y, r.Dx(), r.Dy(),
					subRect(img, r), ts.dir)...)
			}
			ts.writeCmds(nil, cmds)
			ts.saveFrame(img.Pix, w, h, stride)
			return
		}
	}
	// Transmit + display the whole frame (a=T), scaled to fill the terminal
	// (c/r cells) so we can render at a capped resolution. Reusing placement
	// id p=1 replaces the image in place — no delete, so no black flicker.
	ts.mu.Lock()
	cols, rows := ts.cols, ts.rows
	ts.mu.Unlock()
	t0 := time.Now()
	cmds := fullFrameCmds(ts.imageID, w, h, cols, rows, tightPixels(img), ts.dir)
	encMS := float64(time.Since(t0).Microseconds()) / 1000
	ts.writeCmds(homeCursor(), cmds)
	ts.saveFrame(img.Pix, w, h, stride)
	dbg("present img=%dx%d fill=%dx%d encode=%.1fms", w, h, cols, rows, encMS)
}

// saveFrame records the just-transmitted frame for next frame's diff.
func (ts *termState) saveFrame(pix []byte, w, h, stride int) {
	ts.lastFrame = append(ts.lastFrame[:0], pix...)
	ts.lastW, ts.lastH, ts.lastStride = w, h, stride
}

// writeCmds writes an optional prefix (e.g. cursor-home) followed by the kitty
// commands as a single write, so a frame reaches the terminal atomically.
func (ts *termState) writeCmds(prefix []byte, cmds [][]byte) {
	n := len(prefix)
	for _, c := range cmds {
		n += len(c)
	}
	buf := make([]byte, 0, n)
	buf = append(buf, prefix...)
	for _, c := range cmds {
		buf = append(buf, c...)
	}
	nw, err := ts.out.Write(buf)
	dbg("write %d/%d bytes err=%v", nw, len(buf), err)
}

// setup switches to the alternate screen, hides the cursor, and enables
// full-motion SGR-pixel mouse reporting.
func setup(out io.Writer) {
	io.WriteString(out, "\x1b[?1049h"+ // alternate screen buffer
		"\x1b[?25l"+ // hide cursor
		"\x1b[2J"+ // clear
		"\x1b[?1003h"+ // report all mouse motion + buttons
		"\x1b[?1006h"+ // SGR mouse encoding
		"\x1b[?1016h") // SGR-Pixels: report mouse position in pixels
}

// teardown reverses setup and frees the transmitted image.
func teardown(out io.Writer, imageID int) {
	out.Write(deleteImageCmd(imageID))
	io.WriteString(out, "\x1b[?1016l\x1b[?1006l\x1b[?1003l"+ // mouse off
		"\x1b[?25h"+ // show cursor
		"\x1b[?1049l") // leave alternate screen
}

// window implements shell.Window against the terminal.
type window struct{ ts *termState }

func (w *window) Invalidate()                    { w.ts.dirty.Store(true) }
func (w *window) SetTitle(title string)          { w.ts.setTitle(title) }
func (w *window) Close()                         { w.ts.finish() }
func (w *window) DarkMode() bool                 { return true } // terminals are conventionally dark
func (w *window) OpenURL(string) error           { return nil }
func (w *window) ClipboardRead() (string, error) { return "", nil }

// ClipboardWrite copies text to the system clipboard via OSC 52, which kitty
// and friends honor even over SSH.
func (w *window) ClipboardWrite(text string) error {
	fmt.Fprintf(w.ts.out, "%c]52;c;%s%c", esc, b64(text), 0x07)
	return nil
}

// frame implements shell.Frame; Target routes the rasterized pixels to present.
type frame struct{ ts *termState }

func (f *frame) Size() geom.Size { return f.ts.logicalSize() }
func (f *frame) Scale() float32 {
	f.ts.mu.Lock()
	rs := f.ts.renderCur
	f.ts.mu.Unlock()
	if rs <= 0 {
		rs = 1
	}
	return rs
}

// The damage rect is ignored: termState.present does its own tile-level diff
// against the last frame it sent, which is finer than a single rect and is what
// makes an update cost bytes proportional to the change rather than the screen.
func (f *frame) Target() shell.Target {
	return shell.PixelTarget{Put: func(img *image.RGBA, _ geom.Rect) { f.ts.present(img) }}
}
