package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
)

// serve serves the web build directory over HTTP with no-store headers (so a
// reload always gets the current wasm). If b is non-nil, it also mounts a
// Server-Sent Events endpoint and injects a tiny reload client into
// index.html — the dev live-reload channel.
//
// It binds the requested port, or the next free one if it's taken, so
// `gophics dev -p web` never dies on a busy 8080.
//
// bind is the interface: 127.0.0.1 by default, because this serves a source
// tree's build output and, in dev, a reload endpoint, and neither is something
// to offer the coffee shop's Wi-Fi unasked. Testing on a phone is what -bind
// 0.0.0.0 is for.
func serve(dir, bind string, port int, b *broadcaster) error {
	mux := http.NewServeMux()
	if b != nil {
		mux.HandleFunc("/_gophics/reload", sseHandler(b))
	}
	files := http.FileServer(http.Dir(dir))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		if b != nil && (r.URL.Path == "/" || r.URL.Path == "/index.html") {
			serveIndexWithReload(w, dir)
			return
		}
		files.ServeHTTP(w, r)
	})
	ln, err := listenFrom(bind, port)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: serving %s at %s\n", dir, serveURL(ln.Addr()))
	return http.Serve(ln, mux)
}

// defaultBind is the interface serve uses unless -bind says otherwise.
const defaultBind = "127.0.0.1"

// listenFrom binds bind:port, or the next free port up to port+20.
func listenFrom(bind string, port int) (net.Listener, error) {
	for p := port; p < port+20; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(p)))
		if err == nil {
			if p != port {
				fmt.Fprintf(os.Stderr, "gophics: port %d in use — using %d\n", port, p)
			}
			return ln, nil
		}
	}
	return nil, fmt.Errorf("no free port in %d..%d", port, port+19)
}

// serveURL is the address to print for a bound listener: localhost for the
// wildcard and loopback binds, the interface itself otherwise.
func serveURL(a net.Addr) string {
	host, port, err := net.SplitHostPort(a.String())
	if err != nil {
		return "http://" + a.String() + "/"
	}
	if ip := net.ParseIP(host); ip == nil || ip.IsUnspecified() || ip.IsLoopback() {
		host = "localhost"
	}
	return "http://" + net.JoinHostPort(host, port) + "/"
}
