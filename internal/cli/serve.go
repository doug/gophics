package cli

import (
	"fmt"
	"net/http"
	"os"
)

// serve serves the web build directory over HTTP with no-store headers (so a
// reload always gets the current wasm). If b is non-nil, it also mounts a
// Server-Sent Events endpoint and injects a tiny reload client into
// index.html — the dev live-reload channel.
func serve(dir string, port int, b *broadcaster) error {
	mux := http.NewServeMux()
	if b != nil {
		mux.HandleFunc("/_gossamer/reload", sseHandler(b))
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
	addr := fmt.Sprintf(":%d", port)
	fmt.Fprintf(os.Stderr, "gossamer: serving %s at http://localhost%s/\n", dir, addr)
	return http.ListenAndServe(addr, mux)
}
