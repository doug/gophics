package mobile

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every exported Bridge method has to survive gomobile, and the failure mode is
// silence.
//
// A native host calls these methods directly — the CLI binds this package
// alongside the app's — and gomobile exports only what it can represent. What
// it cannot represent it *drops without a word*: methods returning shell types
// simply do not appear in the generated Java, with no warning at build time and
// no error at run time. The first sign is somebody asking why they cannot call
// a method that is plainly here.
//
// So the rules are checked here, where a violation is a failing test naming the
// method, rather than in a gradle log naming nothing.
//
// The rules gomobile applies to an exported method:
//   - at most one result, unless the second is error
//   - parameters and results limited to bool, int/int8..64, float32/64, string,
//     []byte, and named types from the packages being bound
//
// A method that breaks them is not a bug in itself — it is a method a host
// cannot call, which has to be a decision rather than an accident. Renaming it
// to lowercase, or moving it off Bridge, is how you make that decision.
func TestExportedBridgeMethodsAreBindable(t *testing.T) {
	methods := exportedBridgeMethods(t)
	if len(methods) < 30 {
		t.Fatalf("found only %d exported Bridge methods; the scan is broken and "+
			"this test would pass without checking anything", len(methods))
	}

	for _, m := range methods {
		if goSideOnly[m.name] {
			continue
		}
		if n := len(m.results); n > 1 {
			if n > 2 || m.results[1] != "error" {
				t.Errorf("Bridge.%s returns %v — gomobile allows a second result "+
					"only when it is error, and refuses to bind the whole package "+
					"otherwise", m.name, m.results)
			}
		}
		for _, typ := range append(append([]string{}, m.params...), m.results...) {
			if !bindableType(typ) {
				t.Errorf("Bridge.%s uses %q, which gomobile cannot represent — it "+
					"will drop the method from the generated host API silently",
					m.name, typ)
			}
		}
	}
}

// Doc comments may not contain a slash-star sequence.
//
// gobind copies Go doc comments into Javadoc block comments, so prose like
// "Deliver*/Fail*" — natural shorthand, and what three comments here used to
// say — closes the comment early and spills English into Java. The Android
// build then fails with "error: class, interface, or enum expected" pointing at
// a sentence, which is a long way from the cause.
func TestDocCommentsDoNotCloseAJavadocBlock(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		for i, line := range strings.Split(string(src), "\n") {
			trimmed := strings.TrimSpace(line)
			if strings.HasPrefix(trimmed, "//") && strings.Contains(trimmed, "*/") {
				t.Errorf("%s:%d contains %q in a comment; gobind copies it into "+
					"Javadoc, where it ends the block and breaks the Android "+
					"build. Write it as \"Deliver… and Fail…\".", f, i+1, "*/")
			}
		}
	}
	if checked == 0 {
		t.Fatal("no source files scanned")
	}
}

// goSideOnly are exported Bridge methods that gomobile drops on purpose.
//
// Each returns a shell.<X> interface and exists to satisfy shell.<X>Window, the
// contract app.wireCapabilities type-asserts to publish a capability to the
// widget tree. They are Go-side API that happens to be exported because an
// interface requires the name — a host never calls them, and could not.
//
// The list is here rather than absent so that dropping them is a decision on
// the record. Adding a name to it should mean "a host cannot call this and
// should not"; if the answer is "a host should call this", the fix is the
// signature, not the list.
var goSideOnly = map[string]bool{
	"Speakers": true, "Battery": true, "Camera": true, "CameraPreview": true,
	"Gamepads": true, "Geolocation": true, "Haptic": true, "Lifecycle": true,
	"Links": true, "Microphone": true, "Socket": true,
}

type bridgeMethod struct {
	name    string
	params  []string
	results []string
}

func exportedBridgeMethods(t *testing.T) []bridgeMethod {
	t.Helper()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var out []bridgeMethod
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			continue
		}
		for _, d := range file.Decls {
			fn, ok := d.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || !fn.Name.IsExported() {
				continue
			}
			if !isBridgeReceiver(fn.Recv) {
				continue
			}
			out = append(out, bridgeMethod{
				name:    fn.Name.Name,
				params:  fieldTypes(fn.Type.Params),
				results: fieldTypes(fn.Type.Results),
			})
		}
	}
	return out
}

func isBridgeReceiver(recv *ast.FieldList) bool {
	if recv == nil || len(recv.List) != 1 {
		return false
	}
	return strings.TrimPrefix(typeString(recv.List[0].Type), "*") == "Bridge"
}

func fieldTypes(fl *ast.FieldList) []string {
	if fl == nil {
		return nil
	}
	var out []string
	for _, f := range fl.List {
		n := len(f.Names)
		if n == 0 {
			n = 1
		}
		for range n {
			out = append(out, typeString(f.Type))
		}
	}
	return out
}

func typeString(e ast.Expr) string {
	switch t := e.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + typeString(t.X)
	case *ast.SelectorExpr:
		return typeString(t.X) + "." + t.Sel.Name
	case *ast.ArrayType:
		return "[]" + typeString(t.Elt)
	case *ast.MapType:
		return fmt.Sprintf("map[%s]%s", typeString(t.Key), typeString(t.Value))
	case *ast.FuncType:
		return "func"
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.Ellipsis:
		return "..." + typeString(t.Elt)
	}
	return "?"
}

// bindableType reports whether gomobile can carry a type across the boundary.
//
// Named types declared in this package are allowed: gomobile binds them too,
// because the package is bound. Types from other packages are not, unless the
// caller also binds those — which the CLI does not.
func bindableType(typ string) bool {
	switch typ {
	case "bool", "string", "error",
		"int", "int8", "int16", "int32", "int64",
		"float32", "float64", "byte", "rune", "[]byte":
		return true
	}
	// A named type from this package (MonitorHost, PreviewHost, MediaHost…).
	base := strings.TrimPrefix(strings.TrimPrefix(typ, "..."), "*")
	if !strings.Contains(base, ".") && !strings.HasPrefix(base, "[]") &&
		!strings.HasPrefix(base, "map[") && base != "func" && base != "interface{}" {
		return true
	}
	return false
}
