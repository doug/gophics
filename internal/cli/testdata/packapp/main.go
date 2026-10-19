// Command packapp is the app the desktop packaging tests are pointed at.
//
// It imports nothing from gophics, which is deliberate: the packaging tests
// build it for darwin, linux and windows in one run, and what they assert on
// is the bundle, the desktop entry, the archive and the PE header around the
// binary — none of which depends on what the binary does. Linking the real
// framework three times would make the suite minutes long and would test the
// shell, not the packaging.
//
// It lives under testdata for the reason bindapp does: the go tool skips
// testdata when it expands ./..., so no sweep compiles it, while
// `go build <import path>` still does.
package main

import "fmt"

func main() { fmt.Println("packapp") }
