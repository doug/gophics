//go:build js && wasm

// Web implementation of the shell WebSocket capability (shell/socket.go) using
// the browser's WebSocket object. Binary frames are delivered as ArrayBuffer
// (binaryType set below) rather than Blob, so a message decodes synchronously in
// the onmessage handler without an await. Reuses the package jsToBytes/bytesToJS
// helpers from media_web.go.

package web

import (
	"errors"
	"syscall/js"

	"github.com/doug/gophics/shell"
)

// Socket satisfies shell.SocketWindow for the web shell. The WebSocket API is
// universally available, so this is always non-nil.
func (w *window) Socket() shell.Socket { return webSocket{} }

type webSocket struct{}

func (webSocket) Dial(url string, h shell.SocketHandlers) {
	ws := js.Global().Get("WebSocket").New(url)
	ws.Set("binaryType", "arraybuffer") // deliver binary frames as ArrayBuffer, not Blob

	conn := &webSocketConn{ws: ws}
	var onOpen, onMessage, onClose, onError js.Func
	closed := false
	var dialErr error

	release := func() {
		onOpen.Release()
		onMessage.Release()
		onClose.Release()
		onError.Release()
	}

	onOpen = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		if h.OnOpen != nil {
			h.OnOpen(conn)
		}
		return nil
	})

	onMessage = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if len(args) == 0 {
			return nil
		}
		data := args[0].Get("data")
		if data.Type() == js.TypeString {
			if h.OnText != nil {
				h.OnText(data.String())
			}
			return nil
		}
		// ArrayBuffer (binaryType is "arraybuffer").
		u8 := js.Global().Get("Uint8Array").New(data)
		if h.OnMessage != nil {
			h.OnMessage(jsToBytes(u8))
		}
		return nil
	})

	// A WebSocket "error" is always followed by a "close" (per WHATWG), so we
	// only record the error here and report it once, from the close handler —
	// which also owns releasing the js.Funcs (releasing here would leave the
	// close callback pointing at a freed function).
	onError = js.FuncOf(func(_ js.Value, _ []js.Value) any {
		dialErr = errors.New("websocket error")
		return nil
	})

	onClose = js.FuncOf(func(_ js.Value, args []js.Value) any {
		if closed {
			return nil
		}
		closed = true
		err := dialErr
		if err == nil && len(args) > 0 && !args[0].Get("wasClean").Bool() {
			err = errors.New("websocket closed uncleanly")
		}
		if h.OnClose != nil {
			h.OnClose(err)
		}
		release()
		return nil
	})

	ws.Call("addEventListener", "open", onOpen)
	ws.Call("addEventListener", "message", onMessage)
	ws.Call("addEventListener", "error", onError)
	ws.Call("addEventListener", "close", onClose)
}

type webSocketConn struct{ ws js.Value }

// Send transmits a binary message. bytesToJS copies into a fresh Uint8Array; a
// typed-array view is a valid argument to WebSocket.send.
func (c *webSocketConn) Send(b []byte) { c.ws.Call("send", bytesToJS(b)) }

func (c *webSocketConn) SendText(s string) { c.ws.Call("send", s) }

func (c *webSocketConn) Close() { c.ws.Call("close") }
