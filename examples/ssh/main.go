// Command ssh serves a gophics app over SSH, rendered with the kitty graphics
// protocol. It shows that gophics's terminal backend is transport-agnostic:
// the SSH server (here gliderlabs/ssh; charmbracelet/wish works identically) is
// the caller's concern, and gophics only needs the session's byte stream, its
// size, and resize notifications — adapted to terminal.TTY below.
//
// Run:  go run .        then, from a kitty-graphics terminal:  ssh -p 2222 localhost
// Quit the app with Ctrl-Q; disconnect ends the session cleanly.
//
// gophics core has no SSH dependency — this lives in its own module.
package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"log"
	"math"
	"sync"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gophics/app"
	"github.com/doug/gophics/geom"
	"github.com/doug/gophics/paint"
	gsh "github.com/doug/gophics/shell"
	"github.com/doug/gophics/shell/terminal"
	"github.com/doug/gophics/widget"
)

func main() {
	signer, err := ephemeralHostKey()
	if err != nil {
		log.Fatal(err)
	}
	srv := &ssh.Server{Addr: ":2222", Handler: handleSession}
	srv.AddHostKey(signer)
	log.Println("gophics SSH demo on :2222 — connect with:  ssh -p 2222 localhost")
	log.Fatal(srv.ListenAndServe())
}

// handleSession runs one gophics app instance for the life of an SSH
// connection. Each client gets its own widget tree and event loop, so per-user
// state is isolated for free.
func handleSession(s ssh.Session) {
	if _, _, ok := s.Pty(); !ok {
		io.WriteString(s, "this demo needs a PTY — connect with: ssh -t -p 2222 localhost\r\n")
		return
	}
	h, err := app.NewHandler(&demo{}, app.Config{
		Size: geom.Size{W: 900, H: 600},
		Font: goregular.TTF,
	})
	if err != nil {
		log.Printf("ssh session: %v", err)
		return
	}
	// RunTTY blocks until the client disconnects (session EOF) or hits Ctrl-Q.
	if err := terminal.RunTTY(h, gsh.Config{Title: "gophics over ssh"}, newSSHTTY(s)); err != nil {
		log.Printf("ssh session: %v", err)
	}
}

// --- sshTTY: adapt an ssh.Session to terminal.TTY ---------------------------

// sshTTY makes an ssh.Session (itself an io.Reader+io.Writer) drive the terminal
// backend. It intentionally does NOT implement terminal.FileTransport, so the
// backend transfers pixels inline as base64 over the encrypted channel rather
// than via a temp file — the correct choice for a remote client.
type sshTTY struct {
	ssh.Session
	resize       chan struct{}
	cellW, cellH int

	mu         sync.Mutex
	cols, rows int
}

func newSSHTTY(s ssh.Session) *sshTTY {
	pty, winCh, _ := s.Pty()
	t := &sshTTY{
		Session: s,
		resize:  make(chan struct{}, 1),
		cellW:   10, cellH: 20, // SSH reports size in cells; assume a cell size
		cols: pty.Window.Width, rows: pty.Window.Height,
	}
	go func() {
		for w := range winCh { // SSH window-change messages
			t.mu.Lock()
			t.cols, t.rows = w.Width, w.Height
			t.mu.Unlock()
			select {
			case t.resize <- struct{}{}:
			default:
			}
		}
	}()
	return t
}

func (t *sshTTY) Size() (int, int) {
	t.mu.Lock()
	defer t.mu.Unlock()
	// The SSH pty-req carries size in CELLS, so estimate pixels from a cell
	// size. For exact pixels, query the client with CSI 14 t and parse the
	// reply out of the session input.
	return t.cols * t.cellW, t.rows * t.cellH
}

func (t *sshTTY) Resize() <-chan struct{} { return t.resize }

func ephemeralHostKey() (gossh.Signer, error) {
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	return gossh.NewSignerFromKey(priv)
}

// --- demo app ---------------------------------------------------------------

// demo is a small interactive gophics app: an animated pulse field plus a dot
// that tracks the pointer — enough to show rendering, animation, and mouse
// input all working over SSH.
type demo struct{}

func (*demo) CreateState() widget.State { return &demoState{} }

type demoState struct {
	widget.StateBase[*demo]
	t     float64
	ptr   geom.Pt
	moved bool
}

func (s *demoState) Init(ctx widget.Ctx) { ctx.AddTicker(s) }

func (s *demoState) Tick(dt float64) bool {
	s.SetState(func() { s.t += dt })
	return true
}

var (
	colBg   = paint.RGB(0.03, 0.04, 0.08)
	colA    = paint.RGB(0.20, 0.85, 0.90)
	colB    = paint.RGB(0.90, 0.30, 0.55)
	colText = paint.RGB(0.92, 0.94, 0.97)
	colDim  = paint.RGB(0.45, 0.50, 0.62)
)

func (s *demoState) Build(widget.Ctx) widget.Widget {
	t, ptr, moved := s.t, s.ptr, s.moved
	return widget.Interactive{
		Handler: widget.Handler{
			OnPress: func(p geom.Pt) { s.SetState(func() { s.ptr, s.moved = p, true }) },
			OnDrag:  func(p, _ geom.Pt) { s.SetState(func() { s.ptr, s.moved = p, true }) },
		},
		Child: widget.Canvas{Clip: true, Draw: func(c paint.Canvas, size geom.Size) {
			c.Clear(colBg)
			cx, cy := size.W/2, size.H/2
			const step = 30
			for gy := float32(step) / 2; gy < size.H; gy += step {
				for gx := float32(step) / 2; gx < size.W; gx += step {
					d := math.Hypot(float64(gx-cx), float64(gy-cy))
					m := float32(math.Sin(d*0.03-t*2.5)*0.5 + 0.5)
					r := 1.5 + m*5
					col := paint.Lerp(colB, colA, m).WithAlpha(0.25 + 0.75*m)
					c.FillRRect(geom.RectXYWH(gx-r, gy-r, r*2, r*2), r, col)
				}
			}
			if moved {
				c.FillRRect(geom.RectXYWH(ptr.X-9, ptr.Y-9, 18, 18), 9, colText)
			}
			c.Text("gophics over ssh", geom.Pt{X: 28, Y: 50}, 30, colText)
			c.Text("move the mouse · Ctrl-Q to quit", geom.Pt{X: 28, Y: 76}, 14, colDim)
		}},
	}
}
