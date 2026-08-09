package shell

// WebSocket capability: a bidirectional message socket to a ws:// or wss:// URL.
// A Window opts in by implementing SocketWindow; widgets reach it via
// ctx.Socket(), nil where unsupported. Unlike most capabilities this one is
// genuinely exercised end-to-end off the browser: the web shell wraps the
// browser WebSocket object, and the desktop/mobile shells share a dependency-
// free, pure-Go RFC 6455 client (shell/socket_client.go, build tag !js) — no
// CGo, no external module.
//
// All handler callbacks fire on the UI goroutine. The generated PostedSocket
// wrapper marshals them through Owner.Post, so a platform implementation may
// invoke them from any goroutine (a browser event, a background frame-reader)
// without hand-marshaling. The SocketConn handed to OnOpen carries no callbacks
// of its own (Send/SendText/Close only), so it is used directly and needs no
// wrapping.

// SocketWindow is implemented by a Window that can open a WebSocket. The app
// runner type-asserts the Window to it and publishes Socket() to the widget tree
// — the same shape as FilePickerWindow/ShareWindow.
type SocketWindow interface {
	Socket() Socket
}

// Socket opens WebSocket connections.
type Socket interface {
	// Dial opens a connection to url (ws:// or wss://) and drives it through h.
	// It returns immediately; the outcome is reported asynchronously — h.OnOpen
	// on success (with the live conn), or h.OnClose with a non-nil err on a dial
	// or handshake failure (in which case OnOpen never fires). Every h callback
	// fires on the UI goroutine.
	Dial(url string, h SocketHandlers)
}

// SocketConn is a live connection, handed to SocketHandlers.OnOpen. Its methods
// are safe to call from the UI goroutine; sends are written in order. It carries
// no callbacks of its own — inbound data arrives through the SocketHandlers the
// connection was dialed with.
type SocketConn interface {
	// Send transmits one binary message.
	Send([]byte)
	// SendText transmits one UTF-8 text message.
	SendText(string)
	// Close initiates a clean close handshake. OnClose fires once it completes.
	Close()
}

// SocketHandlers are the lifecycle callbacks for a dialed connection. Any field
// may be nil. All fire on the UI goroutine (see PostedSocket).
type SocketHandlers struct {
	// OnOpen fires once, when the connection is established, with the live conn.
	OnOpen func(SocketConn)
	// OnMessage fires for each inbound binary message.
	OnMessage func([]byte)
	// OnText fires for each inbound text (UTF-8) message.
	OnText func(string)
	// OnClose fires exactly once when the connection ends: err is nil on a clean
	// close, non-nil on a transport/handshake error (including a dial failure,
	// where OnOpen never fired).
	OnClose func(err error)
}
