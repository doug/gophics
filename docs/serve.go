//go:build ignore

// Command serve hosts the docs/ site locally for preview — a pure-Go static
// file server (no python needed). WASM is served with the correct MIME type so
// WebAssembly.instantiateStreaming works; localhost is a secure context, so the
// WebGPU demos run.
//
//	go run docs/serve.go            # http://localhost:8099
//	go run docs/serve.go -addr :9000
//	go run docs/serve.go -build=false   # serve whatever is already on disk
//
// # Demo freshness
//
// A request for /demos/<name>.wasm rebuilds that example first. The wasm files
// are build output (gitignored — 19 of them at ~15 MB), so without this they
// are whatever was last built, and nothing says how old that is. That is not
// hypothetical: a shader fix six directories away from examples/ left every
// demo serving a weeks-old binary, and the only symptom was validation errors
// in the browser console that had already been fixed in the source.
//
// It deliberately does NOT hash inputs or keep its own cache. Go's build cache
// is already content-addressed over the full transitive dependency graph —
// including //go:embed'ed files, which is exactly what a naive "did the example
// directory change?" check would miss. Re-running `go build` asks that cache
// rather than duplicating it, and on a warm cache the whole cost is the link
// step: ~0.5s, against a 15 MB transfer. Correct beats clever here.
package main

import (
	"flag"
	"fmt"
	"log"
	"mime"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

func main() {
	addr := flag.String("addr", ":8099", "listen address")
	build := flag.Bool("build", true, "rebuild a demo's wasm when it is requested")
	flag.Parse()

	mime.AddExtensionType(".wasm", "application/wasm")

	// Serve the docs/ dir whether run from the repo root or from inside docs/.
	dir := "docs"
	if _, err := os.Stat("index.html"); err == nil {
		dir = "."
	}
	root, _ := filepath.Abs(dir)
	repo := filepath.Dir(root)

	fs := http.FileServer(http.Dir(root))
	b := &builder{repo: repo, out: filepath.Join(root, "demos")}

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store, must-revalidate")
		if *build {
			if name, ok := demoName(r.URL.Path); ok {
				if err := b.build(name); err != nil {
					// Surface the compiler error instead of quietly serving a
					// stale binary — a broken example should look broken.
					log.Printf("build %s: %v", name, err)
					http.Error(w, "building "+name+":\n\n"+err.Error(), http.StatusInternalServerError)
					return
				}
			}
		}
		fs.ServeHTTP(w, r)
	})

	log.Printf("serving %s on http://localhost%s (no-cache, rebuild-on-request=%v)", root, *addr, *build)
	log.Fatal(http.ListenAndServe(*addr, handler))
}

// demoPath matches /demos/<name>.wasm and nothing else; the name is restricted
// so a request can never name a directory outside examples/.
var demoPath = regexp.MustCompile(`^/demos/([a-z0-9_]+)\.wasm$`)

func demoName(p string) (string, bool) {
	m := demoPath.FindStringSubmatch(p)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// builder compiles examples to wasm on demand, one at a time per example.
type builder struct {
	repo string
	out  string

	mu       sync.Mutex
	building map[string]*sync.Mutex
}

// build compiles examples/<name> to the served wasm path. Concurrent requests
// for the same demo (a reload while the first is still linking) wait rather
// than racing on the output file.
func (b *builder) build(name string) error {
	src := filepath.Join(b.repo, "examples", name)
	if _, err := os.Stat(src); err != nil {
		return nil // not an example: let the file server 404 or serve a stray file
	}

	b.mu.Lock()
	if b.building == nil {
		b.building = map[string]*sync.Mutex{}
	}
	lk, ok := b.building[name]
	if !ok {
		lk = &sync.Mutex{}
		b.building[name] = lk
	}
	b.mu.Unlock()

	lk.Lock()
	defer lk.Unlock()

	start := time.Now()
	cmd := exec.Command("go", "build", "-ldflags=-s -w",
		"-o", filepath.Join(b.out, name+".wasm"), "./examples/"+name)
	cmd.Dir = b.repo
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v\n%s", err, strings.TrimSpace(string(out)))
	}
	if d := time.Since(start); d > 1500*time.Millisecond {
		// Only worth a line when something actually recompiled; a warm cache
		// is quiet.
		log.Printf("rebuilt %s in %s", name, d.Round(10*time.Millisecond))
	}
	return nil
}
