// Package apisurface enumerates the module's public API.
//
// PLAN.md §1.7 promises "a small surface and the Go 1 compatibility ethos after
// 1.0". A promise about a number needs the number to be measurable, and every
// refactor that claims to shrink it needs a diff rather than an assertion — so
// the surface is written to design/api-surface.txt and a test fails when the
// tree and the file disagree.
//
// What counts as surface is what a caller can name and a rename can break:
// exported top-level objects, exported methods on exported named types, and
// exported struct fields and interface methods. A struct field is API —
// widget.Text.Wrap is something apps write.
//
// Two things are deliberately excluded. internal/ and examples/ are not API.
// shell/mobile is not either: its ~130 exported Bridge methods exist because
// gomobile requires them, they are called only from Kotlin and Swift, and they
// are versioned with the host app rather than with this module — counting them
// would swamp the number that the promise is about.
package apisurface

import (
	"fmt"
	"go/types"
	"os"
	"slices"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"
)

// Excluded package-path substrings. Kept as substrings rather than exact paths
// so a new package under any of them is excluded by construction.
var excluded = []string{
	"/internal/",
	"/internal",
	"/examples/",
	"/docs/",
	"/skills/",
	"/cmd/",
	"/shell/mobile",
}

// Entry is one exported identifier, rendered as a stable one-line string.
type Entry string

// Targets are the build configurations the surface is collected under.
//
// One platform is not enough. The shells are build-constrained — shell/web is
// js/wasm only, shell/desktop is !js — so a scan on the host machine cannot see
// shell/web.Run at all, and a manifest taken that way silently omits a public
// package while reading as complete. The surface is the union across the
// platforms the toolkit supports.
var Targets = []struct{ GOOS, GOARCH string }{
	{"darwin", "arm64"},
	{"linux", "amd64"},
	{"windows", "amd64"},
	{"js", "wasm"},
	{"android", "arm64"},
	{"ios", "arm64"},
}

// Collect returns the sorted public surface of the module rooted at dir, as the
// union over Targets.
//
// Consumers are not scanned here; see Consumers for why that is a separate
// question.
func Collect(dir string) ([]Entry, error) {
	seen := map[Entry]bool{}
	for _, t := range Targets {
		if err := collectInto(seen, dir, t.GOOS, t.GOARCH); err != nil {
			return nil, err
		}
	}
	out := make([]Entry, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	slices.Sort(out)
	return out, nil
}

func collectInto(seen map[Entry]bool, dir, goos, goarch string) error {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedTypes | packages.NeedSyntax |
			packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
		Dir: dir,
		Env: append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0"),
	}
	pkgs, err := packages.Load(cfg, "./...")
	if err != nil {
		return fmt.Errorf("apisurface: load %s/%s: %w", goos, goarch, err)
	}
	// Type errors are not fatal here the way they are for a single-platform
	// scan: a cross-compile of a package whose cgo or platform deps are absent
	// reports errors while still yielding the declarations we want, and another
	// target will cover it. A package that fails everywhere contributes nothing
	// and shows up as a removal in the diff, which is the signal.
	for _, p := range pkgs {
		if p.Types == nil || skip(p.PkgPath) {
			continue
		}
		for _, e := range pkgEntries(p) {
			seen[e] = true
		}
	}
	return nil
}

func skip(path string) bool {
	for _, x := range excluded {
		if strings.Contains(path, x) {
			return true
		}
	}
	return false
}

func pkgEntries(p *packages.Package) []Entry {
	var out []Entry
	short := shortPath(p.PkgPath)
	scope := p.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if !obj.Exported() {
			continue
		}
		out = append(out, Entry(fmt.Sprintf("%s.%s %s", short, name, kindOf(obj))))

		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		// Methods on the named type, and on its pointer — a method set is split
		// between the two and both are callable from an addressable value.
		for _, t := range []types.Type{named, types.NewPointer(named)} {
			ms := types.NewMethodSet(t)
			for i := range ms.Len() {
				m := ms.At(i).Obj()
				if m.Exported() {
					out = append(out, Entry(fmt.Sprintf("%s.%s.%s method", short, name, m.Name())))
				}
			}
		}
		switch u := named.Underlying().(type) {
		case *types.Struct:
			for i := range u.NumFields() {
				if f := u.Field(i); f.Exported() {
					out = append(out, Entry(fmt.Sprintf("%s.%s.%s field", short, name, f.Name())))
				}
			}
		case *types.Interface:
			for i := range u.NumMethods() {
				if m := u.Method(i); m.Exported() {
					out = append(out, Entry(fmt.Sprintf("%s.%s.%s method", short, name, m.Name())))
				}
			}
		}
	}
	return out
}

func kindOf(obj types.Object) string {
	switch obj.(type) {
	case *types.Func:
		return "func"
	case *types.TypeName:
		return "type"
	case *types.Const:
		return "const"
	case *types.Var:
		return "var"
	default:
		return "object"
	}
}

// shortPath drops the module prefix so the manifest reads as package paths
// rather than repeating the module on every line.
func shortPath(p string) string {
	const mod = "github.com/doug/gophics/"
	if s, ok := strings.CutPrefix(p, mod); ok {
		return s
	}
	if p == strings.TrimSuffix(mod, "/") {
		return "."
	}
	return p
}

// Consumers reports which of the module's own packages are imported from dirs.
//
// This exists because of a specific mistake. The audit that produced
// design/api-surface-reduction.md concluded "nothing imports intl" and was
// wrong: examples/tally does, and calls intl.Auto() — on Android and iOS, where
// the environment variables it reads do not exist. It was missed because tally
// is a *separate module*, invisible to `go list ./...` from the root, which is
// how the whole audit was taken.
//
// So any claim of the form "no caller anywhere" has to be checked against the
// separate modules too, and this makes that cheap.
func Consumers(dirs ...string) (map[string][]string, error) {
	used := map[string]map[string]bool{}
	for _, dir := range dirs {
		cfg := &packages.Config{
			Mode: packages.NeedName | packages.NeedImports | packages.NeedSyntax | packages.NeedTypes,
			Dir:  dir,
		}
		pkgs, err := packages.Load(cfg, "./...")
		if err != nil {
			return nil, fmt.Errorf("apisurface: consumers in %s: %w", dir, err)
		}
		for _, p := range pkgs {
			for imp := range p.Imports {
				if !strings.HasPrefix(imp, "github.com/doug/gophics/") {
					continue
				}
				short := shortPath(imp)
				if used[short] == nil {
					used[short] = map[string]bool{}
				}
				used[short][p.PkgPath] = true
			}
		}
	}
	out := map[string][]string{}
	for pkg, byWhom := range used {
		for c := range byWhom {
			out[pkg] = append(out[pkg], c)
		}
		sort.Strings(out[pkg])
	}
	return out, nil
}
