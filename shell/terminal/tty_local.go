//go:build darwin || linux

package terminal

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/doug/gophics/shell"
)

// Run presents handler h in the current process's terminal (os.Stdin /
// os.Stdout), blocking until the app exits. It is a convenience wrapper over
// RunTTY that puts the terminal in raw mode, tracks size via the tty ioctls,
// and restores everything on exit.
//
// To serve a gophics app over SSH instead, implement TTY over the SSH session
// (its channel is the reader/writer, the pty-req carries the size, window-change
// messages drive Resize) and call RunTTY — see the package doc.
//
// Building any gophics app with -tags gophics_term routes app.Run here.
func Run(h shell.Handler, cfg shell.Config) (err error) {
	inFD := int(os.Stdin.Fd())
	if !isatty(inFD) {
		return errNotATerminal
	}
	restore, err := enterRaw(inFD)
	if err != nil {
		return fmt.Errorf("terminal: raw mode: %w", err)
	}
	defer func() {
		if rerr := restore(); rerr != nil && err == nil {
			err = rerr
		}
	}()

	tty := newLocalTTY(inFD)
	defer tty.close()
	return RunTTY(h, cfg, tty)
}

// localTTY adapts the process terminal to the TTY interface: os.Stdin/os.Stdout
// for I/O, SIGWINCH for resize, and TIOCGWINSZ for size. A terminating signal
// ends the input stream (Read returns io.EOF) so RunTTY unwinds cleanly and
// Run's defers restore the terminal.
//
// Stdin is read on its own goroutine and handed over through a channel, so
// that the signal can end the stream without touching the descriptor. The
// obvious alternative — closing os.Stdin to wake the reader — does not work:
// stdin is a blocking descriptor, so Close returns at once while the pending
// Read keeps it open, and the signal has no effect until the next keypress.
// When that key finally arrived the descriptor really closed, Read failed,
// RunTTY returned, and restore() ran its ioctl on a closed fd 0 — leaving the
// user's shell in raw mode, with no echo and no line editing.
type localTTY struct {
	fd     int
	resize chan struct{}
	sigs   chan os.Signal
	stop   chan struct{} // closed by close: the signal goroutine exits
	quit   chan struct{} // closed on SIGINT/SIGTERM: Read reports EOF
	chunks chan []byte   // what the stdin goroutine has read
	rerr   error         // the stdin goroutine's terminal error, once chunks is closed
	buf    []byte        // unread remainder of the last chunk
}

func newLocalTTY(fd int) *localTTY {
	t := &localTTY{
		fd:     fd,
		resize: make(chan struct{}, 1),
		sigs:   make(chan os.Signal, 1),
		stop:   make(chan struct{}),
		quit:   make(chan struct{}),
		chunks: make(chan []byte),
	}
	signal.Notify(t.sigs, syscall.SIGWINCH, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		for {
			select {
			case <-t.stop:
				return
			case s := <-t.sigs:
				if s == syscall.SIGWINCH {
					select {
					case t.resize <- struct{}{}:
					default:
					}
					continue
				}
				// SIGINT/SIGTERM: end the input stream → RunTTY's reader
				// sees EOF → the app gets Closed and exits.
				close(t.quit)
				return
			}
		}
	}()
	go func() {
		defer close(t.chunks)
		for {
			b := make([]byte, 4096)
			n, err := os.Stdin.Read(b)
			if n > 0 {
				select {
				case t.chunks <- b[:n]:
				case <-t.stop:
					return
				}
			}
			if err != nil {
				t.rerr = err
				return
			}
		}
	}()
	return t
}

// Read hands out what the stdin goroutine has read, or io.EOF once a
// terminating signal has arrived. A read blocked in the kernel is left to
// finish on its own; nothing waits for it.
func (t *localTTY) Read(p []byte) (int, error) {
	if len(t.buf) == 0 {
		select {
		case <-t.quit:
			return 0, io.EOF
		case b, ok := <-t.chunks:
			if !ok {
				if t.rerr != nil {
					return 0, t.rerr
				}
				return 0, io.EOF
			}
			t.buf = b
		}
	}
	n := copy(p, t.buf)
	t.buf = t.buf[n:]
	return n, nil
}

func (t *localTTY) Write(p []byte) (int, error) { return os.Stdout.Write(p) }
func (t *localTTY) Resize() <-chan struct{}     { return t.resize }

// TempDir satisfies FileTransport: the local terminal shares the filesystem, so
// pixels can be handed over via a temp file (kitty t=t) instead of inline.
func (t *localTTY) TempDir() string { return os.TempDir() }

func (t *localTTY) Size() (int, int) {
	cols, rows, x, y, err := winsize(t.fd)
	if err != nil {
		return 0, 0
	}
	if x == 0 || y == 0 {
		return cols * 8, rows * 16 // terminal didn't report pixels; estimate
	}
	return x, y
}

func (t *localTTY) close() {
	signal.Stop(t.sigs)
	close(t.stop)
}

// CellGrid satisfies terminal.CellGrid: the terminal's column/row count, used
// to scale one image to fill the whole terminal.
func (t *localTTY) CellGrid() (cols, rows int) {
	c, r, _, _, err := winsize(t.fd)
	if err != nil {
		return 0, 0
	}
	return c, r
}
