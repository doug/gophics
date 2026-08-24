package gpu

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGlyphAtlasFrameClockIsTicked guards the failure that hid a rendering bug
// for the life of this package: GlyphMaskAtlas.AdvanceFrame had no callers.
//
// The atlas stamps each glyph with the frame that used it, refuses to evict
// anything the current frame references, and reclaims pages nothing has drawn
// from in a while. All three read a counter only AdvanceFrame moves. With no
// caller the counter stayed at zero, every entry looked equally old, and none
// of that machinery did anything — which no test noticed, because a function
// that is never called breaks nothing that is tested.
//
// So this asserts the wiring rather than the behaviour: something in this
// package, outside its tests, must advance the clock.
func TestGlyphAtlasFrameClockIsTicked(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	found := ""
	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			continue // a file for another platform; the build tag decides
		}
		// Skip the forwarding method's own body. Counting that would make
		// this test pass on a package where the only mention of AdvanceFrame
		// is a wrapper nobody calls — which is the exact shape of the bug,
		// and is what my first version of this test did.
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name == "AdvanceFrame" {
				continue
			}
			ast.Inspect(fn, func(n ast.Node) bool {
				sel, ok := n.(*ast.SelectorExpr)
				if !ok || sel.Sel == nil || sel.Sel.Name != "AdvanceFrame" {
					return true
				}
				found = name
				return false
			})
			if found != "" {
				break
			}
		}
		if found != "" {
			break
		}
	}
	if found == "" {
		t.Error("nothing in this package calls AdvanceFrame, so the glyph atlas's " +
			"frame counter never moves — eviction cannot tell a glyph on screen " +
			"from one abandoned long ago, and stale pages are never reclaimed")
	}
}
