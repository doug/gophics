package cli

import (
	"encoding/xml"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// The app name is a display name, and it lands in five syntaxes that each
// reserve different characters: plist and manifest XML, a YAML scalar in
// project.yml, and Go string literals. Nothing escaped it, so a name with
// `&`, `: ` or `"` produced a host project that did not parse — and create
// reported success. The name below carries every character one of those
// syntaxes rejects; each file is then parsed by a real parser, not eyeballed.
func TestCreateEscapesTheDisplayNamePerSyntax(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the scaffold directory is named after the app, and Windows filenames cannot contain : \" or <")
	}
	const name = `Tom & Jerry: "Pro" <beta>`
	root := t.TempDir()
	t.Chdir(root)
	if err := cmdCreate([]string{name, "-module", "example.com/tj", "-p", "ios,android"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	dir := filepath.Join(root, name)

	plist := filepath.Join(dir, "ios", "App", "Info.plist")
	if got := plistDisplayName(t, plist); got != name {
		t.Errorf("Info.plist CFBundleDisplayName = %q, want %q", got, name)
	}
	if _, err := exec.LookPath("plutil"); err == nil {
		if out, err := exec.Command("plutil", "-lint", plist).CombinedOutput(); err != nil {
			t.Errorf("plutil -lint: %v\n%s", err, out)
		}
	}

	manifest := filepath.Join(dir, "android", "app", "src", "main", "AndroidManifest.xml")
	if got := manifestLabel(t, manifest); got != name {
		t.Errorf("AndroidManifest.xml android:label = %q, want %q", got, name)
	}

	// project.yml is what xcodegen regenerates the plist from, so its scalar
	// matters more than the plist's. No YAML parser in the standard library:
	// the value is a double-quoted scalar whose escapes are Go's, so Unquote
	// is the check, and xcodegen itself when it is installed.
	yml := filepath.Join(dir, "ios", "project.yml")
	if got := yamlScalar(t, yml, "CFBundleDisplayName"); got != name {
		t.Errorf("project.yml CFBundleDisplayName = %q, want %q", got, name)
	}
	if _, err := exec.LookPath("xcodegen"); err == nil {
		cmd := exec.Command("xcodegen", "generate")
		cmd.Dir = filepath.Join(dir, "ios")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Errorf("xcodegen generate: %v\n%s", err, out)
		} else if got := plistDisplayName(t, plist); got != name {
			t.Errorf("after xcodegen, CFBundleDisplayName = %q, want %q", got, name)
		}
	}

	if got := goTitle(t, filepath.Join(dir, "ui", "app.go")); got != name {
		t.Errorf("ui/app.go Title = %q, want %q", got, name)
	}
}

func TestCreateRejectsControlCharacters(t *testing.T) {
	t.Chdir(t.TempDir())
	for _, name := range []string{"two\nlines", "tab\tbed", "", "\x7f"} {
		err := cmdCreate([]string{name, "-module", "example.com/x", "-p", "desktop"})
		if err == nil {
			t.Errorf("create %q succeeded; a name with a control character cannot be quoted into every file it lands in", name)
			continue
		}
		if entries, _ := os.ReadDir("."); len(entries) != 0 {
			t.Errorf("create %q wrote %d entries before rejecting the name", name, len(entries))
		}
	}
}

// plistDisplayName parses the plist as XML — which fails on a bare `&` or
// `<` the way plutil does — and returns the CFBundleDisplayName string.
func plistDisplayName(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	var text, key string
	var lastKey string
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		switch v := tok.(type) {
		case xml.StartElement:
			key = v.Name.Local
			text = ""
		case xml.CharData:
			text += string(v)
		case xml.EndElement:
			if key == "key" {
				lastKey = text
			} else if key == "string" && lastKey == "CFBundleDisplayName" {
				return text
			}
			key = ""
		}
	}
}

func manifestLabel(t *testing.T, path string) string {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	dec := xml.NewDecoder(f)
	for {
		tok, err := dec.Token()
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "application" {
			for _, a := range se.Attr {
				if a.Name.Local == "label" {
					return a.Value
				}
			}
			t.Fatalf("%s: <application> has no android:label", path)
		}
	}
}

func yamlScalar(t *testing.T, path, key string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(b), "\n") {
		k, v, ok := strings.Cut(strings.TrimSpace(line), ": ")
		if !ok || k != key {
			continue
		}
		s, err := strconv.Unquote(v)
		if err != nil {
			t.Fatalf("%s: %s is %s, not a double-quoted scalar: %v", path, key, v, err)
		}
		return s
	}
	t.Fatalf("%s: no %s", path, key)
	return ""
}

// goTitle parses the scaffolded ui package and returns Config's Title.
func goTitle(t *testing.T, path string) string {
	t.Helper()
	f, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("%s does not parse: %v", path, err)
	}
	var title string
	ast.Inspect(f, func(n ast.Node) bool {
		kv, ok := n.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		if id, ok := kv.Key.(*ast.Ident); ok && id.Name == "Title" {
			if lit, ok := kv.Value.(*ast.BasicLit); ok {
				title, _ = strconv.Unquote(lit.Value)
			}
		}
		return true
	})
	return title
}
