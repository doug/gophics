package cli

import (
	"fmt"
	"net"
	"net/http"
	"os"
)

// serve serves the web build directory over HTTP with no-store headers (so a
// reload always gets the current wasm). If b is non-nil, it also mounts a
// Server-Sent Events endpoint and injects a tiny reload client into
// index.html — the dev live-reload channel.
//
// It binds the requested port, or the next free one if it's taken, so
// `gophics dev -p web` never dies on a busy 8080.
func serve(dir string, port int, b *broadcaster) error {
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
	ln, addr, err := listenFrom(port)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: serving %s at http://localhost%s/\n", dir, addr)
	return http.Serve(ln, mux)
}

// listenFrom binds :port, or the next free port up to port+20, returning the
// listener and the ":N" it bound.
func listenFrom(port int) (net.Listener, string, error) {
	for p := port; p < port+20; p++ {
		addr := fmt.Sprintf(":%d", p)
		ln, err := net.Listen("tcp", addr)
		if err == nil {
			if p != port {
				fmt.Fprintf(os.Stderr, "gophics: port %d in use — using %d\n", port, p)
			}
			return ln, addr, nil
		}
	}
	return nil, "", fmt.Errorf("no free port in %d..%d", port, port+19)
}
