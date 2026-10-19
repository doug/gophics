// Command plain shapes a line of text and nothing else. It is the control for
// TestWebFontMechanismIsNotLinkedWhenUnused: a web app that never asks for
// on-demand fonts must not carry the machinery for them.
//
// It lives under testdata so the go tool leaves it out of ./... — it is an
// input to a test, not a package of the module. The test names its directory
// explicitly.
package main

import (
	"os"

	"github.com/doug/gophics/text"
)

func main() {
	data, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	f, err := text.Parse(data)
	if err != nil {
		panic(err)
	}
	// Shape through a Shaper so every path a normal app uses is reachable:
	// the fallback chain, resolveFace and the missing-glyph report.
	println(int(text.NewShaper(f).Line(os.Args[2], 16).Width))
}
