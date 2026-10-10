package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// These tests must not leave a Go package inside a module.
//
// Generating a bind package writes one into the app's own build/ directory.
// Done against an app in examples/, that package exists for as long as the
// test runs and is deleted at the end of it — so a `go build ./...` or
// `go test ./...` sharing the checkout can enumerate a package that is about
// to vanish, and an interrupted run leaves one behind to be compiled by the
// next sweep. examples/tally even hides its build/ in a .gitignore, so a
// leftover there is invisible until a sweep trips over it.
//
// The generator's own tests point at an app under testdata instead, which the
// go tool skips when it expands ./..., and this is the guard that keeps them
// there. Only directories this run created are reported: a leftover from an
// earlier interrupted run is the symptom, not something these tests did.
func TestMain(m *testing.M) {
	before := exampleBuildDirs()
	code := m.Run()
	if code == 0 {
		for d := range exampleBuildDirs() {
			if before[d] {
				continue
			}
			println("this run generated a package inside the module: " + d)
			println("point the test at testdata/bindapp, or ask for the name with bindName")
			code = 1
		}
	}
	os.Exit(code)
}

// exampleBuildDirs is the set of build/ directories under examples/. Nothing
// checked in is called that; they are all generator or packaging output.
func exampleBuildDirs() map[string]bool {
	root, err := packageDir("github.com/doug/gophics/examples/mirror")
	if err != nil {
		return nil // the examples cannot be located; nothing to say
	}
	examples := filepath.Dir(root)
	ents, err := os.ReadDir(examples)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		dir := filepath.Join(examples, e.Name(), "build")
		if _, err := os.Stat(dir); err == nil {
			out[dir] = true
		}
	}
	return out
}
