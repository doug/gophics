// Command gophics is the developer CLI for gophics apps: build, run, and
// hot-reload one codebase across desktop, web, terminal, and mobile.
//
// Install: go install github.com/doug/gophics/cmd/gophics@latest
package main

import (
	"os"

	"github.com/doug/gophics/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
