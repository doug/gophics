// Package capscan works out which platform capabilities an app actually
// reaches, so its manifest can be derived rather than remembered.
//
// Capabilities are unusually easy to find in gophics. Each one is a single
// zero-argument method on the concrete type widget.Ctx — ctx.Camera(),
// ctx.Microphone() — with no interface indirection and no reflection anywhere
// in the path. So a type-checked walk over the build finds every use, and the
// answer is exact rather than a heuristic.
//
// Two things are deliberately not derived from capabilities:
//
//   - Networking. It is not detected here at all. The core widget package
//     imports net/http for NetworkImage, so every app has it in the graph and
//     a check would be true always — see manifest.Baseline.
//
//   - Anything reached from a dependency the scan cannot see. It scans the
//     whole build, dependencies included, but a capability reached by
//     reflection or built at runtime is invisible by construction. Extras exist
//     for that, and are the app's to declare.
package capscan

import (
	"fmt"
	"go/types"
	"os"
	"sort"
	"strings"

	"golang.org/x/tools/go/packages"

	"github.com/doug/gophics/internal/manifest"
)

// ctxType is the type whose methods are capability accessors.
const (
	widgetPkg = "github.com/doug/gophics/widget"
	ctxName   = "Ctx"
)

// Result is what a scan found.
type Result struct {
	// Capabilities are the capability names the build reaches, sorted.
	Capabilities []string
	// Packages is how many packages were scanned, for reporting — a scan that
	// silently covered nothing should be visible rather than look clean.
	Packages int
}

// Target is the platform to scan for.
//
// It matters: a capability reached only from platform-specific code is
// invisible to a scan configured for a different one, and the answer would be
// silently short. GOOS/GOARCH are set through the environment rather than as
// build tags — android and ios are operating systems, and passing them as tags
// makes a darwin host compile its own files alongside the target's, which does
// not type-check.
type Target struct {
	GOOS   string // empty: the host
	GOARCH string // empty: the host
	Tags   []string
}

// Scan type-checks the build rooted at pattern and reports what it reaches.
//
// dir is the directory to resolve the pattern in; pattern is a go list pattern
// such as "." or "./mobile".
func Scan(dir, pattern string, target Target) (Result, error) {
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedImports | packages.NeedDeps |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
		Dir: dir,
		Env: os.Environ(),
	}
	if target.GOOS != "" {
		cfg.Env = append(cfg.Env, "GOOS="+target.GOOS)
	}
	if target.GOARCH != "" {
		cfg.Env = append(cfg.Env, "GOARCH="+target.GOARCH)
	}
	if len(target.Tags) > 0 {
		cfg.BuildFlags = []string{"-tags=" + strings.Join(target.Tags, ",")}
	}

	pkgs, err := packages.Load(cfg, pattern)
	if err != nil {
		return Result{}, fmt.Errorf("capscan: load %s: %w", pattern, err)
	}
	if len(pkgs) == 0 {
		return Result{}, fmt.Errorf("capscan: %s matched no packages", pattern)
	}
	// Load reports per-package errors rather than failing, and a package that
	// failed to type-check contributes no capabilities — which would look
	// exactly like an app that uses none.
	var loadErrs []string
	packages.Visit(pkgs, nil, func(p *packages.Package) {
		for _, e := range p.Errors {
			loadErrs = append(loadErrs, e.Error())
		}
	})
	if len(loadErrs) > 0 {
		return Result{}, fmt.Errorf("capscan: the build does not type-check, so "+
			"a scan would under-report:\n  %s", strings.Join(loadErrs, "\n  "))
	}

	found := map[string]bool{}
	res := Result{}

	packages.Visit(pkgs, nil, func(p *packages.Package) {
		res.Packages++
		if p.TypesInfo == nil {
			return
		}
		// Every selection the type checker resolved. A capability use is a
		// method selection whose receiver is widget.Ctx — which covers both a
		// call and a method value, since either one reaches the capability.
		for _, sel := range p.TypesInfo.Selections {
			if sel.Kind() != types.MethodVal {
				continue
			}
			recv := sel.Recv()
			if recv == nil || !isWidgetCtx(recv.String()) {
				continue
			}
			// Ctx carries far more than capabilities — Post, Invalidate,
			// DarkMode, the painter. Only names the permission table knows
			// are capabilities; the rest are ordinary framework calls and
			// declaring anything for them would be noise.
			name := sel.Obj().Name()
			if _, ok := manifest.For(name); ok {
				found[name] = true
			}
		}
	})

	for name := range found {
		res.Capabilities = append(res.Capabilities, name)
	}
	sort.Strings(res.Capabilities)
	return res, nil
}

// isWidgetCtx reports whether a type string names widget.Ctx. Compared as a
// string because the receiver may be the type or a pointer to it, and the
// package may be vendored under a different path in a consuming module.
func isWidgetCtx(typ string) bool {
	typ = strings.TrimPrefix(typ, "*")
	return typ == widgetPkg+"."+ctxName || strings.HasSuffix(typ, "/widget."+ctxName)
}
