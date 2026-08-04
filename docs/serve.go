//go:build ignore

// Command serve hosts the docs/ site locally for preview — a pure-Go static
// file server (no python needed). WASM is served with the correct MIME type so
// WebAssembly.instantiateStreaming works; localhost is a secure context, so the
// WebGPU demos run.
//
//	go run docs/serve.go            # http://localhost:8099
//	go run docs/serve.go -addr :9000
package main

import (
	"flag"
	"log"
	"mime"
	"net/http"
	"os"
	"path/filepath"
)

func main() {
	addr := flag.String("addr", ":8099", "listen address")
	flag.Parse()

	mime.AddExtensionType(".wasm", "application/wasm")

	// Serve the docs/ dir whether run from the repo root or from inside docs/.
	dir := "docs"
	if _, err := os.Stat("index.html"); err == nil {
		dir = "."
	}
	root, _ := filepath.Abs(dir)

	fs := http.FileServer(http.Dir(root))
	nocache := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		fs.ServeHTTP(w, r)
	})
	log.Printf("serving %s on http://localhost%s (no-cache)", root, *addr)
	log.Fatal(http.ListenAndServe(*addr, nocache))
}
