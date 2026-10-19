// Package apisurface enumerates the module's public API, with signatures.
//
// gophics promises "a small surface and the Go 1 compatibility ethos after
// 1.0". A promise about a number needs the number to be measurable, and every
// refactor that claims to shrink it needs a diff rather than an assertion — so
// the surface is written to internal/apisurface/testdata/api-surface.txt and a
// test fails when the
// tree and the file disagree.
//
// What counts as surface is what a caller can name and a change can break:
// exported top-level objects, exported methods on exported named types, and
// exported struct fields and interface methods — each with its full signature.
// A struct field is API — widget.Text.Wrap is something apps write.
//
// Signatures are recorded because for most of this file's life they were not,
// and that made the gate blind to the most common way to break a caller.
// Changing a parameter type, adding a result, retyping a struct field or
// adding a method to an interface all left the manifest byte-identical: the
// old format recorded that widget.Text.Wrap was a field, not that it was a
// bool. The build stayed green because nothing in this repo had to call the
// changed thing the old way — only somebody else's app did, and it is not here
// to fail. A compatibility gate that cannot see a signature is a gate against
// renames, which are the breaks a compiler finds anyway.
//
// Two things are deliberately excluded. internal/ and examples/ are not API.
// shell/mobile is not either: its ~130 exported Bridge methods exist because
// gomobile requires them, they are called only from Kotlin and Swift, and they
// are versioned with the host app rather than with this module — counting them
// would swamp the number that the promise is about.
//
// See README.md in this directory for the manifest format, the stability
// tiers, and what the gate can and cannot see.
package apisurface

import (
	"fmt"
	"go/constant"
	"go/types"
	"math"
	"os"
	"slices"
	"sort"
	"strconv"
	"strings"

	"golang.org/x/tools/go/packages"
)

// modulePrefix is this module's import path. Types from inside the module are
// qualified by their module-relative package path, so a line reads the way an
// import of it reads; everything else is qualified by package name, the way
// source does.
const modulePrefix = "github.com/doug/gophics/"

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

// Entry is one API element, rendered as a stable one-line string:
//
//	<package>.<Name>[.<Member>] <kind> <signature>
//
// The first two fields are the key: the thing a caller names, and what sort of
// thing it is. Neither contains a space, so the split is unambiguous, and the
// signature holds everything else. Sorting by the whole line therefore sorts
// by key, which is what makes a signature change read as a -/+ pair of
// adjacent lines rather than as an unrelated removal and addition pages apart.
type Entry string

// Key is the dotted name, without kind or signature.
func (e Entry) Key() string {
	k, _, _ := strings.Cut(string(e), " ")
	return k
}

// Kind is one of type, alias, field, method, ifacemethod, func, const, var.
//
// ifacemethod is separate from method on purpose: adding a method to an
// exported interface is a new line, which reads like an addition, but it
// breaks every outside implementation of that interface. The kind is what lets
// Diff call that what it is.
func (e Entry) Kind() string {
	rest, _, _ := strings.Cut(strings.TrimPrefix(string(e), e.Key()+" "), " ")
	return rest
}

// Signature is the type information: parameters and results for a callable,
// the type for a field or var, the underlying type for a defined type, the
// value for a constant. Empty for kinds that have none.
func (e Entry) Signature() string {
	s := string(e)
	s = strings.TrimPrefix(s, e.Key()+" ")
	s = strings.TrimPrefix(s, e.Kind())
	return strings.TrimPrefix(s, " ")
}

// Package is the module-relative package path the entry belongs to.
//
// It is the key up to the first dot after the last slash: package paths use
// slashes and the names inside them use dots, so "shell/desktop.Run" splits
// where it should and "sound/mp3.Decode" does not lose its directory.
func (e Entry) Package() string {
	k := e.Key()
	start := strings.LastIndex(k, "/") + 1
	if i := strings.Index(k[start:], "."); i >= 0 {
		return k[:start+i]
	}
	return k
}

func entry(key, kind, sig string) Entry {
	if sig == "" {
		return Entry(key + " " + kind)
	}
	return Entry(key + " " + kind + " " + sig)
}

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
	return sorted(seen), nil
}

// CollectTarget returns the surface of the module rooted at dir under one
// build configuration.
//
// The tests for the extractor itself use this: they build a throwaway module
// with one package in it, and scanning it six times to prove that a parameter
// type is recorded would cost six package loads to learn the same thing once.
// Collect is still what the manifest is taken with, because a single target
// cannot see the other platforms' shells.
func CollectTarget(dir, goos, goarch string) ([]Entry, error) {
	seen := map[Entry]bool{}
	if err := collectInto(seen, dir, goos, goarch); err != nil {
		return nil, err
	}
	return sorted(seen), nil
}

func sorted(seen map[Entry]bool) []Entry {
	out := make([]Entry, 0, len(seen))
	for e := range seen {
		out = append(out, e)
	}
	slices.Sort(out)
	return out
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
		key := short + "." + name
		switch o := obj.(type) {
		case *types.Func:
			out = append(out, entry(key, "func", funcSig(o.Signature(), nil)))
		case *types.Const:
			out = append(out, entry(key, "const", typeString(o.Type())+" = "+constValue(o)))
		case *types.Var:
			out = append(out, entry(key, "var", typeString(o.Type())))
		case *types.TypeName:
			out = append(out, typeEntries(key, o)...)
		default:
			out = append(out, entry(key, "object", typeString(obj.Type())))
		}
	}
	return out
}

// typeEntries renders a defined type, its exported fields or interface
// methods, and its exported method set.
func typeEntries(key string, tn *types.TypeName) []Entry {
	// An alias is recorded as what it points at — unaliased, because the alias
	// name is what every *other* line already says and "X alias X" records
	// nothing. Retargeting one is then a signature change on a line that
	// otherwise never moves, and every reference to it stays put.
	if tn.IsAlias() {
		// One step, not all the way down: `type Widget = any` is recorded as
		// "any", which is what the declaration says, where a full Unalias
		// would resolve through any's own alias and print interface{}.
		target := types.Unalias(tn.Type())
		if a, ok := tn.Type().(*types.Alias); ok {
			target = a.Rhs()
		}
		return []Entry{entry(key, "alias", typeString(target))}
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		// A type parameter's name, or anything else go/types does not hand
		// back as named. Record the type and move on.
		return []Entry{entry(key, "type", typeString(tn.Type()))}
	}
	params := typeParams(named.TypeParams())
	var out []Entry
	switch u := named.Underlying().(type) {
	case *types.Struct:
		out = append(out, entry(key, "type", join(params, "struct")))
		for i := range u.NumFields() {
			f := u.Field(i)
			if !f.Exported() {
				continue
			}
			sig := typeString(f.Type())
			if f.Embedded() {
				// Embedding is promotion: the field's own methods arrive in
				// the outer type's method set, so changing an embedded field
				// to a plain one of the same type removes methods without
				// touching any other line.
				sig = "embedded " + sig
			}
			out = append(out, entry(key+"."+f.Name(), "field", sig))
		}
		out = append(out, methodEntries(key, named)...)
	case *types.Interface:
		out = append(out, entry(key, "type", join(params, "interface")))
		// NumMethods, not NumExplicitMethods: an embedded interface's methods
		// are part of what an implementer has to provide, so they are part of
		// the contract whichever side of an embed they were written on.
		for i := range u.NumMethods() {
			m := u.Method(i)
			if !m.Exported() {
				continue
			}
			out = append(out, entry(key+"."+m.Name(), "ifacemethod", funcSig(m.Signature(), nil)))
		}
	default:
		out = append(out, entry(key, "type", join(params, underlyingString(named.Underlying()))))
		out = append(out, methodEntries(key, named)...)
	}
	return out
}

// methodEntries renders the exported method set of named, once per method.
//
// The receiver is rendered as the method's first parameter — the method
// expression form, T.M — so that moving a method from a value receiver to a
// pointer receiver shows up. That move is a real break: it drops the method
// from the value's method set, and a value that used to satisfy an interface
// stops satisfying it. Nothing else in the line would change.
func methodEntries(key string, named *types.Named) []Entry {
	var out []Entry
	seen := map[string]bool{}
	// Value method set first, so a method that exists on both is recorded
	// against the value receiver — the stronger promise of the two.
	for _, recv := range []types.Type{named, types.NewPointer(named)} {
		ms := types.NewMethodSet(recv)
		for i := range ms.Len() {
			m, ok := ms.At(i).Obj().(*types.Func)
			if !ok || !m.Exported() || seen[m.Name()] {
				continue
			}
			seen[m.Name()] = true
			out = append(out, entry(key+"."+m.Name(), "method", funcSig(m.Signature(), recv)))
		}
	}
	return out
}

// qualifier names the package a type comes from.
func qualifier(p *types.Package) string {
	if p == nil {
		return ""
	}
	if s, ok := strings.CutPrefix(p.Path(), modulePrefix); ok {
		return s
	}
	return p.Name()
}

// typeString renders a type the way a caller would write it.
//
// It is a renderer of its own rather than a call to types.TypeString because
// of one difference that matters: a function type nested inside another type
// has to lose its parameter names too. go/types prints
//
//	func(func(lat float64, lon float64, accuracy float64, err error))
//
// which puts four names a caller cannot use into a line that is supposed to
// hold only the contract — so renaming a callback parameter would show up in
// the manifest as a -/+ pair on a stable key, the shape reserved for a break.
// The same recursion is what keeps a generic type's *reference* spelling,
// Animated[T], apart from its declaration, Animated[T comparable].
//
// Shapes with no API-relevant naming to strip — anonymous structs and
// interfaces, type-set constraints — fall through to go/types, which is both
// deterministic and better at them than a hand-rolled copy would be.
func typeString(t types.Type) string {
	switch t := t.(type) {
	case *types.Basic:
		return t.Name()
	case *types.Alias:
		// Kept as the alias, not its target: the alias name is what source
		// writes, and the target is recorded once on the alias's own line.
		return namedRef(t.Obj(), t.TypeArgs(), nil)
	case *types.Named:
		return namedRef(t.Obj(), t.TypeArgs(), t.TypeParams())
	case *types.TypeParam:
		return t.Obj().Name()
	case *types.Pointer:
		return "*" + typeString(t.Elem())
	case *types.Slice:
		return "[]" + typeString(t.Elem())
	case *types.Array:
		return "[" + strconv.FormatInt(t.Len(), 10) + "]" + typeString(t.Elem())
	case *types.Map:
		return "map[" + typeString(t.Key()) + "]" + typeString(t.Elem())
	case *types.Chan:
		switch t.Dir() {
		case types.SendOnly:
			return "chan<- " + typeString(t.Elem())
		case types.RecvOnly:
			return "<-chan " + typeString(t.Elem())
		default:
			return "chan " + typeString(t.Elem())
		}
	case *types.Signature:
		return funcSig(t, nil)
	}
	return types.TypeString(t, qualifier)
}

// namedRef renders a reference to a declared type: its qualified name, plus
// type arguments if it is instantiated, plus bare type parameter names if it
// is the uninstantiated generic. The constraints belong on the declaration
// line and nowhere else — a receiver reading *StateBase[W widget.Widget] turns
// every method of a generic type into a restatement of its constraint.
func namedRef(obj *types.TypeName, args *types.TypeList, params *types.TypeParamList) string {
	var b strings.Builder
	if q := qualifier(obj.Pkg()); q != "" {
		b.WriteString(q + ".")
	}
	b.WriteString(obj.Name())
	switch {
	case args != nil && args.Len() > 0:
		var parts []string
		for i := range args.Len() {
			parts = append(parts, typeString(args.At(i)))
		}
		b.WriteString("[" + strings.Join(parts, ", ") + "]")
	case params != nil && params.Len() > 0:
		var parts []string
		for i := range params.Len() {
			parts = append(parts, params.At(i).Obj().Name())
		}
		b.WriteString("[" + strings.Join(parts, ", ") + "]")
	}
	return b.String()
}

// underlyingString renders an underlying type, routing function types through
// funcSig so that a defined function type loses its parameter names the same
// way a declared function does.
func underlyingString(t types.Type) string {
	if sig, ok := t.(*types.Signature); ok {
		return funcSig(sig, nil)
	}
	return typeString(t)
}

// funcSig renders a signature with its parameter and result types but without
// their names.
//
// Names are left out because they are not part of the contract: Go has no
// keyword arguments, so renaming a parameter cannot break a caller. If they
// were recorded, a rename would land in the manifest as a -/+ pair on a stable
// key — indistinguishable from an actual type change, which is the one thing
// this file exists to make obvious.
//
// recv, when non-nil, is prepended as the first parameter.
func funcSig(sig *types.Signature, recv types.Type) string {
	var b strings.Builder
	b.WriteString("func")
	b.WriteString(typeParams(sig.TypeParams()))
	b.WriteByte('(')
	var parts []string
	if recv != nil {
		parts = append(parts, typeString(recv))
	}
	ps := sig.Params()
	for i := range ps.Len() {
		t := ps.At(i).Type()
		if sig.Variadic() && i == ps.Len()-1 {
			if sl, ok := t.(*types.Slice); ok {
				parts = append(parts, "..."+typeString(sl.Elem()))
				continue
			}
		}
		parts = append(parts, typeString(t))
	}
	b.WriteString(strings.Join(parts, ", "))
	b.WriteByte(')')

	rs := sig.Results()
	switch rs.Len() {
	case 0:
	case 1:
		b.WriteString(" " + typeString(rs.At(0).Type()))
	default:
		var out []string
		for i := range rs.Len() {
			out = append(out, typeString(rs.At(i).Type()))
		}
		b.WriteString(" (" + strings.Join(out, ", ") + ")")
	}
	return b.String()
}

// typeParams renders a declaration's type parameter list with its
// constraints. Empty for a non-generic declaration.
//
// Type parameter names are kept, unlike ordinary parameter names: a receiver
// renders as *StateBase[W], so dropping the name from the declaration would
// leave the manifest using a name it never introduced. The constraint is the
// part that is contract — it is what decides whether somebody's type can be
// passed — so narrowing one has to show.
// join puts a space between a type parameter list and what follows it, and
// nothing at all when the declaration is not generic.
func join(params, rest string) string {
	if params == "" {
		return rest
	}
	return params + " " + rest
}

func typeParams(tp *types.TypeParamList) string {
	if tp == nil || tp.Len() == 0 {
		return ""
	}
	var parts []string
	for i := range tp.Len() {
		p := tp.At(i)
		parts = append(parts, p.Obj().Name()+" "+typeString(p.Constraint()))
	}
	return "[" + strings.Join(parts, ", ") + "]"
}

// constValue renders a constant's value, which for a constant is most of the
// contract: theme.FontBold is a map key an app writes as a string literal, and
// an iota block's numbers reach disk and the wire wherever anything persists
// one.
func constValue(c *types.Const) string {
	v := c.Val()
	if v == nil {
		return "?"
	}
	switch v.Kind() {
	case constant.String:
		s := constant.StringVal(v)
		// Long strings are not what constants in an API look like, but an
		// embedded blob would make a line unreadable, so cap it. The length
		// stays in the rendering: truncating silently would let two different
		// blobs render identically.
		if len(s) > 72 {
			return fmt.Sprintf("%s…+%d", strconv.Quote(s[:72]), len(s)-72)
		}
		return strconv.Quote(s)
	case constant.Float:
		// ExactString would render 0.5 as 1/2. A rational is exact but it is
		// not what the source says, and this file is read by people. The
		// shortest float64 rendering round-trips, so it still separates any
		// two values a Go program can hold.
		if f, _ := constant.Float64Val(v); !math.IsInf(f, 0) {
			return strconv.FormatFloat(f, 'g', -1, 64)
		}
	}
	return v.ExactString()
}

// shortPath drops the module prefix so the manifest reads as package paths
// rather than repeating the module on every line.
func shortPath(p string) string {
	if s, ok := strings.CutPrefix(p, modulePrefix); ok {
		return s
	}
	if p == strings.TrimSuffix(modulePrefix, "/") {
		return "."
	}
	return p
}

// Packages lists the module-relative packages the entries cover, sorted.
func Packages(entries []Entry) []string {
	seen := map[string]bool{}
	for _, e := range entries {
		seen[e.Package()] = true
	}
	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

// Consumers reports which of the module's own packages are imported from dirs.
//
// This exists because of a specific mistake. The 2026-08 API-surface audit
// concluded "nothing imports intl" and was
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
				if !strings.HasPrefix(imp, modulePrefix) {
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
