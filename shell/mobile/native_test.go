package mobile

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The reference hosts in native/ are not compiled by this repo — they are
// copied into an app's own iOS or Android project. That is exactly why they
// need checking here: nothing else ever will, and both defects these tests
// catch had been sitting in the Swift files unnoticed, in code that could not
// have compiled for anyone who tried it.

func nativeFiles(t *testing.T, ext string) map[string]string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join("native", "*"+ext))
	if err != nil || len(paths) == 0 {
		t.Fatalf("no %s files under native/: %v", ext, err)
	}
	out := map[string]string{}
	for _, p := range paths {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		out[p] = string(b)
	}
	return out
}

// TestSwiftHostsImportTheBoundModule catches a file that references the
// generated types without importing the module that defines them — which is
// how all of these shipped, because "cannot find type MobileBridge in scope"
// is invisible until someone builds an iOS app.
func TestSwiftHostsImportTheBoundModule(t *testing.T) {
	for p, src := range nativeFiles(t, ".swift") {
		if !strings.Contains(src, "MobileBridge") {
			continue
		}
		if !regexp.MustCompile(`(?m)^import Mobile$`).MatchString(src) {
			t.Errorf("%s uses MobileBridge but never imports Mobile", p)
		}
	}
}

// TestSwiftHostsConformToTheProtocolNotTheClass catches the other one.
//
// gomobile emits a protocol and a class with the same name for each Go
// interface. Swift keeps the class under that name and renames the protocol
// with a Protocol suffix, so conforming to the bare name is inheritance from a
// class and fails to compile — while reading exactly like correct code.
var swiftConformance = regexp.MustCompile(`:\s*Mobile(\w+Host)\b(?:Protocol)?`)

func TestSwiftHostsConformToTheProtocolNotTheClass(t *testing.T) {
	for p, src := range nativeFiles(t, ".swift") {
		for _, m := range swiftConformance.FindAllString(src, -1) {
			if !strings.HasSuffix(m, "Protocol") {
				t.Errorf("%s conforms to %q, the generated class; Swift names the protocol %sProtocol",
					p, strings.TrimSpace(m), strings.TrimSpace(m))
			}
		}
	}
}

// TestEveryHostRoleHasBothPlatforms catches a role implemented for one
// platform and forgotten on the other — which is how iOS went without a
// camera preview host while Android had one, with nothing to say so.
func TestEveryHostRoleHasBothPlatforms(t *testing.T) {
	roles := map[string]map[string]bool{}
	for _, ext := range []string{".swift", ".kt"} {
		for p := range nativeFiles(t, ext) {
			role := strings.TrimSuffix(filepath.Base(p), ext)
			if roles[role] == nil {
				roles[role] = map[string]bool{}
			}
			roles[role][ext] = true
		}
	}
	for role, have := range roles {
		if !have[".swift"] {
			t.Errorf("%s exists for Android but not iOS", role)
		}
		if !have[".kt"] {
			t.Errorf("%s exists for iOS but not Android", role)
		}
	}
}

// TestSwiftHostsTypecheck is the real check, and it is expensive: it binds the
// framework with gomobile and runs swiftc against it. Set GOPHICS_IOS_HOSTS=1
// to include it. The static tests above exist because this one cannot run in
// most places — it needs macOS, Xcode with an iOS SDK, and gomobile.
func TestSwiftHostsTypecheck(t *testing.T) {
	if os.Getenv("GOPHICS_IOS_HOSTS") == "" {
		t.Skip("set GOPHICS_IOS_HOSTS=1 to typecheck against a real gomobile bind")
	}
	for _, tool := range []string{"gomobile", "swiftc", "xcrun"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}
	dir := t.TempDir()
	fw := filepath.Join(dir, "Mobile.xcframework")
	bind := exec.Command("gomobile", "bind", "-target=ios", "-o", fw,
		"github.com/doug/gophics/shell/mobile")
	if out, err := bind.CombinedOutput(); err != nil {
		t.Fatalf("gomobile bind: %v\n%s", err, out)
	}
	sdk, err := exec.Command("xcrun", "--sdk", "iphoneos", "--show-sdk-path").Output()
	if err != nil {
		t.Fatalf("xcrun: %v", err)
	}
	for p := range nativeFiles(t, ".swift") {
		out, err := exec.Command("swiftc", "-typecheck",
			"-sdk", strings.TrimSpace(string(sdk)),
			"-target", "arm64-apple-ios15.0",
			"-F", filepath.Join(fw, "ios-arm64"), p).CombinedOutput()
		if err != nil {
			t.Errorf("%s does not typecheck:\n%s", p, out)
		}
	}
}

// TestExampleHostsMatchTheReference catches the copies drifting.
//
// The files under native/ are meant to be copied into an app's project, and
// mirror did exactly that — so GophicsMonitor.kt and GophicsPreview.kt now
// exist twice, identical but for the package line, with nothing keeping them
// that way. A fix applied to one would silently leave the other broken, and
// the broken one is the copy people are told to start from.
//
// Comparing them here is the cheapest thing that makes the duplication safe:
// the reference stays the source of truth, and a divergence is a failure with
// the diff in it rather than a surprise months later.
func TestExampleHostsMatchTheReference(t *testing.T) {
	refs := nativeFiles(t, ".kt")
	byName := map[string]string{}
	for p, src := range refs {
		byName[filepath.Base(p)] = src
	}

	copies, err := filepath.Glob(
		filepath.Join("..", "..", "examples", "*", "android", "app", "src",
			"main", "java", "dev", "gophics", "*", "Gophics*.kt"))
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, p := range copies {
		ref, ok := byName[filepath.Base(p)]
		if !ok {
			continue // an app's own host, not a copy of a reference
		}
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		checked++
		if stripPackage(string(b)) != stripPackage(ref) {
			t.Errorf("%s has diverged from shell/mobile/native/%s — "+
				"apply the change to the reference and re-copy, or the file "+
				"people are told to start from stays broken",
				p, filepath.Base(p))
		}
	}
	if checked == 0 {
		t.Fatal("no example copies found; this test is checking nothing")
	}
}

// stripPackage drops the one line a copy is expected to change.
func stripPackage(src string) string {
	var out []string
	for _, l := range strings.Split(src, "\n") {
		if strings.HasPrefix(l, "package ") {
			continue
		}
		out = append(out, l)
	}
	return strings.Join(out, "\n")
}
