package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The bind package name must come from something a checkout cannot change.
// It used to be the directory basename at build time and the typed name at
// create time, so a project created as lingo and cloned as gophics_lingo had
// a host importing lingomobile while the build produced gophicslingomobile.
func TestBindPkgNameFollowsTheImportPath(t *testing.T) {
	for path, want := range map[string]string{
		"github.com/doug/gophics/examples/hn":     "hnmobile",
		"github.com/doug/gophics/examples/mirror": "mirrormobile",
		"github.com/doug/gophics_lingo":           "gophicslingomobile",
		"example.com/My-App":                      "myappmobile",
		"example.com/app/v2":                      "appmobile",
		"scratchapp":                              "scratchappmobile",
		// create used to allow a leading digit here, which is not a Go
		// package name; the build never did.
		"example.com/2048": "appmobile",
	} {
		if got := bindPkgName(path); got != want {
			t.Errorf("bindPkgName(%q) = %q, want %q", path, got, want)
		}
	}
}

// A freshly created app's hosts must import the package the build will bind,
// even when the app is named differently from its module — the case that
// produced the mismatch.
func TestCreatedHostsImportWhatTheBuildBinds(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)
	const module = "github.com/someone/gophics_lingo"
	if err := cmdCreate([]string{"lingo", "-module", module, "-p", "ios,android"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	pkg := bindPkgName(module)
	if err := checkHostBindImport(filepath.Join(root, "lingo", "android"), pkg, false); err != nil {
		t.Errorf("android host: %v", err)
	}
	if err := checkHostBindImport(filepath.Join(root, "lingo", "ios"), titleFirst(pkg), true); err != nil {
		t.Errorf("ios host: %v", err)
	}
	// And the check is not vacuous: the host really does import a bind package.
	kt := findFile(t, filepath.Join(root, "lingo", "android"), "MainActivity.kt")
	if !strings.Contains(kt, "import "+pkg+"."+titleFirst(pkg)) {
		t.Errorf("MainActivity.kt does not import %s", pkg)
	}
}

// A host that imports another name fails before the bind, naming both sides
// and the line to write — not minutes later inside the Kotlin compiler.
func TestHostImportMismatchExplainsItself(t *testing.T) {
	host := t.TempDir()
	src := filepath.Join(host, "app", "src", "main", "java", "com", "example", "lingo")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "package com.example.lingo\n\nimport lingomobile.Lingomobile\nimport mobile.Bridge\n"
	if err := os.WriteFile(filepath.Join(src, "MainActivity.kt"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	err := checkHostBindImport(host, "gophicslingomobile", false)
	if err == nil {
		t.Fatal("a host importing lingomobile passed a build of gophicslingomobile")
	}
	for _, want := range []string{"lingomobile", "gophicslingomobile", "import gophicslingomobile.Gophicslingomobile", "MainActivity.kt"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q:\n%v", want, err)
		}
	}
	if err := checkHostBindImport(host, "lingomobile", false); err != nil {
		t.Errorf("a matching host was rejected: %v", err)
	}
}

// The checked-in example hosts must pass the check against the names the
// build gives them, or the check would break apps that work.
func TestExampleHostsMatchTheirBindPackage(t *testing.T) {
	for _, app := range []string{"mirror", "hn", "health"} {
		opts := buildOpts{pkg: "github.com/doug/gophics/examples/" + app}
		// bindName, not resolveBindPkg: the name is all this needs, and
		// resolving mirror's would write a bind package into examples/ for the
		// length of the run.
		name, err := bindName(opts)
		if err != nil {
			t.Fatalf("%s: %v", app, err)
		}
		dir, err := packageDir(opts.pkg)
		if err != nil {
			t.Fatal(err)
		}
		if err := checkHostBindImport(filepath.Join(dir, "android"), name, false); err != nil {
			t.Errorf("%s android: %v", app, err)
		}
		if err := checkHostBindImport(filepath.Join(dir, "ios"), titleFirst(name), true); err != nil {
			t.Errorf("%s ios: %v", app, err)
		}
	}
}

func findFile(t *testing.T, root, name string) string {
	t.Helper()
	var found string
	filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() && d.Name() == name {
			found = p
		}
		return nil
	})
	if found == "" {
		t.Fatalf("no %s under %s", name, root)
	}
	b, err := os.ReadFile(found)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
