package cli

import (
	"bytes"
	"debug/macho"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// What makes a directory an application on macOS is its layout and its
// Info.plist, so that is what is checked — on the bundle this actually
// produced, read back with a plist parser, rather than on the strings the
// generator assembled.
func TestMacBundleLayoutAndInfoPlist(t *testing.T) {
	out := t.TempDir()
	p, arts, _ := runPack(t, packOpts{
		goos: "darwin", out: out, dmg: false,
		name: "Packapp", id: "com.gophics.packapp", version: "2.1.0", build: "42",
		icon: testIcon(t, 1024), category: "Graphics", copyright: "(c) 2026 Someone",
	})
	app := filepath.Join(out, "Packapp.app")
	if arts[0] != app {
		t.Errorf("first artifact is %s, want the bundle %s", arts[0], app)
	}

	// The executable: in Contents/MacOS, named by CFBundleExecutable, and a
	// Mach-O rather than whatever else might have been copied there.
	exe := filepath.Join(app, "Contents", "MacOS", "Packapp")
	fi, err := os.Stat(exe)
	if err != nil {
		t.Fatalf("Contents/MacOS/Packapp: %v", err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("the bundle executable is not executable (mode %v)", fi.Mode())
	}
	mf, err := macho.Open(exe)
	if err != nil {
		t.Fatalf("the bundle executable is not a Mach-O: %v", err)
	}
	defer mf.Close()
	if mf.Type != macho.TypeExec {
		t.Errorf("Mach-O type %v, want an executable", mf.Type)
	}

	raw, err := os.ReadFile(filepath.Join(app, "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := parsePlist(raw)
	if err != nil {
		t.Fatalf("Info.plist does not parse: %v", err)
	}
	for k, want := range map[string]any{
		"CFBundleExecutable":            "Packapp",
		"CFBundleIdentifier":            "com.gophics.packapp",
		"CFBundleName":                  "Packapp",
		"CFBundleDisplayName":           "Packapp",
		"CFBundlePackageType":           "APPL",
		"CFBundleShortVersionString":    "2.1.0",
		"CFBundleVersion":               "42",
		"CFBundleInfoDictionaryVersion": "6.0",
		"CFBundleIconFile":              "Packapp",
		"LSApplicationCategoryType":     "public.app-category.graphics-design",
		"LSMinimumSystemVersion":        macMinimumSystemVersion,
		"NSHumanReadableCopyright":      "(c) 2026 Someone",
		// A 1x backing store on a Retina display is a blurred window, and the
		// framework's logical/physical split assumes the real scale factor.
		"NSHighResolutionCapable": true,
	} {
		if got := info[k]; got != want {
			t.Errorf("Info.plist[%q] = %#v, want %#v", k, got, want)
		}
	}
	if got, ok := info["CFBundleSupportedPlatforms"].([]any); !ok || len(got) != 1 || got[0] != "MacOSX" {
		t.Errorf("CFBundleSupportedPlatforms = %#v", info["CFBundleSupportedPlatforms"])
	}
	// CFBundleExecutable has to name the file that is actually there, or Launch
	// Services reports a damaged application.
	if info["CFBundleExecutable"] != filepath.Base(exe) {
		t.Errorf("CFBundleExecutable = %v but the executable is %s", info["CFBundleExecutable"], filepath.Base(exe))
	}
	// Same for the icon: the key is a base name resolved inside Resources.
	icns := filepath.Join(app, "Contents", "Resources", info["CFBundleIconFile"].(string)+".icns")
	if _, err := os.Stat(icns); err != nil {
		t.Errorf("CFBundleIconFile names %s, which is not in Resources: %v", info["CFBundleIconFile"], err)
	}

	pkginfo, err := os.ReadFile(filepath.Join(app, "Contents", "PkgInfo"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pkginfo) != "APPL????" {
		t.Errorf("PkgInfo = %q, want APPL????", pkginfo)
	}

	// The entitlements sit beside the bundle, not inside it: they describe what
	// the app was signed with and are an input to codesign.
	if _, err := os.Stat(filepath.Join(app, "Contents", "Resources", p.o.name+".entitlements")); err == nil {
		t.Error("the entitlements were copied into the bundle, where they describe nothing")
	}
	ents, err := os.ReadFile(filepath.Join(out, "Packapp.entitlements"))
	if err != nil {
		t.Fatal(err)
	}
	entd, err := parsePlist(ents)
	if err != nil {
		t.Fatalf("the entitlements do not parse: %v", err)
	}
	if entd["com.apple.security.app-sandbox"] != true {
		t.Errorf("entitlements = %#v, want the sandbox key", entd)
	}
}

// An unsigned bundle is the normal result of a first packaging run and must
// not be an error — but it must say what the signature would buy, because the
// failure it prevents happens on someone else's Mac.
func TestUnsignedIsNotAFailure(t *testing.T) {
	out := t.TempDir()
	_, _, log := runPack(t, packOpts{goos: "darwin", out: out, dmg: false, icon: testIcon(t, 256)})
	if _, err := os.Stat(filepath.Join(out, "Packapp.app", "Contents", "_CodeSignature")); err == nil {
		t.Error("the bundle carries a _CodeSignature with no identity given")
	}
	for _, want := range []string{"unsigned", "Gatekeeper", "-identity", "find-identity"} {
		if !strings.Contains(log, want) {
			t.Errorf("the unsigned notice does not mention %q:\n%s", want, log)
		}
	}
}

// Notarisation has three preconditions and skips on each with its own
// explanation. None of them may fail the build, and none of them may be
// silent: this is the step nobody has done before the first time.
func TestNotarisationSkipsExplainThemselves(t *testing.T) {
	t.Setenv("APPLE_ID", "")
	t.Setenv("APPLE_APP_PASSWORD", "")
	t.Setenv("APPLE_TEAM_ID", "")

	var log bytes.Buffer
	p := &packer{o: packOpts{goos: "darwin"}, out: &log}
	if err := p.notarizeMac("/nonexistent/Packapp.app", ""); err != nil {
		t.Errorf("unsigned: %v, want a skip", err)
	}
	if !strings.Contains(log.String(), "unsigned") {
		t.Errorf("an unsigned bundle was not notarised but nothing said why:\n%s", log.String())
	}

	log.Reset()
	p = &packer{o: packOpts{goos: "darwin", identity: "Developer ID Application: X (T)"}, out: &log}
	if err := p.notarizeMac("/nonexistent/Packapp.app", ""); err != nil {
		t.Errorf("no credentials: %v, want a skip", err)
	}
	for _, want := range []string{"store-credentials", "notary-profile", "NOTARY_PROFILE"} {
		if !strings.Contains(log.String(), want) {
			t.Errorf("the no-credentials notice does not mention %q:\n%s", want, log.String())
		}
	}
}

// Packaging must not require a signing identity to exist, and must not try to
// use a tool that is not there — cross-packaging a .app from Linux cannot sign
// it, and that is a note, not a failure.
func TestSignIsSkippedWhenCodesignIsAbsent(t *testing.T) {
	onlyGoOnPath(t)
	var log bytes.Buffer
	p := &packer{o: packOpts{goos: "darwin", identity: "Developer ID Application: X (T)"}, out: &log}
	if err := p.signMacApp(t.TempDir(), ""); err != nil {
		t.Errorf("signMacApp with no codesign: %v, want a skip", err)
	}
	if !strings.Contains(log.String(), "codesign") {
		t.Errorf("nothing explained the skipped signature:\n%s", log.String())
	}
}

// A display name is whatever the developer typed, and it lands in XML. An
// unescaped ampersand produces a bundle that Launch Services refuses with a
// message about nothing in particular.
func TestDisplayNameSurvivesXMLEscaping(t *testing.T) {
	const name = `Tom & Jerry <Pro> "99"`
	out := t.TempDir()
	runPack(t, packOpts{goos: "darwin", out: out, dmg: false, name: name})
	raw, err := os.ReadFile(filepath.Join(out, name+".app", "Contents", "Info.plist"))
	if err != nil {
		t.Fatal(err)
	}
	info, err := parsePlist(raw)
	if err != nil {
		t.Fatalf("Info.plist with a punctuated name does not parse: %v", err)
	}
	if info["CFBundleDisplayName"] != name {
		t.Errorf("CFBundleDisplayName = %#v, want %q", info["CFBundleDisplayName"], name)
	}
}

func TestPlistRoundTrip(t *testing.T) {
	src := plistObj{
		{"S", "text"},
		{"T", true},
		{"F", false},
		{"N", 42},
		{"A", []string{"one", "two"}},
		{"D", plistObj{{"Inner", "yes"}}},
	}
	got, err := parsePlist(encodePlist(src))
	if err != nil {
		t.Fatal(err)
	}
	if got["S"] != "text" || got["T"] != true || got["F"] != false || got["N"] != 42 {
		t.Errorf("scalars round-tripped as %#v", got)
	}
	arr, ok := got["A"].([]any)
	if !ok || len(arr) != 2 || arr[1] != "two" {
		t.Errorf("array round-tripped as %#v", got["A"])
	}
	inner, ok := got["D"].(map[string]any)
	if !ok || inner["Inner"] != "yes" {
		t.Errorf("nested dict round-tripped as %#v", got["D"])
	}
}

// A duplicate key is worth refusing rather than tolerating: the readers
// disagree about which one wins, so the app behaves differently depending on
// which tool looked at it.
func TestParsePlistRejectsDuplicateKeys(t *testing.T) {
	_, err := parsePlist([]byte(`<?xml version="1.0"?>
<plist version="1.0"><dict>
<key>A</key><string>one</string>
<key>A</key><string>two</string>
</dict></plist>`))
	if err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Errorf("err = %v, want a duplicate-key error", err)
	}
}

// The .dmg is the artifact a Mac user is handed. hdiutil is the one tool with
// no in-process substitute, so this is a capability skip rather than a defect:
// off macOS there is nothing that can make one.
func TestDMGIsAReadableDiskImage(t *testing.T) {
	if runtime.GOOS != "darwin" || !have("hdiutil") {
		t.Skip("capability skip: hdiutil is macOS-only, and no other tool can write a UDZO image")
	}
	out := t.TempDir()
	_, arts, log := runPack(t, packOpts{goos: "darwin", out: out, dmg: true, name: "Packapp"})
	var dmg string
	for _, a := range arts {
		if strings.HasSuffix(a, ".dmg") {
			dmg = a
		}
	}
	if dmg == "" {
		t.Fatalf("no .dmg among %v\n%s", arts, log)
	}
	// imageinfo parses the image without mounting it, which a test should not
	// do to the machine running it.
	if info, err := exec.Command("hdiutil", "imageinfo", dmg).CombinedOutput(); err != nil {
		t.Errorf("hdiutil imageinfo %s: %v\n%s", dmg, err, info)
	} else if !strings.Contains(string(info), "UDZO") {
		t.Errorf("%s is not a compressed image:\n%s", filepath.Base(dmg), info)
	}
	// The staging directory is scaffolding and must not survive.
	if _, err := os.Stat(filepath.Join(out, ".dmg")); err == nil {
		t.Error("the .dmg staging directory was left behind")
	}
}
