//go:build !js

package shell

import (
	"bytes"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// TestSocketClientRoundTrip stands up an httptest server that upgrades to
// WebSocket (a ~30-line inline server handshake + echo loop) and verifies the
// pure-Go client (NewSocket) round-trips a text message, a binary message, and
// performs a clean close. It exercises the real handshake and framing over a
// live TCP connection — no browser, no external dependency.
func TestSocketClientRoundTrip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(wsEchoServer(t)))
	defer srv.Close()
	wsURL := "ws" + strings.TrimPrefix(srv.URL, "http")

	type msg struct {
		text  string
		bin   []byte
		isBin bool
	}
	msgs := make(chan msg, 4)
	opened := make(chan SocketConn, 1)
	closed := make(chan error, 1)

	NewSocket().Dial(wsURL, SocketHandlers{
		OnOpen:    func(c SocketConn) { opened <- c },
		OnText:    func(s string) { msgs <- msg{text: s} },
		OnMessage: func(b []byte) { msgs <- msg{bin: b, isBin: true} },
		OnClose:   func(err error) { closed <- err },
	})

	var conn SocketConn
	select {
	case conn = <-opened:
	case err := <-closed:
		t.Fatalf("dial failed before open: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for OnOpen")
	}

	conn.SendText("hello, socket")
	binPayload := []byte{0x00, 0x01, 0x02, 0xff, 0x7f, 0x80}
	conn.Send(binPayload)

	// The echo server returns each message in order.
	first := recvMsg(t, msgs)
	if first.isBin || first.text != "hello, socket" {
		t.Errorf("first echo = %+v, want text %q", first, "hello, socket")
	}
	second := recvMsg(t, msgs)
	if !second.isBin || !bytes.Equal(second.bin, binPayload) {
		t.Errorf("second echo = %+v, want binary %v", second, binPayload)
	}

	conn.Close()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("clean close reported error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for OnClose")
	}
}

func recvMsg[T any](t *testing.T, ch <-chan T) T {
	t.Helper()
	select {
	case v := <-ch:
		return v
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for message")
		var zero T
		return zero
	}
}

// wsEchoServer returns an http.Handler that completes the server side of the
// WebSocket opening handshake by hijacking the connection, then echoes every
// data frame back and honors a close frame. It reuses the client's framing
// helpers (wsConn.readFrame, acceptKey, the op* constants) since the test is in
// package shell; server→client frames are written unmasked per RFC 6455 §5.3.
func wsEchoServer(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		key := r.Header.Get("Sec-WebSocket-Key")
		if key == "" {
			t.Error("server: missing Sec-WebSocket-Key")
			http.Error(w, "bad upgrade", http.StatusBadRequest)
			return
		}
		hj, ok := w.(http.Hijacker)
		if !ok {
			t.Error("server: ResponseWriter is not a Hijacker")
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			t.Errorf("server: hijack: %v", err)
			return
		}
		defer conn.Close()

		resp := "HTTP/1.1 101 Switching Protocols\r\n" +
			"Upgrade: websocket\r\n" +
			"Connection: Upgrade\r\n" +
			"Sec-WebSocket-Accept: " + acceptKey(key) + "\r\n\r\n"
		if _, err := io.WriteString(conn, resp); err != nil {
			return
		}

		sc := &wsConn{conn: conn, br: buf.Reader}
		for {
			_, opcode, payload, err := sc.readFrame()
			if err != nil {
				return
			}
			switch opcode {
			case opClose:
				writeServerFrame(conn, opClose, payload)
				return
			case opPing:
				writeServerFrame(conn, opPong, payload)
			case opText, opBinary:
				writeServerFrame(conn, opcode, payload)
			}
		}
	}
}

// writeServerFrame writes one unmasked frame (server→client), FIN set.
func writeServerFrame(w io.Writer, opcode byte, payload []byte) {
	var header [10]byte
	header[0] = 0x80 | opcode
	n := len(payload)
	hlen := 2
	switch {
	case n < 126:
		header[1] = byte(n)
	case n < 65536:
		header[1] = 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
		hlen = 4
	default:
		header[1] = 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
		hlen = 10
	}
	w.Write(header[:hlen])
	w.Write(payload)
}

// compile-time guard: the pure-Go client implements the capability interfaces.
var (
	_ Socket     = socketClient{}
	_ SocketConn = (*wsConn)(nil)
)
