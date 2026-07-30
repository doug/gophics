package cli

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"text/template"
)

func cmdCreate(args []string) error {
	fs := flag.NewFlagSet("create", flag.ContinueOnError)
	var module string
	fs.StringVar(&module, "module", "", "Go module path (default: the app name)")
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if fs.NArg() < 1 {
		return fmt.Errorf("usage: gossamer create <name> [-module path]")
	}
	name := fs.Arg(0)
	if module == "" {
		module = name
	}
	if _, err := os.Stat(name); err == nil {
		return fmt.Errorf("%q already exists", name)
	}
	data := map[string]string{"Module": module, "Name": name}
	for path, tmpl := range scaffold {
		if err := writeTemplate(filepath.Join(name, path), tmpl, data); err != nil {
			return err
		}
	}
	// Initialize the module. We intentionally do NOT `go get` gossamer here: it
	// isn't published yet (and uses local replace forks), so that would hang or
	// fail. The user wires the dependency themselves (see printed guidance).
	if err := run(name, nil, "go", "mod", "init", module); err != nil {
		return fmt.Errorf("go mod init: %w", err)
	}
	fmt.Printf(`created %s

Next:
  cd %s
  # add the gossamer dependency (not yet published), e.g. a replace to a local checkout:
  #   go mod edit -require=github.com/doug/gossamer@v0.0.0
  #   go mod edit -replace=github.com/doug/gossamer=/path/to/gossamer
  #   go mod tidy
  gossamer dev -p web       # fastest loop
  gossamer run -p desktop    # native window
`, name, name)
	return nil
}

func writeTemplate(path, tmpl string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	t, err := template.New(path).Parse(tmpl)
	if err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return t.Execute(f, data)
}

// scaffold maps output paths to templates for a new cross-platform app.
var scaffold = map[string]string{
	"main.go":     mainTmpl,
	"ui/app.go":   uiTmpl,
	"README.md":   readmeTmpl,
	".gitignore":  "/build/\n*.wasm\n",
}

const mainTmpl = `// Command {{.Name}} is a gossamer app. One codebase runs on desktop, web,
// terminal, and mobile; the shell is chosen by build tags at compile time.
//
// Root and Config are exported so ` + "`gossamer dev --hot`" + ` can build this
// package as a plugin and hot-reload the widget tree.
package main

import (
	"log"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/doug/gossamer/app"
	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/widget"

	"{{.Module}}/ui"
)

// Root returns the app's root widget.
func Root() widget.Widget { return ui.Root() }

// Config returns the app's window/runtime configuration.
func Config() app.Config {
	return app.Config{
		Title: "{{.Name}}",
		Size:  geom.Size{W: 480, H: 720},
		Font:  goregular.TTF,
	}
}

func main() {
	if err := app.Run(Root(), Config()); err != nil {
		log.Fatal(err)
	}
}
`

const uiTmpl = `// Package ui holds {{.Name}}'s widget tree.
package ui

import (
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

// Root returns the app's root widget. Edit this and save — ` + "`gossamer dev`" + `
// live-reloads (web) or hot-restarts (desktop).
func Root() widget.Widget {
	return widget.Center(widget.Text{
		S:     "Hello, {{.Name}} 👋",
		Size:  28,
		Font:  "",
		Color: paint.RGB(0.10, 0.10, 0.12),
	})
}
`

const readmeTmpl = `# {{.Name}}

A [gossamer](https://github.com/doug/gossamer) app — one codebase for desktop,
web, terminal, and mobile.

## Develop

    gossamer dev -p web        # live-reload in the browser (fastest)
    gossamer dev -p desktop    # native window, hot-restart on save
    gossamer dev -p desktop --hot   # experimental: state-preserving hot reload (linux/macOS)

## Build

    gossamer build -p web       # → build/web/  (wasm + html)
    gossamer build -p desktop   # → build/desktop/app  (single binary)
    gossamer build -p terminal  # → build/terminal/app

Run ` + "`gossamer doctor`" + ` to check your toolchain.
`
