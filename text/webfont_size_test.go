//go:build !js

package text

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The web-font mechanism must cost nothing to an app that does not use it.
// Web payload is a figure this project tracks, and a fallback path that linked
// the browser's fetch() and a per-script cache into every wasm binary would be
// a regression paid for by every app in exchange for a feature most do not
// want.
//
// The claim is testable rather than assertable, so it is tested. Two programs
// under testdata/sizeprobe differ in nothing but a call to UseWebFonts; this
// links both for js/wasm and checks that the machinery is in the one that asks
// for it and absent from the one that does not. The second program is not
// decoration: without it the test would pass just as happily against symbol
// names no build ever emits, which would make it a test of its own spelling.
//
// Function names are read out of the wasm binary's name section rather than
// with `go tool nm`, which does not recognise a wasm object file.
func TestWebFontMechanismIsNotLinkedWhenUnused(t *testing.T) {
	if _, err := exec.LookPath("go"); err != nil {
		t.Skipf("no go tool on PATH to link with: %v", err)
	}
	dir := t.TempDir()

	build := func(pkg string) []byte {
		t.Helper()
		out := filepath.Join(dir, pkg+".wasm")
		cmd := exec.Command("go", "build", "-o", out, "./testdata/sizeprobe/"+pkg)
		cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm", "CGO_ENABLED=0")
		if msg, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("building %s for js/wasm: %v\n%s", pkg, err, msg)
		}
		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	plain, webfonts := build("plain"), build("webfonts")

	// webFontMap is the per-script cache; fetch.Do is the browser fetch()
	// wrapper behind FetchFontSource. Neither has any other way into a
	// binary, so their absence is the whole claim and their presence in the
	// other binary is the control.
	//
	// "UseWebFonts" itself is deliberately not a needle: the method's *name*
	// survives in type metadata and in the missing-glyph message, which name
	// the remedy on purpose. A name string is not the code.
	for _, needle := range []string{"webFontMap", "urlFontSource", "fetch.Do"} {
		if bytes.Contains(plain, []byte(needle)) {
			t.Errorf("a program that never calls UseWebFonts still links %q", needle)
		}
		if !bytes.Contains(webfonts, []byte(needle)) {
			t.Errorf("a program that does call UseWebFonts does not link %q; "+
				"the needle is stale and the check above proves nothing", needle)
		}
	}

	// Reported rather than bounded. A threshold here would be a second,
	// worse copy of the payload budget, and it would fail on a Go release
	// that moved the baseline rather than on a change to this package.
	t.Logf("js/wasm: plain %d bytes, with web fonts %d bytes (+%d)",
		len(plain), len(webfonts), len(webfonts)-len(plain))
}
