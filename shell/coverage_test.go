package shell_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every capability must be implemented on mobile, or say why not.
//
// This gate exists because the alternative already happened: seven capabilities
// — Share, Notifier, SecureStorage, Permissions, FilePicker, Preferences and
// WebView — had no method on the mobile Bridge at all, while the hand-written
// status table called them "stub", which means "interface plus a compile-checked
// TODO". Nothing compared the claim to the code, so the gap survived long enough
// to be documented as something else.
//
// A capability reaching a platform is a decision either way. Implemented, or
// listed below with the reason. What is not allowed is neither.
//
// "Implemented" here means the accessor exists, which is weaker than working:
// Gamepads publishes a Poll that safely returns nothing because there is no
// hardware to develop against. The generated capability matrix calls that
// "hollow" and is the place to look for it; this gate is only about whether the
// platform was considered at all.
var mobileExempt = map[string]string{
	"Menus": "a desktop menu bar; mobile apps draw their own menus in the widget tree",
	"Tray":  "a desktop system tray / menu bar extra; no mobile equivalent exists",
	"WindowControl": "title, fullscreen and resize belong to a windowing system; " +
		"the mobile analogues (orientation lock, status-bar style) are a different " +
		"capability, not this one",
	"Permissions": "mobile asks per capability instead — Camera.Authorize, " +
		"Microphone.Authorize, Notifier.Authorize, Photos.Authorize — which is " +
		"what both platforms actually model; a generic permission API would have " +
		"to invent a capability enum that maps onto neither",
	"WebView": "deliberately unbuilt: a native subview composited over the GPU " +
		"layer is the most expensive item here and the least useful, because " +
		"Apple and Google both steer OAuth to ASWebAuthenticationSession and " +
		"Custom Tabs, which OpenURL already reaches",
}

// capabilityNames reads the <X>Window interfaces the same way capgen does.
func capabilityNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var caps []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			name := ts.Name.Name
			if name == "Window" || !strings.HasSuffix(name, "Window") {
				return true
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, m := range it.Methods.List {
				fn, ok := m.Type.(*ast.FuncType)
				if !ok || len(m.Names) != 1 {
					continue
				}
				if fn.Params.NumFields() != 0 || fn.Results.NumFields() != 1 {
					continue
				}
				caps = append(caps, m.Names[0].Name)
			}
			return true
		})
	}
	sort.Strings(caps)
	return caps
}

var mobileAccessor = regexp.MustCompile(`(?m)^func \(b \*Bridge\) (\w+)\(\) shell\.\w+`)

func TestEveryCapabilityReachesMobileOrSaysWhyNot(t *testing.T) {
	caps := capabilityNames(t)
	if len(caps) < 20 {
		t.Fatalf("derived only %d capabilities; the scan is broken and this test "+
			"would pass without checking anything", len(caps))
	}

	// What shell/mobile actually publishes.
	have := map[string]bool{}
	paths, err := filepath.Glob(filepath.Join("mobile", "*.go"))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no Go files under mobile/: %v", err)
	}
	for _, p := range paths {
		if strings.HasSuffix(p, "_test.go") {
			continue
		}
		src, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range mobileAccessor.FindAllStringSubmatch(string(src), -1) {
			have[m[1]] = true
		}
	}
	if len(have) < 10 {
		t.Fatalf("found only %d mobile capability accessors; the scan is broken", len(have))
	}

	for _, c := range caps {
		_, exempt := mobileExempt[c]
		switch {
		case have[c] && exempt:
			t.Errorf("capability %q is implemented on mobile but still listed as "+
				"exempt — remove it from mobileExempt", c)
		case !have[c] && !exempt:
			t.Errorf("capability %q has no mobile implementation and no entry in "+
				"mobileExempt.\n"+
				"Implement `func (b *Bridge) %s() shell.%s` in shell/mobile, or add "+
				"an entry saying why the platform does not get it — so \"not on "+
				"mobile\" is a decision on the record rather than something nobody "+
				"noticed.", c, c, c)
		}
	}

	// And nothing lingers in the exempt list for a capability that no longer
	// exists, which would quietly excuse a name that means nothing.
	known := map[string]bool{}
	for _, c := range caps {
		known[c] = true
	}
	for name := range mobileExempt {
		if !known[name] {
			t.Errorf("mobileExempt lists %q, which is not a capability", name)
		}
	}
}
