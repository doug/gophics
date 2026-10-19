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
//	widget/wire_gen.go          — wireCapabilities(owner, window): one type-assert
//	                              per <X>Window, publishing what the window exposes
//	widget/posted_gen.go        — Posted<Cap> wrappers that deliver every callback
//	                              through a post func (the UI-goroutine scheduler),
//	                              so shell implementations may invoke callbacks
//	                              from any goroutine and the documented
//	                              "callbacks fire on the UI goroutine" contract
//	                              is enforced centrally, not per platform
//	widget/extension_gen.go     — the out-of-tree seam: widget.NewCapability and
//	                              the registry Owner.WireCapabilities reads, so a
//	                              package in another module can declare a
//	                              capability without patching this tree. Only the
//	                              hollow probe derives from shell.Window; the rest
//	                              is invariant, and lives in the generator because
//	                              it reads generated plumbing (see writeExtension)
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
	"regexp"
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

// structInfo is one struct type declared in package shell — the "handlers"
// shape (a struct whose fields are callbacks), e.g. SocketHandlers.
type structInfo struct {
	name    string
	st      *ast.StructType
	imports map[string]string
}

type gen struct {
	fset    *token.FileSet
	ifaces  map[string]*ifaceInfo
	structs map[string]*structInfo
	windows []capWindow
	caps    []capability
	// wrappers is the set of interfaces that need a Posted<T> wrapper: reachable
	// from a capability and (transitively) carrying callbacks.
	wrappers map[string]bool
	// callbackStructs is the set of struct types that carry callbacks (a func
	// field, or a field that is itself wrappable) and so must be rebuilt with
	// each callback field posted when passed as a parameter.
	callbackStructs map[string]bool
	// shellDecls is every top-level type name declared in package shell, used to
	// qualify them when the wrappers are emitted into widget.
	shellDecls map[string]bool
}

// collectShellDecls records every type name declared in package shell.
//
// Not just the interfaces and structs parseShell keeps: a named enum
// (Permission, AppState, BiometricKind) appears in a wrapped signature and has
// to be qualified too. Missing one is a compile error in generated code rather
// than anything subtle, but the list has to be complete for the generator to be
// re-runnable without hand edits.
func (g *gen) collectShellDecls(dir string) {
	g.shellDecls = map[string]bool{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		fail(err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		f, err := parser.ParseFile(g.fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			fail(err)
		}
		for _, decl := range f.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.TYPE {
				continue
			}
			for _, spec := range gd.Specs {
				if ts, ok := spec.(*ast.TypeSpec); ok {
					g.shellDecls[ts.Name.Name] = true
				}
			}
		}
	}
}

func main() {
	root, err := moduleRoot()
	if err != nil {
		fail(err)
	}
	g := &gen{fset: token.NewFileSet(), ifaces: map[string]*ifaceInfo{}, structs: map[string]*structInfo{}}
	g.parseShell(filepath.Join(root, "shell"))
	g.collectShellDecls(filepath.Join(root, "shell"))
	if len(g.caps) == 0 {
		fail(fmt.Errorf("no capability windows found under shell/"))
	}
	g.computeWrappers()
	g.writeWidget(root)
	g.writeApp(root)
	g.writePosted(root)
	g.writeExtension(root)
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
				if st, ok := ts.Type.(*ast.StructType); ok {
					g.structs[ts.Name.Name] = &structInfo{name: ts.Name.Name, st: st, imports: imports}
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

// computeWrappers finds every shell interface or handler-struct reachable from a
// capability that (transitively) carries a callback, via fixed-point iteration.
// An interface is wrappable if a method has a func-typed param or names a
// wrappable interface / callback-struct; a struct is a callback-struct if a
// field is func-typed or names a wrappable interface / callback-struct. The two
// sets are mutually recursive (an interface whose method takes a handler struct;
// a handler struct with a field that is a wrappable interface), so they're
// computed together and then split into wrappers (interfaces) and callbackStructs.
func (g *gen) computeWrappers() {
	// Reachable: capability types plus every shell type named in their
	// signatures / fields, transitively (interfaces and structs both).
	reachable := map[string]bool{}
	var reach func(name string)
	reach = func(name string) {
		if reachable[name] {
			return
		}
		var nodes []ast.Node
		if info, ok := g.ifaces[name]; ok {
			for _, m := range info.it.Methods.List {
				nodes = append(nodes, m.Type)
			}
		} else if s, ok := g.structs[name]; ok {
			for _, f := range s.st.Fields.List {
				nodes = append(nodes, f.Type)
			}
		} else {
			return // builtin / external type
		}
		reachable[name] = true
		for _, n := range nodes {
			ast.Inspect(n, func(nn ast.Node) bool {
				if id, ok := nn.(*ast.Ident); ok {
					reach(id.Name)
				}
				return true
			})
		}
	}
	for _, c := range g.caps {
		reach(c.Type)
	}

	// wrap holds both wrappable interfaces and callback structs; fixed point.
	wrap := map[string]bool{}
	for changed := true; changed; {
		changed = false
		for name := range reachable {
			if wrap[name] {
				continue
			}
			need := false
			if info, ok := g.ifaces[name]; ok {
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
							if wrap[x.Name] {
								need = true
							}
						}
						return true
					})
				}
			} else if s, ok := g.structs[name]; ok {
				for _, f := range s.st.Fields.List {
					switch t := f.Type.(type) {
					case *ast.FuncType:
						need = true
					case *ast.Ident:
						if wrap[t.Name] {
							need = true
						}
					}
				}
			}
			if need {
				wrap[name] = true
				changed = true
			}
		}
	}
	g.wrappers = map[string]bool{}
	g.callbackStructs = map[string]bool{}
	for name := range reachable {
		if !wrap[name] {
			continue
		}
		if g.ifaces[name] != nil {
			g.wrappers[name] = true
		} else if g.structs[name] != nil {
			g.callbackStructs[name] = true
		}
	}
}

// funcFields returns the (name, type) of each func-typed field of a struct, in
// source order — the callbacks that a Posted wrapper must re-post.
func (g *gen) funcFields(name string) [][2]any {
	var out [][2]any
	s := g.structs[name]
	if s == nil {
		return out
	}
	for _, f := range s.st.Fields.List {
		ft, ok := f.Type.(*ast.FuncType)
		if !ok {
			continue
		}
		for _, n := range f.Names {
			out = append(out, [2]any{n.Name, ft})
		}
	}
	return out
}

// postedClosure returns a closure literal that posts a call to origExpr — the
// building block reused by both direct callback params and struct callback
// fields. A callback argument that is itself a wrappable shell interface is
// wrapped through Posted<T> before the app sees it.
func (g *gen) postedClosure(origExpr string, cft *ast.FuncType) string {
	if len(fieldTypes(cft.Results)) != 0 {
		fail(fmt.Errorf("capgen: callback with results (%s) not supported", g.exprStr(cft)))
	}
	cparams := fieldTypes(cft.Params)
	var csig, cargs []string
	for j, cp := range cparams {
		csig = append(csig, fmt.Sprintf("c%d %s", j, g.qualify(g.exprStr(cp))))
		arg := fmt.Sprintf("c%d", j)
		if id, ok := cp.(*ast.Ident); ok && g.wrappers[id.Name] {
			arg = fmt.Sprintf("posted%sOf(c%d, p.post)", id.Name, j)
		}
		cargs = append(cargs, arg)
	}
	return fmt.Sprintf("func(%s) { p.post(func() { %s(%s) }) }",
		strings.Join(csig, ", "), origExpr, strings.Join(cargs, ", "))
}

func (g *gen) exprStr(e ast.Expr) string {
	var b bytes.Buffer
	if err := printer.Fprint(&b, g.fset, e); err != nil {
		fail(err)
	}
	return b.String()
}

// qualify rewrites bare shell type names as shell-qualified ones.
//
// The posted wrappers are parsed out of package shell, where its own types are
// written bare, and emitted into package widget, where they are not. Rewriting
// the rendered string is enough because the input is a type expression: the
// identifiers in it are type names, and the ones that need qualifying are
// exactly the ones declared in shell.
func (g *gen) qualify(s string) string {
	return identRE.ReplaceAllStringFunc(s, func(id string) string {
		if g.shellDecls[id] {
			return "shell." + id
		}
		return id
	})
}

// identRE matches a Go identifier not already preceded by a dot, so an
// already-qualified name (time.Duration) is left alone.
var identRE = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*\b`)

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
		fmt.Fprintf(&b, "\t%s shell.%s\n", lowerFirst(c.Field), c.Type)
	}
	// The out-of-tree capabilities cannot be fields: this file is generated from
	// shell/*.go, and a capability declared in somebody else's module is by
	// definition not in there. They live in one slice indexed by registration
	// slot instead, which is why Capability[T].Of is a type assertion rather
	// than a field read. See extension_gen.go for the whole mechanism.
	b.WriteString("\t// extCaps holds the capabilities registered from outside this module, in\n")
	b.WriteString("\t// registration order (see NewCapability). Nil until a Window is wired, and\n")
	b.WriteString("\t// re-read on every WireCapabilities, exactly like the fields above.\n")
	b.WriteString("\textCaps []any\n")
	b.WriteString("}\n\n")
	for _, c := range g.caps {
		fmt.Fprintf(&b, "// %s returns the platform %s capability (shell.%s), or nil when the running\n", c.Field, c.Field, c.Type)
		fmt.Fprintf(&b, "// platform can't provide it. See shell/*.go for its contract.\n")
		fmt.Fprintf(&b, "func (c Ctx) %s() shell.%s { return c.el.owner.%s }\n\n", c.Field, c.Type, lowerFirst(c.Field))
	}
	writeFormatted(filepath.Join(root, "widget", "capabilities_gen.go"), b.Bytes())
}

func (g *gen) writeApp(root string) {
	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package widget\n\n")
	b.WriteString("import \"github.com/doug/gophics/shell\"\n\n")
	b.WriteString("// WireCapabilities publishes each capability the Window opts into (by\n")
	b.WriteString("// implementing the matching shell.<X>Window) to the widget Owner. A window that\n")
	b.WriteString("// doesn't implement a given interface leaves that capability nil. Callback-\n")
	b.WriteString("// carrying capabilities are wrapped in a posted adapter so every\n")
	b.WriteString("// callback is delivered on the UI goroutine via Owner.Post, regardless of which\n")
	b.WriteString("// goroutine the platform implementation completes on.\n")
	b.WriteString("// It is a method on Owner rather than a function in app because it uses\n")
	b.WriteString("// nothing from app: only shell, the Owner it writes to, and Owner.Post. Living\n")
	b.WriteString("// here is what lets the Capabilities fields and the posted wrappers stay\n")
	b.WriteString("// unexported \xe2\x80\x94 an app names ctx.<Cap>(), never the plumbing.\n")
	b.WriteString("func (o *Owner) WireCapabilities(w shell.Window) {\n")
	for _, win := range g.windows {
		fmt.Fprintf(&b, "\tif x, ok := w.(shell.%s); ok {\n", win.Name)
		for _, c := range win.Caps {
			if g.wrappers[c.Type] {
				fmt.Fprintf(&b, "\t\to.%s = posted%sOf(x.%s(), o.Post)\n", lowerFirst(c.Field), c.Type, c.Field)
			} else {
				fmt.Fprintf(&b, "\t\to.%s = x.%s()\n", lowerFirst(c.Field), c.Field)
			}
		}
		b.WriteString("\t}\n")
	}
	// Out-of-tree capabilities are wired from the registry rather than from a
	// generated type-assert, because their <X>Window interfaces are not in this
	// module and so cannot be named here. Same contract: published when the
	// window provides one, left nil when it does not.
	b.WriteString("\to.wireExtCapabilities(w)\n")
	b.WriteString("}\n")
	// The runtime reads a few capabilities directly — the app runner publishes
	// the accessibility tree, for instance — and it is a different package, so
	// the unexported fields need readers. One per capability, generated, rather
	// than re-exporting the fields: a reader is a promise about a value, a field
	// is a promise about a layout.
	b.WriteString("\n")
	for _, c := range g.caps {
		fmt.Fprintf(&b, "// %s returns the platform %s capability, or nil when the running\n", c.Field, c.Field)
		fmt.Fprintf(&b, "// platform cannot provide it. Widgets read it through Ctx.%s().\n", c.Field)
		fmt.Fprintf(&b, "func (o *Owner) %s() shell.%s { return o.%s }\n\n", c.Field, c.Type, lowerFirst(c.Field))
	}
	writeFormatted(filepath.Join(root, "widget", "wire_gen.go"), b.Bytes())
}

func (g *gen) writePosted(root string) {
	names := make([]string, 0, len(g.wrappers))
	for n := range g.wrappers {
		names = append(names, n)
	}
	sort.Strings(names)

	// Collect external imports used by the wrapped method signatures — and by
	// the fields of any callback-struct they pass (a handler field may name a
	// qualified type like time.Duration).
	importPaths := map[string]bool{}
	collect := func(node ast.Node, imports map[string]string) {
		ast.Inspect(node, func(nn ast.Node) bool {
			if sel, ok := nn.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if path, ok := imports[x.Name]; ok {
						importPaths[path] = true
					}
				}
			}
			return true
		})
	}
	for _, n := range names {
		info := g.ifaces[n]
		for _, m := range info.it.Methods.List {
			collect(m.Type, info.imports)
		}
	}
	for name := range g.callbackStructs {
		s := g.structs[name]
		for _, f := range s.st.Fields.List {
			collect(f.Type, s.imports)
		}
	}
	paths := make([]string, 0, len(importPaths))
	for p := range importPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package widget\n\n")
	b.WriteString("import (\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "\t%q\n", p)
	}
	b.WriteString("\n\t\"github.com/doug/gophics/shell\"\n")
	b.WriteString(")\n\n")
	for _, n := range names {
		g.writeWrapper(&b, n)
	}
	writeFormatted(filepath.Join(root, "widget", "posted_gen.go"), b.Bytes())
}

// writeExtension emits widget/extension_gen.go: the seam through which a
// package *outside* this module can declare a capability and have it reach the
// widget tree.
//
// Why this file is generated at all, when almost none of it varies with the
// shell interfaces: the storage it reads (Capabilities.extCaps) and the wiring
// that fills it (Owner.WireCapabilities) are generated, and a hand-written file
// that reached into generated plumbing would make the generator's output no
// longer self-contained — change the field name and a hand-written file breaks
// at a distance. The one genuinely derived part is capProbe, whose method set
// is shell.Window's: it is the window handed to a provider at registration
// time to prove the provider is answering the *window* rather than inventing a
// capability out of nothing, so it has to track shell.Window exactly. Writing
// those stubs by hand means a new Window method silently turns the hollow gate
// into a compile error in a file nobody associates with it.
func (g *gen) writeExtension(root string) {
	win := g.ifaces["Window"]
	if win == nil {
		fail(fmt.Errorf("capgen: shell.Window not found; the hollow probe cannot be generated"))
	}

	// capProbe's signatures may name types from the files Window is declared
	// in (geom, image, ...), so carry their imports across.
	// "fmt" is used by CheckAbsent's message; the rest come from shell.Window's
	// own signatures, since capProbe restates them.
	importPaths := map[string]bool{"fmt": true}
	for _, m := range win.it.Methods.List {
		ast.Inspect(m.Type, func(nn ast.Node) bool {
			if sel, ok := nn.(*ast.SelectorExpr); ok {
				if x, ok := sel.X.(*ast.Ident); ok {
					if path, ok := win.imports[x.Name]; ok {
						importPaths[path] = true
					}
				}
			}
			return true
		})
	}
	paths := make([]string, 0, len(importPaths))
	for p := range importPaths {
		paths = append(paths, p)
	}
	sort.Strings(paths)

	var b bytes.Buffer
	b.WriteString("// Code generated by internal/capgen; DO NOT EDIT.\n\n")
	b.WriteString("package widget\n\n")
	b.WriteString("import (\n")
	for _, p := range paths {
		fmt.Fprintf(&b, "\t%q\n", p)
	}
	b.WriteString("\n\t\"github.com/doug/gophics/shell\"\n")
	b.WriteString(")\n\n")
	b.WriteString(extensionBody)

	// capProbe: a Window that can do nothing but be a Window. Generated from
	// shell.Window so it cannot drift from it.
	b.WriteString("// capProbe is a Window that implements the base contract and nothing else:\n")
	b.WriteString("// no <X>Window, in this module or any other. Its methods are generated from\n")
	b.WriteString("// shell.Window, so adding a method there does not quietly stop the probe from\n")
	b.WriteString("// compiling as one.\n")
	b.WriteString("//\n")
	b.WriteString("// It is never a real window and never reaches a shell; it exists only so\n")
	b.WriteString("// NewCapability can ask a provider what it answers for a window that provides\n")
	b.WriteString("// nothing. The honest answer is nil.\n")
	b.WriteString("type capProbe struct{}\n\n")
	// Asserted, not assumed: if the probe stopped satisfying shell.Window the
	// hollow gate would still compile — capProbe{} would simply fail to be
	// passed — and this says so at the declaration instead.
	b.WriteString("var _ shell.Window = capProbe{}\n\n")
	for _, m := range win.it.Methods.List {
		if len(m.Names) != 1 {
			fail(fmt.Errorf("capgen: embedded interface in shell.Window not supported"))
		}
		ft, ok := m.Type.(*ast.FuncType)
		if !ok {
			fail(fmt.Errorf("capgen: shell.Window member %v is not a method", m.Names))
		}
		var params []string
		for _, p := range fieldTypes(ft.Params) {
			params = append(params, fmt.Sprintf("_ %s", g.qualify(g.exprStr(p))))
		}
		results := fieldTypes(ft.Results)
		var res []string
		for i, r := range results {
			res = append(res, fmt.Sprintf("r%d %s", i, g.qualify(g.exprStr(r))))
		}
		fmt.Fprintf(&b, "func (capProbe) %s(%s)", m.Names[0].Name, strings.Join(params, ", "))
		if len(res) > 0 {
			// Named results plus a bare return is the one spelling that yields
			// the zero value of every type — including an interface, a struct
			// and a channel — without the generator having to know how to write
			// a zero literal for each.
			fmt.Fprintf(&b, " (%s) { return }\n", strings.Join(res, ", "))
		} else {
			b.WriteString(" {}\n")
		}
	}
	writeFormatted(filepath.Join(root, "widget", "extension_gen.go"), b.Bytes())
}

// extensionBody is the invariant half of extension_gen.go. It is a literal
// rather than a template because nothing in it derives from the shell
// interfaces; keeping it verbatim means the file reads, and reviews, like the
// hand-written code it would otherwise be.
const extensionBody = `// Out-of-tree capabilities.
//
// The capabilities in capabilities_gen.go are generated by parsing shell/*.go,
// which makes this module's own source the single source of truth — and means a
// package outside it could not add a capability at all without patching the
// tree. That caps the ecosystem at whatever lives in this repository. This file
// is the seam that lifts the cap.
//
// A declaring package writes the same two declarations an in-tree capability
// does — the capability interface and its <X>Window opt-in — in its own
// package, then registers a key:
//
//	package barcode // example.com/barcode, a different module
//
//	type Scanner interface{ Scan(func(code string)) }
//	type ScannerWindow interface{ Scanner() Scanner }
//
//	var Cap = widget.NewCapability[Scanner]("example.com/barcode.Scanner",
//	    func(w shell.Window, post func(func())) Scanner {
//	        x, ok := w.(ScannerWindow)
//	        if !ok {
//	            return nil // this platform's shell cannot scan
//	        }
//	        return postedScanner(x.Scanner(), post)
//	    })
//
// and an app reads it exactly as it reads a built-in one, nil and all:
//
//	if s := barcode.Cap.Of(ctx); s != nil { s.Scan(onCode) }
//
// Four properties are worth being explicit about, because they are the ones
// that would otherwise rot:
//
// Nil stays a real answer. T must be an interface type, enforced at
// registration, so that a nil T can carry "this platform cannot" the way
// ctx.Battery() == nil does. A pointer or struct capability has no value that
// means absent — a nil *Scanner boxed in an any is not nil — so those are
// refused rather than quietly reported as present everywhere.
//
// A hollow capability is refused, loudly. The provider is called once at
// registration with capProbe, a window that implements the base contract and
// no capability at all. A provider that answers *that* with a capability is
// answering regardless of what the platform can do, which is the one shape a
// caller cannot detect: the nil check passes, the calls succeed, nothing
// happens. NewCapability panics, during package initialization, before any
// window exists — so the declaring package's own tests fail on it rather than
// its users' apps misbehaving.
//
// That gate catches the hollow provider; it cannot, by construction, catch a
// hollow *wrapper*. The second way absence leaks away is a posted wrapper that
// is not nil-safe: the provider asserts the window correctly, the window
// implements the opt-in and returns nil (the ordinary way a shell says "not on
// this device"), and the wrapper hands back a live-looking wrapper around
// nothing. Reaching that branch needs a window implementing the declaring
// package's own <X>Window interface, which this package cannot name. So the
// declaring package tests it, with CheckAbsent, naming every window shape that
// should answer nil. Treat that as part of registering a capability, not as
// optional polish.
//
// Callbacks still fire on the UI goroutine, but the declaring package owns
// that. capgen generates the Posted wrappers for in-tree capabilities by
// parsing their source, which it cannot do for a type in another module, so
// the scheduler is handed to the provider as post instead and the provider
// wraps its own callbacks with it. post is nil on a headless Owner with no
// runner, and a wrapper must then return its inner value unwrapped — callbacks
// fire inline — which is what the generated in-tree wrappers do.
//
// No reflection and no plugin loading is involved: registration is an ordinary
// package-level var in a package the app imports, so an app that does not
// import it links none of it, and the lookup is a type assertion on an
// interface value rather than a reflect.Type key.

// Capability is the key for a platform capability declared outside this
// module. NewCapability returns one; Of reads it for an app.
//
// A key, rather than a generated accessor method on Ctx, because a method on
// Ctx can only be written in this package. The key is the only way a type in
// somebody else's module can be named in the lookup's type while staying
// statically typed.
type Capability[T any] struct {
	name string
	slot int
	// provide is the registered provider, still typed. The registry below
	// holds a type-erased copy for the wiring loop; this one is what lets
	// ProvidedBy hand a declaring package its own type back.
	provide func(w shell.Window, post func(func())) T
}

// extProvider is one registration: the declaring package's name for the
// capability, and the type-erased provider the wiring calls per window.
//
// Type-erased because the registry is one slice holding every registered
// capability and they have different types; the type comes back on the way out,
// in Capability[T].Of, where T is known statically again.
type extProvider struct {
	name    string
	provide func(w shell.Window, post func(func())) any
}

// extProviders is the registry, in registration order — a Capability's slot is
// its index.
//
// Appended to only from NewCapability, which a declaring package calls from a
// package-level var, so every registration happens during program
// initialization and is complete before main runs. It is read afterwards, from
// the UI goroutine, and never written again; that is what makes a plain slice
// with no lock correct here.
var extProviders []extProvider

// NewCapability registers an out-of-tree capability and returns its key. Call
// it once, from a package-level var in the package that declares the
// capability.
//
// name identifies the capability for diagnostics and duplicate detection; use
// a fully qualified one (import path plus type) so two modules cannot collide.
//
// provide is handed the live Window and the UI-goroutine scheduler and returns
// the capability, or nil when the running platform's shell does not provide it.
// It must be a type assertion and a wrap, nothing more: it runs once at
// registration against a probe window, and again for every window the app
// wires, so it must have no side effects.
//
// It panics — at initialization, deterministically, in the declaring package's
// own tests — when T is not an interface type, when name is empty or already
// registered, or when provide answers a window that provides nothing. The last
// is the hollow check; see the package notes above for why it is a panic and
// not a returned error.
func NewCapability[T any](name string, provide func(w shell.Window, post func(func())) T) *Capability[T] {
	if name == "" {
		panic("widget.NewCapability: a capability needs a name; use its import path and type, e.g. \"example.com/barcode.Scanner\"")
	}
	if provide == nil {
		panic("widget.NewCapability: " + name + ": provide must not be nil")
	}
	var zero T
	if any(zero) != nil {
		panic("widget.NewCapability: " + name + ": T must be an interface type, so that a nil T can mean \"this platform cannot provide it\"; a struct or pointer capability has no value that means absent")
	}
	for _, p := range extProviders {
		if p.name == name {
			panic("widget.NewCapability: " + name + " is already registered; two packages cannot share a capability name")
		}
	}
	if v := provide(capProbe{}, func(fn func()) { fn() }); any(v) != nil {
		panic("widget.NewCapability: " + name + " is hollow: it returned a capability for a Window that provides nothing, so every platform would report it as present and none of it would work. Return nil when the Window does not implement your <X>Window interface, and make any posted wrapper nil-safe")
	}
	k := &Capability[T]{name: name, slot: len(extProviders), provide: provide}
	extProviders = append(extProviders, extProvider{
		name: name,
		provide: func(w shell.Window, post func(func())) any {
			// A nil T converts to a nil any, so absence survives the erasure.
			return provide(w, post)
		},
	})
	return k
}

// Of returns the capability for ctx's app, or nil when the running platform
// does not provide it — the same contract as the generated Ctx accessors, and
// the same caveat: capabilities are nil while the tree mounts and appear on the
// build after the shell wires them.
func (k *Capability[T]) Of(c Ctx) T {
	var zero T
	if k == nil || c.el == nil {
		return zero
	}
	ext := c.el.owner.extCaps
	if k.slot >= len(ext) {
		return zero // no window wired yet, or this Owner predates the registration
	}
	v, _ := ext[k.slot].(T)
	return v
}

// ProvidedBy returns what this capability's provider answers for w, without an
// Owner, a tree or a running app — the primitive a declaring package's tests
// are written on. CheckAbsent is the assertion built from it, and is what most
// tests want; this one also answers the other direction, that a window which
// does provide the capability gets it.
//
// Callbacks delivered during the call run inline, so a test needs no
// scheduler. Nothing is registered or published; it only calls the provider.
func (k *Capability[T]) ProvidedBy(w shell.Window) T {
	var zero T
	if k == nil || k.provide == nil {
		return zero
	}
	return k.provide(w, func(fn func()) { fn() })
}

// CheckAbsent reports an error when the capability answers any of windows with
// something other than nil. It is the hollow assertion, named so that a
// declaring package makes it in one line rather than inventing its own wording:
//
//	func TestNotHollow(t *testing.T) {
//	    err := Cap.CheckAbsent(
//	        bareWindow{},                // a platform with no scanner at all
//	        scannerWindow{scanner: nil}, // one whose scanner is disabled
//	    )
//	    if err != nil {
//	        t.Fatal(err)
//	    }
//	}
//
// Pass every window shape that must answer nil. The disabled-scanner window is
// the one NewCapability cannot build for itself — it needs the declaring
// package's own opt-in interface — and the one that catches a wrapper that is
// not nil-safe. A plugin whose test omits it is a plugin that can go hollow
// without anybody noticing.
func (k *Capability[T]) CheckAbsent(windows ...shell.Window) error {
	if k == nil {
		return fmt.Errorf("widget: CheckAbsent on a nil capability key")
	}
	for _, w := range windows {
		if v := k.ProvidedBy(w); any(v) != nil {
			return fmt.Errorf("widget: capability %s is hollow: %T cannot provide it, yet the provider answered with %T — an app's nil check passes, the calls succeed and nothing happens", k.name, w, v)
		}
	}
	return nil
}

// Name returns the name the capability was registered under.
func (k *Capability[T]) Name() string {
	if k == nil {
		return ""
	}
	return k.name
}

// RegisteredCapabilities returns the names of every out-of-tree capability
// registered in this binary, in registration order.
//
// For diagnostics: it answers "is the plugin I imported actually linked in",
// which is otherwise invisible — an unused import is a compile error, so the
// usual way to get this wrong is to forget the import entirely and read nil
// forever.
func RegisteredCapabilities() []string {
	names := make([]string, len(extProviders))
	for i, p := range extProviders {
		names[i] = p.name
	}
	return names
}

// wireExtCapabilities re-reads every registered out-of-tree capability from w.
// WireCapabilities calls it last, so one event (shell.CapabilitiesChanged)
// refreshes in-tree and out-of-tree capabilities together.
func (o *Owner) wireExtCapabilities(w shell.Window) {
	if len(extProviders) == 0 {
		o.extCaps = nil
		return
	}
	if len(o.extCaps) != len(extProviders) {
		o.extCaps = make([]any, len(extProviders))
	}
	for i, p := range extProviders {
		o.extCaps[i] = p.provide(w, o.Post)
	}
}

`

// writeWrapper emits Posted<T> + the forwarding struct for one interface.
func (g *gen) writeWrapper(b *bytes.Buffer, name string) {
	info := g.ifaces[name]
	fmt.Fprintf(b, "// posted%s wraps inner so every callback it (or anything it hands out)\n", name)
	fmt.Fprintf(b, "// invokes is delivered through post — the app runner passes Owner.Post, making\n")
	fmt.Fprintf(b, "// the \"callbacks fire on the UI goroutine\" contract hold no matter which\n")
	fmt.Fprintf(b, "// goroutine the platform implementation completes on. Nil-safe: a nil inner\n")
	fmt.Fprintf(b, "// returns nil, and a nil post returns inner unwrapped (callbacks fire inline).\n")
	fmt.Fprintf(b, "func posted%sOf(inner shell.%s, post func(func())) shell.%s {\n", name, name, name)
	fmt.Fprintf(b, "\tif inner == nil || post == nil {\n\t\treturn inner\n\t}\n")
	fmt.Fprintf(b, "\treturn posted%s{inner, post}\n}\n\n", name)
	fmt.Fprintf(b, "type posted%s struct {\n\tinner shell.%s\n\tpost  func(func())\n}\n\n", name, name)

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
		sig = append(sig, fmt.Sprintf("a%d %s", i, g.qualify(g.exprStr(p))))
	}
	var res []string
	for _, r := range results {
		res = append(res, g.qualify(g.exprStr(r)))
	}
	fmt.Fprintf(b, "func (p posted%s) %s(%s)", recv, mname, strings.Join(sig, ", "))
	if len(res) == 1 {
		fmt.Fprintf(b, " %s", res[0])
	} else if len(res) > 1 {
		fmt.Fprintf(b, " (%s)", strings.Join(res, ", "))
	}
	b.WriteString(" {\n")

	// Wrap callback-carrying params: a direct func param, or a handler-struct
	// param (its func fields are re-posted via a rebuilt copy).
	callArgs := make([]string, len(params))
	for i, p := range params {
		callArgs[i] = fmt.Sprintf("a%d", i)
		switch pt := p.(type) {
		case *ast.FuncType:
			fmt.Fprintf(b, "\tf%d := a%d\n", i, i)
			fmt.Fprintf(b, "\tvar w%d %s\n", i, g.qualify(g.exprStr(pt)))
			fmt.Fprintf(b, "\tif f%d != nil {\n\t\tw%d = %s\n\t}\n",
				i, i, g.postedClosure(fmt.Sprintf("f%d", i), pt))
			callArgs[i] = fmt.Sprintf("w%d", i)
		case *ast.Ident:
			if !g.callbackStructs[pt.Name] {
				continue
			}
			// Rebuild the struct with every callback field posted (non-func
			// fields carry over via the copy).
			fmt.Fprintf(b, "\ts%d := a%d\n", i, i)
			fmt.Fprintf(b, "\tw%d := s%d\n", i, i)
			for _, ff := range g.funcFields(pt.Name) {
				fname := ff[0].(string)
				cft := ff[1].(*ast.FuncType)
				src := fmt.Sprintf("s%d.%s", i, fname)
				fmt.Fprintf(b, "\tif %s != nil {\n\t\tg := %s\n\t\tw%d.%s = %s\n\t}\n",
					src, src, i, fname, g.postedClosure("g", cft))
			}
			callArgs[i] = fmt.Sprintf("w%d", i)
		}
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
			rets = append(rets, fmt.Sprintf("posted%sOf(%s, p.post)", id.Name, rn))
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
	if !quiet {
		fmt.Println("capgen: wrote", path)
	}
}

// quiet silences the per-file progress lines. They are what `go generate` has
// always printed and worth keeping there; the generator's own tests write four
// files into a temp dir and the lines are then only noise in front of a
// failure.
var quiet bool

func fail(err error) {
	fmt.Fprintln(os.Stderr, "capgen:", err)
	os.Exit(1)
}

// lowerFirst unexports a name: the Capabilities fields and posted wrappers are
// plumbing that only generated code in this package writes.
func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToLower(s[:1]) + s[1:]
}
