//go:build ignore

// Dev server for the HN web build: rebuilds the GPU wasm on every page load
// and serves everything with no-store headers, so the browser can never run a
// stale build (plain `go build` output is otherwise cached hard by browsers,
// which makes iterating on the wasm maddening).
//
// Run from the repo root:
//
//	go run ./examples/hn/web/devserve.go
//
// then open http://localhost:8100/. Each reload rebuilds and reloads fresh.
package main

import (
	"log"
	"net/http"
	"os"
	"os/exec"
	"sync"
	"time"
)

const (
	webDir = "examples/hn/web"
	pkg    = "./examples/hn/"
	addr   = ":8100"
)

var buildMu sync.Mutex

func buildWasm() (string, error) {
	buildMu.Lock()
	defer buildMu.Unlock()
	start := time.Now()
	cmd := exec.Command("go", "build", "-o", webDir+"/hn.wasm", pkg)
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := cmd.CombinedOutput(); err != nil {
		return string(out), err
	}
	log.Printf("rebuilt hn.wasm in %s", time.Since(start).Round(time.Millisecond))
	return "", nil
}

func main() {
	fs := http.FileServer(http.Dir(webDir))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
		w.Header().Set("Pragma", "no-cache")
		// A page load rebuilds the wasm before serving the document, so the
		// hn.wasm the browser fetches next is always current.
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			if out, err := buildWasm(); err != nil {
				log.Printf("build failed: %v\n%s", err, out)
				http.Error(w, "build failed:\n"+out, http.StatusInternalServerError)
				return
			}
		}
		fs.ServeHTTP(w, r)
	})
	log.Printf("HN web dev server on %s — rebuilds wasm per load, no cache; open http://localhost%s/", addr, addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
