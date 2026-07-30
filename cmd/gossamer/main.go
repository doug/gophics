// Command gossamer is the developer CLI for gossamer apps: build, run, and
// hot-reload one codebase across desktop, web, terminal, and mobile.
//
// Install: go install github.com/doug/gossamer/cmd/gossamer@latest
package main

import (
	"os"

	"github.com/doug/gossamer/internal/cli"
)

func main() { os.Exit(cli.Main(os.Args[1:])) }
