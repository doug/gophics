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
	"main.go":    mainTmpl,
	"ui/app.go":  uiTmpl,
	"README.md":  readmeTmpl,
	".gitignore": "/build/\n*.wasm\n",
}

const mainTmpl = `// Command {{.Name}} is a gossamer app. One codebase runs on desktop, web,
// terminal, and mobile; the shell is chosen by build tags at compile time.
//
// Root and Config are split out so the widget tree stays easy to test and
// import independently of the shell entry point.
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
//
// This starter follows two conventions that let "gossamer dev" restore your
// place on every rebuild (state-preserving hot-restart): increment the counter,
// open the detail page, edit this file, save — you come back to the same page
// with the same count.
//
//  1. State that should survive a restart uses exported fields (or implements
//     widget.Snapshottable). counterState.Count below is captured for free.
//  2. Pushed pages carry plain data and register their type, so the Navigator
//     can rebuild them. Shared dependencies come from context via
//     widget.Provide / widget.MustOf, not from page fields.
package ui

import (
	"fmt"

	"github.com/doug/gossamer/geom"
	"github.com/doug/gossamer/paint"
	"github.com/doug/gossamer/widget"
)

func init() {
	// Register pushable pages so the Navigator's stack survives a hot-restart.
	widget.RegisterSnapshotType[detailPage]()
}

var (
	colBg     = paint.RGB(0.98, 0.98, 0.99)
	colText   = paint.RGB(0.10, 0.10, 0.12)
	colAccent = paint.RGB(0.20, 0.45, 0.95)
	colOnAcc  = paint.RGB(1, 1, 1)
)

// Root returns the app's root widget. Edit this and save: "gossamer dev"
// live-reloads (web) or hot-restarts (desktop), landing back where you were.
func Root() widget.Widget {
	return widget.Navigator{Home: homePage{}}
}

// homePage is a stateful counter. Count is exported, so it is snapshotted and
// restored across a hot-restart with no extra code.
type homePage struct{}

func (homePage) CreateState() widget.State { return &counterState{} }

type counterState struct {
	widget.StateBase[homePage]
	Count int
}

func (s *counterState) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return screen("{{.Name}}", widget.Column(
		widget.Text{S: fmt.Sprintf("Count: %d", s.Count), Size: 28, Color: colText},
		widget.Sized{H: 16},
		button("Increment", func() { s.SetState(func() { s.Count++ }) }),
		widget.Sized{H: 8},
		button("Open detail", func() { nav.Push(detailPage{Count: s.Count}) }),
	))
}

// detailPage carries only data — the count it was opened with. Because it is a
// plain value and registered above, a hot-restart rebuilds it and lands you
// back on this page.
type detailPage struct{ Count int }

func (p detailPage) Build(ctx widget.Ctx) widget.Widget {
	nav := widget.MustOf[widget.Nav](ctx)
	return screen("Detail", widget.Column(
		widget.Text{S: fmt.Sprintf("Opened at count %d", p.Count), Size: 22, Color: colText},
		widget.Sized{H: 16},
		button("Back", nav.Pop),
	))
}

func button(label string, onTap func()) widget.Widget {
	return widget.Interactive{
		Handler: widget.Handler{OnTap: onTap},
		Child: widget.Decorated{Color: colAccent, Radius: 8,
			Child: widget.Padding{Insets: geom.InsetsSymmetric(16, 10),
				Child: widget.Text{S: label, Size: 15, Color: colOnAcc}}},
	}
}

func screen(title string, body widget.Widget) widget.Widget {
	col := widget.Column(
		widget.Text{S: title, Font: "bold", Size: 15, Color: colText},
		widget.Sized{H: 24},
		body,
	)
	return widget.Decorated{Color: colBg, Child: widget.Center(col)}
}
`

const readmeTmpl = `# {{.Name}}

A [gossamer](https://github.com/doug/gossamer) app — one codebase for desktop,
web, terminal, and mobile.

## Develop

    gossamer dev -p web        # live-reload in the browser (fastest loop)
    gossamer dev -p desktop    # native window, rebuild + hot-restart on save

On desktop, a rebuild preserves your place: the current page, scroll position,
text fields, and any exported widget state are restored, so you come back to
exactly where you were. See ui/app.go for the two conventions that enable it.

## Build

    gossamer build -p web       # → build/web/  (wasm + html)
    gossamer build -p desktop   # → build/desktop/app  (single binary)
    gossamer build -p terminal  # → build/terminal/app

Run ` + "`gossamer doctor`" + ` to check your toolchain.
`
