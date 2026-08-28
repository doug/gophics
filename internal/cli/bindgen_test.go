package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The generated bind package has to compile, and the only way to know that is
// to compile it. A template that produces plausible-looking Go is worth nothing
// — the failure would surface as a gomobile error inside a file the user never
// wrote and cannot find.
func TestGeneratedBindPackageCompiles(t *testing.T) {
	pkg := "github.com/doug/gophics/examples/mirror"
	got, err := resolveBindPkg(buildOpts{pkg: pkg})
	if err != nil {
		t.Fatalf("resolveBindPkg(%s): %v", pkg, err)
	}
	want := pkg + "/build/bind"
	if got != want {
		t.Fatalf("bind package = %q, want %q", got, want)
	}
	t.Cleanup(func() {
		dir, err := packageDir(pkg)
		if err == nil {
			os.RemoveAll(filepath.Join(dir, "build"))
		}
	})

	out, err := exec.Command("go", "build", got).CombinedOutput()
	if err != nil {
		t.Fatalf("generated bind package does not compile: %v\n%s", err, out)
	}
}

// An app that carries its own bind package is left alone. health injects a
// HealthKit provider through Start(storeName) and news presents a native login
// sheet — neither is expressible by the convention, and generating over them
// would be worse than not generating at all.
func TestHandWrittenBindPackageWins(t *testing.T) {
	for _, app := range []string{"health", "news"} {
		pkg := "github.com/doug/gophics/examples/" + app
		got, err := resolveBindPkg(buildOpts{pkg: pkg})
		if err != nil {
			t.Errorf("%s: %v", app, err)
			continue
		}
		if want := pkg + "/mobile"; got != want {
			t.Errorf("%s: bind package = %q, want the hand-written %q", app, got, want)
		}
		dir, err := packageDir(pkg)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(filepath.Join(dir, "build", "bind")); err == nil {
			t.Errorf("%s: generated a bind package over a hand-written one", app)
		}
	}
}

// A bind package named directly on the command line is used as given, which is
// what keeps `gophics run -p ios ./examples/news/mobile` working.
func TestNamedBindPackageIsUsedAsGiven(t *testing.T) {
	pkg := "github.com/doug/gophics/examples/news/mobile"
	got, err := resolveBindPkg(buildOpts{pkg: pkg})
	if err != nil {
		t.Fatal(err)
	}
	if got != pkg {
		t.Errorf("bind package = %q, want %q unchanged", got, pkg)
	}
}

// The convention is load-bearing, so failing it has to say so in words that
// name the fix. The old failure mode was an undefined symbol reported from
// inside generated code.
func TestMissingUIPackageExplainsItself(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "go.mod"),
		[]byte("module scratchapp\n\ngo 1.24\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })

	_, err = resolveBindPkg(buildOpts{pkg: "."})
	if err == nil {
		t.Fatal("an app with no ui package must not silently produce a bind package")
	}
	for _, want := range []string{"ui/", "Root()", "Config()"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error does not mention %q, so it does not name the fix:\n%s", want, err)
		}
	}
}

// hostDirFor has to answer differently for an app root and a bind package,
// because the host project sits inside the first and beside the second. The
// wrong answer is not an error but a directory one level too high, which
// reports a missing host project for one that is right there.
func TestHostDirForAppRootAndBindPackage(t *testing.T) {
	root, err := packageDir("github.com/doug/gophics/examples/mirror")
	if err != nil {
		t.Fatal(err)
	}
	news, err := packageDir("github.com/doug/gophics/examples/news")
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		pkg  string
		want string
	}{
		{"github.com/doug/gophics/examples/mirror", filepath.Join(root, "ios")},
		{"github.com/doug/gophics/examples/news/mobile", filepath.Join(news, "ios")},
	}
	for _, c := range cases {
		got, err := hostDirFor(buildOpts{pkg: c.pkg, platform: platform{name: "ios"}})
		if err != nil {
			t.Errorf("%s: %v", c.pkg, err)
			continue
		}
		if got != c.want {
			t.Errorf("hostDirFor(%s) = %s, want %s", c.pkg, got, c.want)
		}
	}
}

// The framework name has to agree with the bind package's name, because the
// host imports it by that name and links <Name>.xcframework. Deriving it from
// the command-line package answers "Main" for an app root — a framework nothing
// builds and the host cannot find.
func TestFrameworkNameFollowsTheBindPackage(t *testing.T) {
	pkg := "github.com/doug/gophics/examples/mirror"
	t.Cleanup(func() {
		if dir, err := packageDir(pkg); err == nil {
			os.RemoveAll(filepath.Join(dir, "build"))
		}
	})
	got, err := frameworkName(buildOpts{pkg: pkg})
	if err != nil {
		t.Fatal(err)
	}
	// examples/mirror/ios/project.yml links Mirrormobile.xcframework and the
	// host does `import Mirrormobile`.
	if got != "Mirrormobile" {
		t.Errorf("frameworkName = %q, want %q — the checked-in iOS host imports that name", got, "Mirrormobile")
	}
}

// An app that carries no ios/ directory still has to be runnable, or "one
// main.go and a ui package" is not actually a whole app.
func TestHostIsGeneratedWhenAbsent(t *testing.T) {
	// The app name — and so the framework name the host imports — comes from the
	// directory, so the temp dir needs a real name rather than TempDir's numeric
	// leaf.
	dir := filepath.Join(t.TempDir(), "scratchapp")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{
		"go.mod":    "module scratchapp\n\ngo 1.24\n",
		"main.go":   "package main\n\nfunc main() {}\n",
		"ui/app.go": "package ui\n",
	} {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(cwd) })

	host, err := ensureHost(buildOpts{pkg: ".", platform: platform{name: "ios"}})
	if err != nil {
		t.Fatalf("ensureHost: %v", err)
	}
	if !dirExists(host) {
		t.Fatalf("ensureHost returned %s, which does not exist", host)
	}
	// The rendered host must actually reference the framework the bind will
	// produce, or it links against a name nothing builds.
	swift, err := os.ReadFile(filepath.Join(host, "App", "App.swift"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"import Scratchappmobile", "ScratchappmobileStart"} {
		if !strings.Contains(string(swift), want) {
			t.Errorf("generated host does not contain %q", want)
		}
	}
	if strings.Contains(string(swift), "{{") {
		t.Error("generated host still contains an unrendered template action")
	}
}

// adb refuses to install when more than one device is attached, and says so in
// an error that names neither. A tablet plugged in beside a running emulator is
// the normal state of anyone debugging on hardware, so the choice is made here
// rather than handed to the user as a failure.
func TestADBTargetPicksADevice(t *testing.T) {
	cases := []struct {
		name    string
		devices []string
		want    string
		wantErr bool
	}{
		{"none", nil, "", false},
		{"one emulator", []string{"emulator-5554"}, "emulator-5554", false},
		{"one device", []string{"R5GL42PGGYR"}, "R5GL42PGGYR", false},
		{
			// The interesting case: someone just plugged a tablet in beside the
			// emulator they had running, and means to use the tablet.
			"device beats emulator",
			[]string{"emulator-5554", "R5GL42PGGYR"},
			"R5GL42PGGYR", false,
		},
		{"two emulators", []string{"emulator-5554", "emulator-5556"}, "emulator-5554", false},
		{"two devices is ambiguous", []string{"R5GL42PGGYR", "R5GL42PGGYS"}, "", true},
	}
	for _, c := range cases {
		got, err := pickADBSerial(c.devices, "")
		if c.wantErr {
			if err == nil {
				t.Errorf("%s: expected an error naming the devices", c.name)
			}
			continue
		}
		if err != nil {
			t.Errorf("%s: %v", c.name, err)
			continue
		}
		if got != c.want {
			t.Errorf("%s: picked %q, want %q", c.name, got, c.want)
		}
	}

	// An explicit serial wins, and a wrong one is an error that lists what is
	// actually attached rather than silently using something else.
	got, err := pickADBSerial([]string{"emulator-5554", "R5GL42PGGYR"}, "emulator-5554")
	if err != nil || got != "emulator-5554" {
		t.Errorf("explicit serial = (%q, %v), want emulator-5554", got, err)
	}
	if _, err := pickADBSerial([]string{"emulator-5554"}, "nope"); err == nil {
		t.Error("an unknown serial must be an error")
	}
}
