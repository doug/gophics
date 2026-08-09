//go:build !js

// Pure-Go, dependency-free RFC 6455 WebSocket client. This is the shell.Socket
// implementation shared by every native target — the desktop shell (macOS,
// Linux, Windows) and the mobile shells (Android/iOS) both return NewSocket()
// from their Window. It speaks the protocol directly over net.Dial/tls.Dial: a
// hand-rolled HTTP Upgrade handshake plus a minimal frame reader/writer with the
// mandatory client-side masking. No CGo, no external module.
//
// Following the capability contract, callbacks are invoked from the background
// read goroutine (Dial returns immediately); the generated PostedSocket wrapper
// marshals them onto the UI goroutine, so nothing here hand-marshals.

package shell

import (
	"bufio"
	"crypto/rand"
	"crypto/sha1"
	"crypto/tls"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"strings"
	"sync"
	"time"
)

// NewSocket returns the pure-Go WebSocket capability for native shells.
func NewSocket() Socket { return socketClient{} }

type socketClient struct{}

func (socketClient) Dial(rawurl string, h SocketHandlers) {
	go func() {
		conn, err := dialWebSocket(rawurl)
		if err != nil {
			if h.OnClose != nil {
				h.OnClose(err)
			}
			return
		}
		if h.OnOpen != nil {
			h.OnOpen(conn)
		}
		conn.readLoop(h)
	}()
}

// WebSocket opcodes (RFC 6455 §5.2).
const (
	opContinuation = 0x0
	opText         = 0x1
	opBinary       = 0x2
	opClose        = 0x8
	opPing         = 0x9
	opPong         = 0xA
)

// wsGUID is the magic value appended to the client key to derive the accept key
// (RFC 6455 §1.3).
const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

// acceptKey computes the expected Sec-WebSocket-Accept for a client key.
func acceptKey(clientKey string) string {
	h := sha1.New()
	io.WriteString(h, clientKey+wsGUID)
	return base64.StdEncoding.EncodeToString(h.Sum(nil))
}

// wsConn is a live connection: a net.Conn plus a buffered reader for framing.
type wsConn struct {
	conn net.Conn
	br   *bufio.Reader

	wmu sync.Mutex // serializes frame writes so they never interleave

	mu        sync.Mutex // guards sentClose/closed
	sentClose bool       // we have sent a close frame (initiator or echo)
	closed    bool       // OnClose has been reported
}

func dialWebSocket(rawurl string) (*wsConn, error) {
	u, err := url.Parse(rawurl)
	if err != nil {
		return nil, err
	}

	var netConn net.Conn
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	switch u.Scheme {
	case "ws":
		host := u.Host
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
		netConn, err = dialer.Dial("tcp", host)
	case "wss":
		host := u.Host
		if u.Port() == "" {
			host = net.JoinHostPort(u.Hostname(), "443")
		}
		netConn, err = tls.DialWithDialer(dialer, "tcp", host, &tls.Config{ServerName: u.Hostname()})
	default:
		return nil, fmt.Errorf("shell: unsupported websocket scheme %q", u.Scheme)
	}
	if err != nil {
		return nil, err
	}

	// Client opening handshake (RFC 6455 §4.1).
	var keyRaw [16]byte
	if _, err := rand.Read(keyRaw[:]); err != nil {
		netConn.Close()
		return nil, err
	}
	key := base64.StdEncoding.EncodeToString(keyRaw[:])

	path := u.RequestURI()
	if path == "" {
		path = "/"
	}
	var req strings.Builder
	fmt.Fprintf(&req, "GET %s HTTP/1.1\r\n", path)
	fmt.Fprintf(&req, "Host: %s\r\n", u.Host)
	req.WriteString("Upgrade: websocket\r\n")
	req.WriteString("Connection: Upgrade\r\n")
	fmt.Fprintf(&req, "Sec-WebSocket-Key: %s\r\n", key)
	req.WriteString("Sec-WebSocket-Version: 13\r\n\r\n")
	if _, err := io.WriteString(netConn, req.String()); err != nil {
		netConn.Close()
		return nil, err
	}

	br := bufio.NewReader(netConn)
	statusLine, err := br.ReadString('\n')
	if err != nil {
		netConn.Close()
		return nil, err
	}
	if !strings.HasPrefix(statusLine, "HTTP/1.1 101") {
		netConn.Close()
		return nil, fmt.Errorf("shell: websocket handshake failed: %s", strings.TrimSpace(statusLine))
	}
	var accept string
	for {
		line, err := br.ReadString('\n')
		if err != nil {
			netConn.Close()
			return nil, err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			break // end of headers
		}
		if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "Sec-WebSocket-Accept") {
			accept = strings.TrimSpace(v)
		}
	}
	if accept != acceptKey(key) {
		netConn.Close()
		return nil, errors.New("shell: websocket handshake: bad Sec-WebSocket-Accept")
	}
	return &wsConn{conn: netConn, br: br}, nil
}

// --- SocketConn -------------------------------------------------------------

func (c *wsConn) Send(b []byte)     { c.writeFrame(opBinary, b) }
func (c *wsConn) SendText(s string) { c.writeFrame(opText, []byte(s)) }

// Close starts a clean close handshake by sending a close frame. The read loop
// observes the peer's echo (or the socket teardown) and reports OnClose(nil).
func (c *wsConn) Close() { c.writeClose() }

// writeClose sends a single close frame (status 1000, normal). Idempotent: only
// the first call for a connection emits a frame.
func (c *wsConn) writeClose() {
	c.mu.Lock()
	if c.sentClose {
		c.mu.Unlock()
		return
	}
	c.sentClose = true
	c.mu.Unlock()
	c.writeFrame(opClose, []byte{0x03, 0xe8}) // 1000
}

// writeFrame writes one masked client frame (all client→server frames MUST be
// masked, RFC 6455 §5.3). FIN is always set — the client never fragments.
func (c *wsConn) writeFrame(opcode byte, payload []byte) error {
	c.wmu.Lock()
	defer c.wmu.Unlock()

	var header [14]byte
	header[0] = 0x80 | opcode // FIN + opcode
	n := len(payload)
	hlen := 2
	switch {
	case n < 126:
		header[1] = 0x80 | byte(n) // mask bit + length
	case n < 65536:
		header[1] = 0x80 | 126
		binary.BigEndian.PutUint16(header[2:], uint16(n))
		hlen = 4
	default:
		header[1] = 0x80 | 127
		binary.BigEndian.PutUint64(header[2:], uint64(n))
		hlen = 10
	}

	var mask [4]byte
	if _, err := rand.Read(mask[:]); err != nil {
		return err
	}
	copy(header[hlen:], mask[:])
	hlen += 4

	if _, err := c.conn.Write(header[:hlen]); err != nil {
		return err
	}
	if n == 0 {
		return nil
	}
	masked := make([]byte, n)
	for i := 0; i < n; i++ {
		masked[i] = payload[i] ^ mask[i&3]
	}
	_, err := c.conn.Write(masked)
	return err
}

// --- read loop --------------------------------------------------------------

func (c *wsConn) readLoop(h SocketHandlers) {
	report := func(err error) {
		c.mu.Lock()
		already := c.closed
		c.closed = true
		c.mu.Unlock()
		if already {
			return
		}
		c.conn.Close()
		if h.OnClose != nil {
			h.OnClose(err)
		}
	}

	var fragOp byte
	var frag []byte
	for {
		fin, opcode, payload, err := c.readFrame()
		if err != nil {
			c.mu.Lock()
			initiated := c.sentClose
			c.mu.Unlock()
			// A socket teardown after we've entered the close handshake is the
			// expected clean end (many servers just drop TCP after echoing close).
			if initiated && (errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) || errors.Is(err, net.ErrClosed)) {
				report(nil)
			} else {
				report(err)
			}
			return
		}

		switch opcode {
		case opPing:
			c.writeFrame(opPong, payload)
		case opPong:
			// no-op
		case opClose:
			c.writeClose() // echo (no-op if we initiated)
			report(nil)
			return
		case opText, opBinary:
			if !fin {
				fragOp = opcode
				frag = append(frag[:0], payload...)
				continue
			}
			deliver(h, opcode, payload)
		case opContinuation:
			frag = append(frag, payload...)
			if fin {
				deliver(h, fragOp, frag)
				frag = nil
			}
		}
	}
}

func deliver(h SocketHandlers, opcode byte, payload []byte) {
	switch opcode {
	case opText:
		if h.OnText != nil {
			h.OnText(string(payload))
		}
	case opBinary:
		if h.OnMessage != nil {
			h.OnMessage(payload)
		}
	}
}

// readFrame reads a single frame. It tolerates a mask bit (unmasking as needed)
// so the same reader also serves the test server, though server→client frames
// are unmasked per spec.
func (c *wsConn) readFrame() (fin bool, opcode byte, payload []byte, err error) {
	var head [2]byte
	if _, err = io.ReadFull(c.br, head[:]); err != nil {
		return
	}
	fin = head[0]&0x80 != 0
	opcode = head[0] & 0x0f
	masked := head[1]&0x80 != 0
	n := int(head[1] & 0x7f)
	switch n {
	case 126:
		var ext [2]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		n = int(binary.BigEndian.Uint16(ext[:]))
	case 127:
		var ext [8]byte
		if _, err = io.ReadFull(c.br, ext[:]); err != nil {
			return
		}
		n = int(binary.BigEndian.Uint64(ext[:]))
	}

	var mask [4]byte
	if masked {
		if _, err = io.ReadFull(c.br, mask[:]); err != nil {
			return
		}
	}
	payload = make([]byte, n)
	if _, err = io.ReadFull(c.br, payload); err != nil {
		return
	}
	if masked {
		for i := range payload {
			payload[i] ^= mask[i&3]
		}
	}
	return
}
