// Command gophics-intl extracts translatable messages from Go source into a
// catalog file that intl.ReadJSON loads.
//
//	gophics-intl -lang de -o locales/de.json ./...
//
// It is re-runnable, which is the whole point: the output of an earlier run is
// merged back in, so translations already done survive and only new keys arrive
// empty. Run it in CI with -check to fail a build whose catalog has drifted
// from its source.
//
//	gophics-intl -lang en -o locales/en.json .          # write or update
//	gophics-intl -lang en -o locales/en.json -check .   # verify, write nothing
//	gophics-intl -refs .                                # where each key is used
//
// A standalone command rather than a `gophics intl` subcommand because the
// `gophics` CLI lives in internal/cli and belongs to the build and scaffolding
// story; folding this in there is a one-file change whenever that is wanted.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/doug/gophics/intl"
	"github.com/doug/gophics/intl/internal/extract"
)

func main() {
	lang := flag.String("lang", "en", "BCP-47 tag of the catalog to write; only the language subtag is kept")
	out := flag.String("o", "", "write the catalog here (default: stdout)")
	check := flag.Bool("check", false, "exit non-zero if the catalog at -o is not what extraction produces, and write nothing")
	refs := flag.Bool("refs", false, "list each message key and its call sites instead of writing a catalog")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: gophics-intl [flags] [dir]\n\n")
		flag.PrintDefaults()
	}
	flag.Parse()

	root := "."
	if flag.NArg() > 0 {
		// Accept the ./... spelling because that is what every other Go tool
		// takes, even though this walks the tree itself rather than asking
		// go/packages — one fewer surprise at the cost of three lines.
		root = strings.TrimSuffix(strings.TrimSuffix(flag.Arg(0), "..."), "/")
		if root == "" {
			root = "."
		}
	}
	if err := run(root, *lang, *out, *check, *refs); err != nil {
		fmt.Fprintln(os.Stderr, "gophics-intl:", err)
		os.Exit(1)
	}
}

func run(root, lang, out string, check, showRefs bool) error {
	abs, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	msgs, err := extract.Walk(os.DirFS(abs), ".")
	if err != nil {
		return err
	}
	if showRefs {
		_, err := os.Stdout.WriteString(extract.Refs(msgs))
		return err
	}

	// Merge: a catalog already at -o keeps its translations. Read before write,
	// and a missing file is not an error — the first run has nothing to merge.
	var prev *intl.Catalog
	if out != "" {
		if f, err := os.Open(out); err == nil {
			prev, err = intl.ReadJSON(f)
			f.Close()
			if err != nil {
				return fmt.Errorf("%s: %w", out, err)
			}
		} else if !os.IsNotExist(err) {
			return err
		}
	}

	var buf bytes.Buffer
	if err := extract.Catalog(lang, msgs, prev).WriteJSON(&buf); err != nil {
		return err
	}

	if out == "" {
		_, err := os.Stdout.Write(buf.Bytes())
		return err
	}
	if check {
		have, err := os.ReadFile(out)
		if err != nil {
			return fmt.Errorf("%s: %w (run without -check to create it)", out, err)
		}
		if !bytes.Equal(have, buf.Bytes()) {
			return fmt.Errorf("%s is out of date; re-run without -check", out)
		}
		return nil
	}
	if dir := filepath.Dir(out); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}
