//go:build !js

package mobile

import "github.com/doug/gophics/shell"

// Socket makes the Bridge a shell.SocketWindow. It reuses the pure-Go RFC 6455
// client in package shell (shared with the desktop shell) — a raw socket behaves
// the same on device as on desktop, so no host bridge or native code is needed.
func (b *Bridge) Socket() shell.Socket { return shell.NewSocket() }
