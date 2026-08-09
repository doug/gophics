//go:build !js

package desktop

import "github.com/doug/gophics/shell"

// Socket makes the desktop window a shell.SocketWindow. It uses the pure-Go
// RFC 6455 client in package shell (shell/socket_client.go) — the same client
// the mobile shells use — so WebSockets work on macOS, Linux, and Windows with
// no CGo and no external dependency.
func (w *window) Socket() shell.Socket { return shell.NewSocket() }
