package terminal

// Serving a gossamer app over SSH.
//
// gossamer bundles no SSH server — that is deliberately out of scope. SSH is a
// security-sensitive domain (auth, host keys, session and PTY negotiation) that
// charmbracelet/wish and gliderlabs/ssh already handle well. gossamer's job is
// only to render and take input, so the terminal backend is transport-agnostic:
// adapt an SSH session to the TTY interface and hand it to RunTTY. gossamer's
// go.mod stays free of any SSH dependency; you add wish or gliderlabs/ssh to
// your own program.
//
// A complete adapter over gliderlabs/ssh (wish is the same — a wish.Middleware
// receives the identical *ssh.Session):
//
//	import (
//	    "sync"
//	    "github.com/gliderlabs/ssh"
//	    "github.com/doug/gossamer/app"
//	    "github.com/doug/gossamer/shell"
//	    "github.com/doug/gossamer/shell/terminal"
//	)
//
//	// sshTTY adapts an ssh.Session (itself an io.Reader+io.Writer) to terminal.TTY.
//	type sshTTY struct {
//	    ssh.Session
//	    resize       chan struct{}
//	    cellW, cellH int
//	    mu           sync.Mutex
//	    cols, rows   int
//	}
//
//	func newSSHTTY(s ssh.Session) *sshTTY {
//	    pty, winCh, _ := s.Pty()
//	    t := &sshTTY{
//	        Session: s,
//	        resize:  make(chan struct{}, 1),
//	        cellW:   10, cellH: 20, // assumed cell size; see the caveat below
//	        cols:    pty.Window.Width, rows: pty.Window.Height,
//	    }
//	    go func() {
//	        for w := range winCh { // SSH window-change messages
//	            t.mu.Lock()
//	            t.cols, t.rows = w.Width, w.Height
//	            t.mu.Unlock()
//	            select {
//	            case t.resize <- struct{}{}:
//	            default:
//	            }
//	        }
//	    }()
//	    return t
//	}
//
//	func (t *sshTTY) Size() (int, int) {
//	    t.mu.Lock()
//	    defer t.mu.Unlock()
//	    // SSH's pty-req reports size in CELLS, so estimate pixels from a cell
//	    // size. For exact pixels, query the client with CSI 14 t and parse the
//	    // reply from the session input instead.
//	    return t.cols * t.cellW, t.rows * t.cellH
//	}
//
//	func (t *sshTTY) Resize() <-chan struct{} { return t.resize }
//
//	func serve(root any /* your root widget */) error {
//	    return ssh.ListenAndServe(":2222", func(s ssh.Session) {
//	        if _, _, ok := s.Pty(); !ok {
//	            io.WriteString(s, "a pty (ssh -t) is required\n")
//	            return
//	        }
//	        h, err := app.NewHandler(root, app.Config{ /* Size, Font, ... */ })
//	        if err != nil {
//	            return
//	        }
//	        // One app instance per connection; RunTTY blocks until the client
//	        // disconnects (the session reaching EOF ends it cleanly).
//	        _ = terminal.RunTTY(h, shell.Config{Title: "my app"}, newSSHTTY(s))
//	    }, ssh.HostKeyFile("/path/to/host_key"))
//	}
//
// Notes:
//   - One app instance and one RunTTY per connection: each client gets its own
//     widget tree and event loop, so per-session state is naturally isolated.
//   - The client's terminal must support the kitty graphics protocol; that is a
//     property of the user's local terminal, not the server.
//   - Pixel size is the one rough edge over SSH (cells, not pixels) — estimate,
//     or query with CSI 14 t as noted above.
