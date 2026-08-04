// Package cli implements the gophics developer CLI. It orchestrates the
// per-platform build incantations (go build with the right tags, GOOS/GOARCH
// for wasm, gomobile bind for mobile), a hot-reloading dev loop, project
// scaffolding, and a toolchain doctor — a single tool over the build-tag
// matrix, in the spirit of the `flutter` command.
//
// It depends only on the standard library and the Go toolchain on PATH.
package cli

import (
	"fmt"
	"os"
)

const version = "0.1.0"

const usage = `gophics — developer CLI for gophics apps

Usage:
  gophics <command> [flags] [package]

Commands:
  run       build and run the app for a platform
  build     build release artifacts for a platform
  dev       iterate with live-reload (web) or rebuild + hot-restart (native)
  create    scaffold a new cross-platform gophics app
  doctor    check the toolchain for each platform
  version   print the gophics CLI version

The optional [package] is the Go package to build (default ".").
Run "gophics <command> -h" for a command's flags.
`

// Main runs the CLI over args (typically os.Args[1:]) and returns a process
// exit code.
func Main(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usage)
		return 2
	}
	cmd, rest := args[0], args[1:]
	var err error
	switch cmd {
	case "run":
		err = cmdRun(rest)
	case "build":
		err = cmdBuild(rest)
	case "dev":
		err = cmdDev(rest)
	case "create":
		err = cmdCreate(rest)
	case "doctor":
		err = cmdDoctor(rest)
	case "version", "-v", "--version":
		fmt.Println("gophics", version)
	case "help", "-h", "--help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "gophics: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "gophics: %v\n", err)
		return 1
	}
	return 0
}
