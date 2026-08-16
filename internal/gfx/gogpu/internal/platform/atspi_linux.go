//go:build linux

// AT-SPI groundwork: reaching the accessibility bus.
//
// This is the first step of a Linux screen-reader bridge, and the step that
// decides whether the rest is worth attempting. AT-SPI is not a C API — it is a
// D-Bus protocol — so a pure-Go implementation needs no bindings, only a D-Bus
// peer. dbus_linux.go already provides most of one: connect, SASL EXTERNAL
// auth, Hello, the full type marshaller and a decoder.
//
// The accessibility bus is a *second* bus, separate from the session bus. Its
// address is published by org.a11y.Bus on the session bus, which is what
// a11yBusAddress asks for. A machine with no assistive technology configured
// has no such service, and that is the honest "no bridge here" answer rather
// than an error worth surfacing.
//
// # What is still missing
//
// Everything so far — including this file — treats D-Bus as a client: send a
// call, wait for the reply. AT-SPI inverts that. The application is a *server*:
// it exports an object per accessible node and answers method calls that the
// screen reader makes on its own schedule (GetChildAtIndex, GetRole, GetExtents
// and so on), the same pull model AppKit uses. Reaching that point needs, on
// top of what exists:
//
//   - an inbound dispatch loop, rather than waitResponse's "discard everything
//     that is not my reply";
//   - METHOD_RETURN and ERROR replies (the encoder already takes a message
//     type, so this is wiring, not new wire format);
//   - signal emission for focus and state changes;
//   - org.freedesktop.DBus.Introspectable, which screen readers call first;
//   - the Accessible, Component and Action interfaces over shell.A11yNode.
//
// None of that needs CGo, and none of it needs a new transport. That is the
// spike's finding: the remaining work is protocol surface, not feasibility.

package platform

import (
	"fmt"
	"os"
	"time"
)

// a11yBusAddress asks the session bus where the accessibility bus lives.
//
// Returns an empty address (and no error) when the machine simply has no
// accessibility bus — no org.a11y.Bus service, or no session bus at all, which
// is the common case on a server or in a container. Callers treat that as "no
// bridge available" and publish nothing, which is what makes
// ctx.Accessibility() correctly nil there.
func a11yBusAddress() (string, error) {
	if os.Getenv("DBUS_SESSION_BUS_ADDRESS") == "" {
		return "", nil
	}
	c, err := dbusConnect()
	if err != nil {
		return "", nil // no session bus: not an error, just no a11y here
	}
	defer c.rw.Close()

	c.rw.SetDeadline(time.Now().Add(5 * time.Second))
	defer c.rw.SetDeadline(time.Time{})

	serial, err := c.sendCall(
		"org.a11y.Bus",
		"/org/a11y/bus",
		"org.a11y.Bus",
		"GetAddress",
		"", nil,
	)
	if err != nil {
		return "", fmt.Errorf("atspi: GetAddress: %w", err)
	}
	for {
		msg, err := c.readMsg()
		if err != nil {
			return "", fmt.Errorf("atspi: read: %w", err)
		}
		if msg.ReplyTo != serial {
			continue
		}
		if msg.Type == dbusMsgError {
			// No org.a11y.Bus on this machine — accessibility is not running.
			return "", nil
		}
		if msg.Type == dbusMsgReturn {
			d := newMsgDecoder(msg.Body, 0)
			return d.readStr()
		}
	}
}

// a11yBusReachable reports whether the accessibility bus can be reached and
// spoken to: the address resolves, the socket accepts a connection, and the
// SASL + Hello handshake completes. This is the precondition for any AT-SPI
// bridge, and is cheap enough to probe at startup.
func a11yBusReachable() bool {
	addr, err := a11yBusAddress()
	if err != nil || addr == "" {
		return false
	}
	raw, err := dbusDialAddr(addr)
	if err != nil {
		return false
	}
	defer raw.Close()

	c := &dbusConn{rw: raw}
	c.rw.SetDeadline(time.Now().Add(5 * time.Second))
	if err := c.auth(); err != nil {
		return false
	}
	name, err := c.hello()
	return err == nil && name != ""
}
