//go:build darwin || linux

package terminal

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/doug/gossamer/shell"
)

// Run presents handler h in the current process's terminal (os.Stdin /
// os.Stdout), blocking until the app exits. It is a convenience wrapper over
// RunTTY that puts the terminal in raw mode, tracks size via the tty ioctls,
// and restores everything on exit.
//
// To serve a gossamer app over SSH instead, implement TTY over the SSH session
// (its channel is the reader/writer, the pty-req carries the size, window-change
// messages drive Resize) and call RunTTY — see the package doc.
//
// Building any gossamer app with -tags gossamer_term routes app.Run here.
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
// for I/O, SIGWINCH for resize, and TIOCGWINSZ for size. Terminating signals
// close stdin so RunTTY's reader sees EOF and unwinds cleanly (restoring the
// terminal via Run's defers).
type localTTY struct {
	fd     int
	resize chan struct{}
	sigs   chan os.Signal
	stop   chan struct{}
}

func newLocalTTY(fd int) *localTTY {
	t := &localTTY{
		fd:     fd,
		resize: make(chan struct{}, 1),
		sigs:   make(chan os.Signal, 1),
		stop:   make(chan struct{}),
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
				// SIGINT/SIGTERM: close stdin → RunTTY's reader hits EOF → exit.
				_ = os.Stdin.Close()
				return
			}
		}
	}()
	return t
}

func (t *localTTY) Read(p []byte) (int, error)  { return os.Stdin.Read(p) }
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
