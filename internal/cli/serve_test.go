package cli

import (
	"net"
	"strings"
	"testing"
)

// The dev server is loopback-only by default: it serves a source tree's build
// output and, in dev, a reload endpoint, which used to be offered to every
// interface while the message said localhost.
func TestServeBindsLoopbackByDefault(t *testing.T) {
	ln, err := listenFrom(defaultBind, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	host, _, err := net.SplitHostPort(ln.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
		t.Errorf("default bind listens on %s, want loopback", host)
	}
	if u := serveURL(ln.Addr()); !strings.HasPrefix(u, "http://localhost:") {
		t.Errorf("serveURL = %q, want a localhost URL", u)
	}
}

func TestServeURLNamesTheInterfaceItBound(t *testing.T) {
	a := &net.TCPAddr{IP: net.ParseIP("192.168.1.20"), Port: 8080}
	if got := serveURL(a); got != "http://192.168.1.20:8080/" {
		t.Errorf("serveURL = %q", got)
	}
	a = &net.TCPAddr{IP: net.IPv4zero, Port: 8080}
	if got := serveURL(a); got != "http://localhost:8080/" {
		t.Errorf("serveURL(0.0.0.0) = %q, want localhost — the wildcard is not a URL anyone can open", got)
	}
}
