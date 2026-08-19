//go:build !nogpu

package gpu

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"testing"

	"github.com/doug/gophics/internal/gfx/wgpu"
)

// TestDestroyBuffersReleasesEveryBuffer checks that DestroyBuffers mentions
// every buffer field on VelloComputeBuffers.
//
// Adding the clip stage added a ClipInps buffer and did not add it here, so
// every frame leaked one until a GC finaliser reclaimed it. That was visible
// only as "wgpu: Buffer released by GC (missing explicit Release)" in a log
// that is off by default, and only on a device run.
//
// This reads the source rather than calling the function, which is unusual and
// deliberate: destroyBuf takes the buffer by value and never clears the field,
// so a runtime check has nothing to observe. The first version of this test
// asserted the fields were nil afterwards, passed, and kept passing with the
// fix removed — a test that could not fail. Comparing the struct's fields
// against the identifiers the function actually references is the check that
// bites.
func TestDestroyBuffersReleasesEveryBuffer(t *testing.T) {
	// Source-reading, so it only runs where the source is. A cross-compiled
	// test binary pushed to a device has the code but not the file.
	if _, err := os.Stat("vello_compute.go"); err != nil {
		t.Skip("source not present (cross-compiled run); this check runs on the build host")
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "vello_compute.go", nil, 0)
	if err != nil {
		t.Fatalf("parse vello_compute.go: %v", err)
	}

	released := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "DestroyBuffers" {
			return true
		}
		ast.Inspect(fn.Body, func(m ast.Node) bool {
			if sel, ok := m.(*ast.SelectorExpr); ok {
				if id, ok := sel.X.(*ast.Ident); ok && id.Name == "bufs" {
					released[sel.Sel.Name] = true
				}
			}
			return true
		})
		return false
	})
	if len(released) == 0 {
		t.Fatal("found no bufs.* references in DestroyBuffers — this test is not checking anything")
	}

	bufType := reflect.TypeOf(VelloComputeBuffers{})
	bufPtr := reflect.TypeOf((*wgpu.Buffer)(nil))
	checked := 0
	for i := 0; i < bufType.NumField(); i++ {
		f := bufType.Field(i)
		if f.Type != bufPtr {
			continue
		}
		checked++
		if !released[f.Name] {
			t.Errorf("DestroyBuffers never mentions %s — every frame leaks one until the GC reclaims it", f.Name)
		}
	}
	if checked == 0 {
		t.Fatal("no *wgpu.Buffer fields found — this test is not checking anything")
	}
}
