//go:build !js

package shell

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
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

// wsEchoServer returns an http.Gestures that completes the server side of the
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

// wsServer completes the server side of the handshake and hands the raw
// connection to body, for tests that need a peer that misbehaves.
func wsServer(t *testing.T, body func(conn net.Conn, br *bufio.Reader)) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			"Sec-WebSocket-Accept: " + acceptKey(r.Header.Get("Sec-WebSocket-Key")) + "\r\n\r\n"
		if _, err := io.WriteString(conn, resp); err != nil {
			return
		}
		body(conn, buf.Reader)
	}))
}

// dialAndOpen dials srv and waits for OnOpen, returning the conn and the
// channel OnClose reports on.
func dialAndOpen(t *testing.T, srv *httptest.Server) (SocketConn, <-chan error) {
	t.Helper()
	opened := make(chan SocketConn, 1)
	closed := make(chan error, 1)
	NewSocket().Dial("ws"+strings.TrimPrefix(srv.URL, "http"), SocketHandlers{
		OnOpen:  func(c SocketConn) { opened <- c },
		OnClose: func(err error) { closed <- err },
	})
	select {
	case c := <-opened:
		return c, closed
	case err := <-closed:
		t.Fatalf("dial failed before open: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for OnOpen")
	}
	return nil, nil
}

// A frame header's 64-bit length was trusted as given. With the high bit set
// it became a negative int and make panicked — one frame from a bad or
// hostile server crashed the app; merely large, it was allocated in full.
// Both are now a protocol error reported through OnClose.
func TestOversizedFrameIsReportedNotAllocated(t *testing.T) {
	for _, tc := range []struct {
		name   string
		length uint64
	}{
		{"high bit set", 1 << 63},
		{"just past the cap", maxMessageSize + 1},
		{"absurd", ^uint64(0)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sent := make(chan struct{})
			srv := wsServer(t, func(conn net.Conn, _ *bufio.Reader) {
				var header [10]byte
				header[0] = 0x80 | opBinary
				header[1] = 127
				binary.BigEndian.PutUint64(header[2:], tc.length)
				conn.Write(header[:])
				<-sent // hold the connection open so EOF cannot be the reason
			})
			defer srv.Close()
			defer close(sent)

			_, closed := dialAndOpen(t, srv)
			select {
			case err := <-closed:
				if !errors.Is(err, errMessageTooLarge) {
					t.Errorf("OnClose reported %v, want %v", err, errMessageTooLarge)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("OnClose never fired for an oversized frame")
			}
		})
	}
}

// A message assembled from fragments is bounded the same way, or the cap is
// only a cap on one frame.
func TestOversizedFragmentedMessageIsReported(t *testing.T) {
	sent := make(chan struct{})
	srv := wsServer(t, func(conn net.Conn, _ *bufio.Reader) {
		chunk := make([]byte, 1<<20)
		// First fragment: FIN clear, binary. Then continuations until the
		// total passes the cap.
		writeRawFrame(conn, 0, opBinary, chunk)
		for i := 0; i <= maxMessageSize/len(chunk); i++ {
			writeRawFrame(conn, 0, opContinuation, chunk)
		}
		<-sent
	})
	defer srv.Close()
	defer close(sent)

	_, closed := dialAndOpen(t, srv)
	select {
	case err := <-closed:
		if !errors.Is(err, errMessageTooLarge) {
			t.Errorf("OnClose reported %v, want %v", err, errMessageTooLarge)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("OnClose never fired for an oversized fragmented message")
	}
}

// writeRawFrame writes one unmasked frame with the FIN bit as given.
func writeRawFrame(w io.Writer, fin byte, opcode byte, payload []byte) {
	var header [10]byte
	header[0] = fin | opcode
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

// Close sent the close frame and then waited for the peer to answer, with no
// bound. A peer that never echoes and never drops TCP left the read loop
// blocked forever, OnClose never fired, and the connection was never
// released.
func TestCloseCompletesWhenThePeerNeverAnswers(t *testing.T) {
	old := closeTimeout
	closeTimeout = 300 * time.Millisecond
	t.Cleanup(func() { closeTimeout = old })

	release := make(chan struct{})
	srv := wsServer(t, func(conn net.Conn, br *bufio.Reader) {
		// Read the client's close frame and ignore it; keep TCP up.
		sc := &wsConn{conn: conn, br: br}
		sc.readFrame()
		<-release
	})
	defer srv.Close()
	defer close(release)

	conn, closed := dialAndOpen(t, srv)
	began := time.Now()
	conn.Close()
	select {
	case err := <-closed:
		if err != nil {
			t.Errorf("a close the caller asked for reported %v", err)
		}
		if took := time.Since(began); took > 3*time.Second {
			t.Errorf("OnClose took %v after Close", took)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("OnClose never fired: the read loop is still waiting for an echo that is not coming")
	}
}

// compile-time guard: the pure-Go client implements the capability interfaces.
var (
	_ Socket     = socketClient{}
	_ SocketConn = (*wsConn)(nil)
)
