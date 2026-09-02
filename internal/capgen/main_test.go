package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// capsOf is the convention the whole capability system hangs on: it decides
// which interfaces in shell/*.go are <X>Window opt-ins. Nothing else checks
// its edge cases — the gates catch output drift, but a parser that silently
// matched the wrong interface (or dropped the right one) would generate wiring
// for it before anyone noticed. These pin the convention as documented in the
// README: methods must be zero-argument, single-result getters returning a
// same-package named type.
func parseIface(t *testing.T, src string) map[string]*ast.InterfaceType {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), "x.go", "package shell\n"+src, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]*ast.InterfaceType{}
	ast.Inspect(f, func(n ast.Node) bool {
		if ts, ok := n.(*ast.TypeSpec); ok {
			if it, ok := ts.Type.(*ast.InterfaceType); ok {
				out[ts.Name.Name] = it
			}
		}
		return true
	})
	return out
}

func TestCapsOfAcceptsAGetterWindow(t *testing.T) {
	ifs := parseIface(t, `
type Widget interface{ Frob() }
type FrobWindow interface{ Frobber() Frobber }
type Frobber interface{ Frob() }
`)
	w, ok := capsOf("FrobWindow", ifs["FrobWindow"])
	if !ok {
		t.Fatal("a well-formed <X>Window was rejected")
	}
	if len(w.Caps) != 1 || w.Caps[0].Field != "Frobber" || w.Caps[0].Type != "Frobber" {
		t.Errorf("got %+v", w.Caps)
	}
}

func TestCapsOfAcceptsMultipleGetters(t *testing.T) {
	ifs := parseIface(t, `
type MediaWindow interface {
	Camera() Camera
	Audio() Audio
}
type Camera interface{ X() }
type Audio interface{ Y() }
`)
	w, ok := capsOf("MediaWindow", ifs["MediaWindow"])
	if !ok || len(w.Caps) != 2 {
		t.Fatalf("multi-getter window: ok=%v caps=%+v", ok, w.Caps)
	}
	// Sorted, so generation order is deterministic across runs.
	if w.Caps[0].Field != "Audio" || w.Caps[1].Field != "Camera" {
		t.Errorf("caps not sorted: %+v", w.Caps)
	}
}

// The rejections are the convention. Each of these shapes exists in real code
// (the base Window has methods with params; handlers embed interfaces) and
// generating wiring for one would be a silent wrong turn.
func TestCapsOfRejectsOffConventionShapes(t *testing.T) {
	cases := map[string]string{
		"a method with params":      `type BadWindow interface{ Thing(x int) Thing }`,
		"two results":               `type BadWindow interface{ Thing() (Thing, error) }`,
		"no results":                `type BadWindow interface{ Thing() }`,
		"an embedded interface":     `type BadWindow interface{ error }`,
		"a pointer result":          `type BadWindow interface{ Thing() *Thing }`,
		"a qualified result":        `type BadWindow interface{ Thing() geom.Size }`,
		"an empty interface":        `type BadWindow interface{}`,
		"one bad method among good": `type BadWindow interface{ Good() Good; Bad(x int) Good }`,
	}
	for name, src := range cases {
		ifs := parseIface(t, src+"\ntype Thing interface{}\ntype Good interface{}\n")
		if _, ok := capsOf("BadWindow", ifs["BadWindow"]); ok {
			t.Errorf("%s was accepted as a capability window", name)
		}
	}
}
