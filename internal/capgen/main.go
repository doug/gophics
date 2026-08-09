// Command capgen generates the platform-capability plumbing from the
// shell.<X>Window interfaces, so adding a capability never means hand-editing
// widget.Owner, the Ctx accessors, and the app-runner wiring in lockstep.
//
// It scans shell/*.go for every interface named "<X>Window" (other than the base
// Window) whose methods are all zero-argument, single-interface-result getters —
// the capability-provider convention (MediaWindow.Camera() Camera, etc.) — and
// emits:
//
//	widget/capabilities_gen.go  — the Capabilities struct (embedded in Owner) plus
//	                              a Ctx.<Cap>() accessor for each capability
//	app/capabilities_gen.go     — wireCapabilities(owner, window): one type-assert
//	                              per <X>Window, publishing what the window exposes
//	shell/posted_gen.go         — Posted<Cap> wrappers that deliver every callback
//	                              through a post func (the UI-goroutine scheduler),
//	                              so shell implementations may invoke callbacks
//	                              from any goroutine and the documented
//	                              "callbacks fire on the UI goroutine" contract
//	                              is enforced centrally, not per platform
//
// The posted wrappers are recursive: a callback argument or method result whose
// type is itself a callback-carrying shell interface (e.g. the Recorder
// delivered by Audio.Record) is wrapped too, so its own callbacks also post.
// Interfaces with no callbacks anywhere (SecureStorage, Playback) pass through
// unwrapped.
//
// Run via `go generate ./...` (see widget/gen.go). The generator locates the
// module root itself, so the working directory doesn't matter.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

type capability struct{ Field, Type string } // e.g. {"Camera", "Camera"} → owner.Camera shell.Camera

type capWindow struct {
	Name string // e.g. "MediaWindow"
	Caps []capability
}

// ifaceInfo is one interface declared in package shell, with the import table
// of its declaring file (to resolve qualified types like image.Image).
type ifaceInfo struct {
	name    string
	it      *ast.InterfaceType
	imports map[string]string // local name → import path
}

type gen struct {
	fset    *token.FileSet
	ifaces  map[string]*ifaceInfo
	windows []capWindow
	caps    []capability
	// wrappers is the set of interfaces that need a Posted wrapper: reachable
	// from a capability and (transitively) carrying callbacks.
	wrappers map[string]bool
}

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	g := &gen{fset: token.NewFileSet(), ifaces: map[string]*ifaceInfo{}}
	g.parseShell(filepath.Join(root, "shell"))
	if len(g.caps) == 0 {
		fail(fmt.Errorf("no capability windows found under shell/"))
	}
	g.computeWrappers()
	g.writeWidget(root)
	g.writeApp(root)
	g.writePosted(root)
}

// moduleRoot walks up from the working directory to the dir holding go.mod.
func moduleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", fmt.Errorf("capgen: go.mod not found above %s", dir)
		}
		dir = parent
	}
}

// parseShell collects every interface in package shell, plus the capability
// windows and the deduplicated capability list.
func (g *gen) parseShell(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fail(err)
	}
	seen := map[string]capability{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") ||
			strings.HasSuffix(e.Name(), "_gen.go") {
			continue
		}
		f, err := parser.ParseFile(g.fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			fail(err)
		}
		imports := map[string]string{}
		for _, imp := range f.Imports {
			path, _ := strconv.Unquote(imp.Path.Value)
			name := filepath.Base(path)
			if imp.Name != nil {
				name = imp.Name.Name
			}
			imports[name] = path
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				ts, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				it, ok := ts.Type.(*ast.InterfaceType)
				if !ok {
					continue
				}
				g.ifaces[ts.Name.Name] = &ifaceInfo{name: ts.Name.Name, it: it, imports: imports}
				name := ts.Name.Name
				if name == "Window" || !strings.HasSuffix(name, "Window") {
					continue
				}
				if w, ok := capsOf(name, it); ok {
					g.windows = append(g.windows, w)
					for _, c := range w.Caps {
						seen[c.Field] = c
					}
				}
			}
		}
	}
	sort.Slice(g.windows, func(i, j int) bool { return g.windows[i].Name < g.windows[j].Name })
	for _, c := range seen {
		g.caps = append(g.caps, c)
	}
	sort.Slice(g.caps, func(i, j int) bool { return g.caps[i].Field < g.caps[j].Field })
}

// capsOf returns the capabilities of a candidate <X>Window interface, and false
// if any method isn't a zero-arg, single-interface-result getter (so it's not a
// capability provider).
func capsOf(name string, it *ast.InterfaceType) (capWindow, bool) {
	w := capWindow{Name: name}
	for _, m := range it.Methods.List {
		if len(m.Names) != 1 {
			return w, false // embedded interface, not a getter
		}
		ft, ok := m.Type.(*ast.FuncType)
		if !ok {
			return w, false
		}
		if ft.Params != nil && len(ft.Params.List) != 0 {
			return w, false
		}
		if ft.Results == nil || len(ft.Results.List) != 1 {
			return w, false
		}
		id, ok := ft.Results.List[0].Type.(*ast.Ident)
		if !ok {
			return w, false // qualified/pointer result — not a same-package capability
		}
		w.Caps = append(w.Caps, capability{Field: m.Names[0].Name, Type: id.Name})
	}
	if len(w.Caps) == 0 {
		return w, false
	}
	sort.Slice(w.Caps, func(i, j int) bool { return w.Caps[i].Field < w.Caps[j].Field })
	return w, true
}

// computeWrappers finds every shell interface reachable from a capability whose
// method set (transitively) carries a callback, via fixed-point iteration:
// wrappable(T) if a method of T has a func-typed param, or names a wrappable
// interface in a param, callback arg, or result.
func (g *gen) computeWrappers() {
	// Reachable: capability types plus interfaces named anywhere in their
	// method signatures, transitively.
	reachable := map[string]bool{}
	var reach func(name string)
	reach = func(name string) {
		if reachable[name] {
			return
		}
		info, ok := g.ifaces[name]
		if !ok {
			return
		}
		reachable[name] = true
		for _, m := range info.it.Methods.List {
			ft, ok := m.Type.(*ast.FuncType)
			if !ok {
				continue
			}
			ast.Inspect(ft, func(n ast.Node) bool {
				if id, ok := n.(*ast.Ident); ok {
					if _, isIface := g.ifaces[id.Name]; isIface {
						reach(id.Name)
					}
				}
				return true
			})
		}
	}
	for _, c := range g.caps {
		reach(c.Type)
	}

	// Wrappable: fixed point.
	wrappable := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name := range reachable {
			if wrappable[name] {
				continue
			}
			info := g.ifaces[name]
			need := false
			for _, m := range info.it.Methods.List {
				ft, ok := m.Type.(*ast.FuncType)
				if !ok {
					continue
				}
				ast.Inspect(ft, func(n ast.Node) bool {
					switch x := n.(type) {
					case *ast.FuncType:
						if x != ft {
							need = true // a callback parameter
						}
					case *ast.Ident:
						if wrappable[x.Name] {
							need = true
						}
					}
					return true
				})
			}
			if need {
				wrappable[name] = true
				changed = true
			}
		}
	}
	g.wrappers = map[string]bool{}
	for name := range reachable {
		if wrappable[name] {
			g.wrappers[name] = true
		}
	}
}

func (g *gen) exprStr(e ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, g.fset, e); err != nil {
		fail(err)
	}
	return b.String()
}

// fieldTypes expands a field list into one type per parameter/result (a field
// with n names contributes n entries).
func fieldTypes(fl *ast.FieldList) []ast.Expr {
	var out []ast.Expr
	if fl == nil {
		return out
	}
	for _, f := range fl.List {
		if _, ok := f.Type.(*ast.Ellipsis); ok {
			fail(fmt.Errorf("capgen: variadic capability methods are not supported"))
		}
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for i := 0; i < n; i++ {
			out = append(out, f.Type)
		}
	}
	return out
}

func (g *gen) writeWidget(root string) {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package widget\n\n")
	b.WriteString("import \"github.com/doug/gophics/shell\"\n\n")
	b.WriteString("// Capabilities holds the optional, platform-provided capabilities a Window may\n")
	b.WriteString("// expose (see shell/*.go). It is embedded in Owner; its fields are promoted, so\n")
	b.WriteString("// the Ctx accessors below read owner.<Cap> and apps read them via Ctx.<Cap>().\n")
	b.WriteString("// Regenerate with: go generate ./...\n")
	b.WriteString("type Capabilities struct {\n")
	for _, c := range g.caps {
		fmt.Fprintf(&b, "\t%s shell.%s\n", c.Field, c.Type)
	}
	b.WriteString("}\n\n")
	for _, c := range g.caps {
		fmt.Fprintf(&b, "// %s returns the platform %s capability (shell.%s), or nil when the running\n", c.Field, c.Field, c.Type)
		fmt.Fprintf(&b, "// platform can't provide it. See shell/*.go for its contract.\n")
		fmt.Fprintf(&b, "func (c Ctx) %s() shell.%s { return c.el.owner.%s }\n\n", c.Field, c.Type, c.Field)
	}
	writeFormatted(filepath.Join(root, "widget", "capabilities_gen.go"), b.Bytes())
}

func (g *gen) writeApp(root string) {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package app\n\n")
	b.WriteString("import (\n\t\"github.com/doug/gophics/shell\"\n\t\"github.com/doug/gophics/widget\"\n)\n\n")
	b.WriteString("// wireCapabilities publishes each capability the Window opts into (by\n")
	b.WriteString("// implementing the matching shell.<X>Window) to the widget Owner. A window that\n")
	b.WriteString("// doesn't implement a given interface leaves that capability nil. Callback-\n")
	b.WriteString("// carrying capabilities are wrapped in their shell.Posted<Cap> adapter so every\n")
	b.WriteString("// callback is delivered on the UI goroutine via Owner.Post, regardless of which\n")
	b.WriteString("// goroutine the platform implementation completes on.\n")
	b.WriteString("func wireCapabilities(o *widget.Owner, w shell.Window) {\n")
	for _, win := range g.windows {
		fmt.Fprintf(&b, "\tif x, ok := w.(shell.%s); ok {\n", win.Name)
		for _, c := range win.Caps {
			if g.wrappers[c.Type] {
				fmt.Fprintf(&b, "\t\to.%s = shell.Posted%s(x.%s(), o.Post)\n", c.Field, c.Type, c.Field)
			} else {
				fmt.Fprintf(&b, "\t\to.%s = x.%s()\n", c.Field, c.Field)
			}
		}
		b.WriteString("\t}\n")
	}
	b.WriteString("}\n")
	writeFormatted(filepath.Join(root, "app", "capabilities_gen.go"), b.Bytes())
}

func (g *gen) writePosted(root string) {
	names := make([]string, 0, len(g.wrappers))
	for n := range g.wrappers {
		names = append(names, n)
	}
	sort.Strings(names)

	// Collect external imports used by the wrapped method signatures.
	importPaths := map[string]bool{}
	for _, n := range names {
		info := g.ifaces[n]
		for _, m := range info.it.Methods.List {
			ast.Inspect(m.Type, func(node ast.Node) bool {
				if sel, ok := node.(*ast.SelectorExpr); ok {
					if x, ok := sel.X.(*ast.Ident); ok {
						if path, ok := info.imports[x.Name]; ok {
							importPaths[path] = true
						}
					}
				}
				return true
			})
		}
	}
	paths := make([]string, 0, len(importPaths))
	for p := range importPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package shell\n\n")
	if len(paths) > 0 {
		b.WriteString("import (\n")
		for _, p := range paths {
			fmt.Fprintf(&b, "\t%q\n", p)
		}
		b.WriteString(")\n\n")
	}
	for _, n := range names {
		g.writeWrapper(&b, n)
	}
	writeFormatted(filepath.Join(root, "shell", "posted_gen.go"), b.Bytes())
}

// writeWrapper emits Posted<T> + the forwarding struct for one interface.
func (g *gen) writeWrapper(b *bytes.Buffer, name string) {
	info := g.ifaces[name]
	fmt.Fprintf(b, "// Posted%s wraps inner so every callback it (or anything it hands out)\n", name)
	fmt.Fprintf(b, "// invokes is delivered through post — the app runner passes Owner.Post, making\n")
	fmt.Fprintf(b, "// the \"callbacks fire on the UI goroutine\" contract hold no matter which\n")
	fmt.Fprintf(b, "// goroutine the platform implementation completes on. Nil-safe: a nil inner\n")
	fmt.Fprintf(b, "// returns nil, and a nil post returns inner unwrapped (callbacks fire inline).\n")
	fmt.Fprintf(b, "func Posted%s(inner %s, post func(func())) %s {\n", name, name, name)
	fmt.Fprintf(b, "\tif inner == nil || post == nil {\n\t\treturn inner\n\t}\n")
	fmt.Fprintf(b, "\treturn posted%s{inner, post}\n}\n\n", name)
	fmt.Fprintf(b, "type posted%s struct {\n\tinner %s\n\tpost  func(func())\n}\n\n", name, name)

	for _, m := range info.it.Methods.List {
		if len(m.Names) != 1 {
			fail(fmt.Errorf("capgen: embedded interface in %s not supported", name))
		}
		g.writeMethod(b, name, m.Names[0].Name, m.Type.(*ast.FuncType))
	}
}

func (g *gen) writeMethod(b *bytes.Buffer, recv, mname string, ft *ast.FuncType) {
	params := fieldTypes(ft.Params)
	results := fieldTypes(ft.Results)

	// Signature.
	var sig []string
	for i, p := range params {
		sig = append(sig, fmt.Sprintf("a%d %s", i, g.exprStr(p)))
	}
	var res []string
	for _, r := range results {
		res = append(res, g.exprStr(r))
	}
	fmt.Fprintf(b, "func (p posted%s) %s(%s)", recv, mname, strings.Join(sig, ", "))
	if len(res) == 1 {
		fmt.Fprintf(b, " %s", res[0])
	} else if len(res) > 1 {
		fmt.Fprintf(b, " (%s)", strings.Join(res, ", "))
	}
	b.WriteString(" {\n")

	// Wrap func-typed params.
	callArgs := make([]string, len(params))
	for i, p := range params {
		callArgs[i] = fmt.Sprintf("a%d", i)
		cft, ok := p.(*ast.FuncType)
		if !ok {
			continue
		}
		if len(fieldTypes(cft.Results)) != 0 {
			fail(fmt.Errorf("capgen: callback with results in %s.%s not supported", recv, mname))
		}
		cparams := fieldTypes(cft.Params)
		var csig, cargs []string
		for j, cp := range cparams {
			csig = append(csig, fmt.Sprintf("c%d %s", j, g.exprStr(cp)))
			arg := fmt.Sprintf("c%d", j)
			// A callback argument that is itself a callback-carrying shell
			// interface (e.g. Recorder) is wrapped before the app sees it.
			if id, ok := cp.(*ast.Ident); ok && g.wrappers[id.Name] {
				arg = fmt.Sprintf("Posted%s(c%d, p.post)", id.Name, j)
			}
			cargs = append(cargs, arg)
		}
		fmt.Fprintf(b, "\tf%d := a%d\n", i, i)
		fmt.Fprintf(b, "\tvar w%d %s\n", i, g.exprStr(cft))
		fmt.Fprintf(b, "\tif f%d != nil {\n", i)
		fmt.Fprintf(b, "\t\tw%d = func(%s) { p.post(func() { f%d(%s) }) }\n",
			i, strings.Join(csig, ", "), i, strings.Join(cargs, ", "))
		b.WriteString("\t}\n")
		callArgs[i] = fmt.Sprintf("w%d", i)
	}

	// Call + wrap interface-typed results.
	call := fmt.Sprintf("p.inner.%s(%s)", mname, strings.Join(callArgs, ", "))
	if len(results) == 0 {
		fmt.Fprintf(b, "\t%s\n}\n\n", call)
		return
	}
	needsWrap := false
	for _, r := range results {
		if id, ok := r.(*ast.Ident); ok && g.wrappers[id.Name] {
			needsWrap = true
		}
	}
	if !needsWrap {
		fmt.Fprintf(b, "\treturn %s\n}\n\n", call)
		return
	}
	var rnames, rets []string
	for i, r := range results {
		rn := fmt.Sprintf("r%d", i)
		rnames = append(rnames, rn)
		if id, ok := r.(*ast.Ident); ok && g.wrappers[id.Name] {
			rets = append(rets, fmt.Sprintf("Posted%s(%s, p.post)", id.Name, rn))
		} else {
			rets = append(rets, rn)
		}
	}
	fmt.Fprintf(b, "\t%s := %s\n", strings.Join(rnames, ", "), call)
	fmt.Fprintf(b, "\treturn %s\n}\n\n", strings.Join(rets, ", "))
}

func writeFormatted(path string, src []byte) {
	out, err := format.Source(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "capgen: format %s: %v\n----\n%s\n----\n", path, err, src)
		os.Exit(1)
	}
	if err := os.WriteFile(path, out, 0o644); err != nil {
		fail(err)
	}
	fmt.Println("capgen: wrote", path)
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "capgen:", err)
	os.Exit(1)
}
