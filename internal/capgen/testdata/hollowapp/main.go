// Command hollowapp is an application that imports a plugin registering a
// hollow capability. It exists to be run and to fail.
//
// The in-process test asserts that widget.NewCapability panics. This asserts
// the consequence that matters: because registration happens in a package-level
// var, the panic lands during program initialization, so an app carrying such a
// plugin does not start — it does not reach main, let alone a frame, and the
// message names the capability. "Fails loudly" is a claim about a program, and
// the only way to check a claim about a program is to run one.
package main

import (
	"fmt"

	"github.com/doug/gophics/shell"
	"github.com/doug/gophics/widget"
)

// Scanner stands in for the plugin's capability.
type Scanner interface{ Scan(found func(code string)) }

type doNothing struct{}

func (doNothing) Scan(found func(code string)) {}

// cap is the registration a careless plugin author writes: it never asks the
// window whether the platform can scan, so every platform reports a scanner
// and none of them scans. The blank identifier because nothing here uses it;
// the panic happens on the way to assigning it.
var _ = widget.NewCapability[Scanner]("example.com/hollowapp.Scanner",
	func(w shell.Window, post func(func())) Scanner { return doNothing{} })

func main() {
	fmt.Println("unreachable: initialization should have panicked")
}
