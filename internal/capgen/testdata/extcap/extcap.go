// Package extcap is a worked example of a gophics capability declared outside
// gophics: the thing a third-party plugin module would ship.
//
// It lives under testdata because that is this repository's way of keeping a
// fixture that must compile but must not run as a test of its own — `go test
// ./...` skips testdata directories, while `go build` of this import path, and
// any test that imports it, compiles it. Treat it as documentation that the
// compiler checks.
//
// The point it proves is what it does *not* import. Everything here comes from
// gophics's public API — widget and shell — so nothing about the mechanism
// depends on being inside the module. Moving this directory into its own module
// with its own go.mod would change nothing but the import path, which is the
// whole claim; TestFixtureUsesOnlyPublicAPI in the parent package asserts it
// rather than trusting it.
//
// A real plugin would also contain the layer this fixture cannot: a
// build-tagged implementation per platform, dlopen/syscall/JS interop under it,
// and a shell Window wrapper that returns it. That is orthogonal to the seam —
// and, like gophics itself, needs no cgo to write.
package extcap

import (
	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Scanner reads barcodes from the device's camera. It is written exactly as an
// in-tree capability is: a small interface whose callbacks deliver results,
// paired with the ScannerWindow opt-in below.
//
// A barcode scanner is a fair example of something that belongs in a plugin
// rather than in the framework: it is genuinely optional (most platforms have
// no scanner, and most apps want no scanner), and it passes the project's own
// capability test — a real platform can legitimately lack it, and it calls
// back.
type Scanner interface {
	// Scan starts scanning and calls found for each code, until Stop. found
	// may be nil, in which case Scan does nothing: scanning with nobody to
	// receive the codes would leave the camera running for no reason, which is
	// the nil-callback rule the shell package documents.
	Scan(found func(code string))
	// Stop stops scanning. A side effect, so it runs regardless of whether
	// anyone asked for the result.
	Stop()
}

// ScannerWindow is the opt-in: a shell Window implements it when the platform
// it speaks for can scan. Declaring it here rather than in gophics is the
// entire difference between a plugin and a patch to the framework.
type ScannerWindow interface {
	// Scanner returns the scanning capability, or nil if unavailable.
	Scanner() Scanner
}

// Cap is the registered key. An app reads the capability through it:
//
//	if s := extcap.Cap.Of(ctx); s != nil {
//	    s.Scan(func(code string) { ... })
//	}
//
// A package-level var, so registration happens during program initialization
// and the hollow check below runs in this package's own tests — not in the
// apps that import it.
var Cap = widget.NewCapability[Scanner]("example.com/extcap.Scanner", provide)

// provide answers the window, and only the window. It is a type assertion and
// a wrap and nothing else, which is both the documented contract for a provider
// and what makes the hollow check meaningful: handed a window that implements
// no ScannerWindow, it returns nil, so a platform without a scanner reports
// nil instead of a scanner that never finds anything.
func provide(w shell.Window, post func(func())) Scanner {
	x, ok := w.(ScannerWindow)
	if !ok {
		return nil
	}
	return postedScanner(x.Scanner(), post)
}

// postedScanner delivers Scan's callbacks through post, the UI-goroutine
// scheduler the framework hands in.
//
// A plugin writes this by hand where an in-tree capability gets it generated:
// capgen builds the Posted wrappers by parsing the capability's source in
// package shell, and it cannot parse a type in somebody else's module. Handing
// the scheduler to the provider instead is what keeps the "all callbacks fire
// on the UI goroutine" contract available out of tree.
//
// The nil-safety is not decoration. A nil inner must produce a nil Scanner, or
// an absent capability comes back as a live-looking wrapper around nothing —
// precisely the hollow shape. NewCapability's registration probe cannot reach
// this branch, because the probe window implements no ScannerWindow and so
// never gets as far as the wrap; a window that implements ScannerWindow and
// returns nil does, and only this package can build one. That is what
// Cap.ProvidedBy is for, and the parent package's test makes exactly that
// assertion. A nil post means no runner is attached (a headless Owner in a
// test); callbacks then fire inline, matching the generated in-tree wrappers.
func postedScanner(inner Scanner, post func(func())) Scanner {
	if inner == nil || post == nil {
		return inner
	}
	return postedWrapper{inner: inner, post: post}
}

type postedWrapper struct {
	inner Scanner
	post  func(func())
}

func (p postedWrapper) Scan(found func(code string)) {
	if found != nil {
		deliver := found
		found = func(code string) { p.post(func() { deliver(code) }) }
	}
	p.inner.Scan(found)
}

func (p postedWrapper) Stop() { p.inner.Stop() }
