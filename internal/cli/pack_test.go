package cli

import (
	"bytes"
	"flag"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The app the packaging tests package. See testdata/packapp/main.go for why it
// is not a real gophics app.
const packApp = "github.com/doug/gophics/internal/cli/testdata/packapp"

// testIcon writes a square PNG and returns its path. The content is a
// gradient rather than a flat fill so a resampled copy is distinguishable
// from a wrongly-cropped one.
func testIcon(t *testing.T, px int) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, px, px))
	for y := 0; y < px; y++ {
		for x := 0; x < px; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 255 / px), uint8(y * 255 / px), 0x80, 0xff})
		}
	}
	p := filepath.Join(t.TempDir(), "icon.png")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}

// runPack packages testdata/packapp with o's overrides and returns the packer,
// the artifacts and everything it printed.
func runPack(t *testing.T, o packOpts) (*packer, []string, string) {
	t.Helper()
	if o.pkg == "" {
		o.pkg = packApp
	}
	if o.version == "" {
		o.version = "1.2.3"
	}
	if o.build == "" {
		o.build = "7"
	}
	if o.category == "" {
		o.category = "Utility"
	}
	if o.name == "" {
		o.name = "Packapp"
	}
	if o.goarch == "" {
		o.goarch = runtime.GOARCH
	}
	if o.out == "" {
		o.out = t.TempDir()
	}
	var log bytes.Buffer
	p := &packer{o: o, out: &log}
	arts, err := p.run()
	if err != nil {
		t.Fatalf("package for %s: %v\n%s", o.goos, err, log.String())
	}
	for _, a := range arts {
		if _, err := os.Stat(a); err != nil {
			t.Errorf("reported artifact %s: %v", a, err)
		}
	}
	return p, arts, log.String()
}

// onlyGoOnPath replaces PATH with a directory holding nothing but a link to
// the go command, and returns the directory.
//
// This is the state of the machine of everyone who runs a packaging command
// for the first time: no codesign identity, no hdiutil on Linux, no
// appimagetool, no resource compiler, no installer compiler. Packaging has to
// work there, because "install five tools first" is how a packaging feature
// goes unused.
func onlyGoOnPath(t *testing.T) string {
	t.Helper()
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := os.Symlink(goBin, filepath.Join(dir, "go")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	return dir
}

// TestPackagesEveryDesktopOSWithNothingButTheGoToolchain is the headline
// guarantee: one Go toolchain, no credentials, no extra tools, and a
// distributable artifact for all three desktops — including the two that are
// not the host, which is what the zero-cgo invariant buys.
func TestPackagesEveryDesktopOSWithNothingButTheGoToolchain(t *testing.T) {
	onlyGoOnPath(t)
	icon := testIcon(t, 512)

	cases := []struct {
		goos string
		// want are paths that must exist under the output directory.
		want []string
		// hints are the substrings a skipped optional step has to mention,
		// because a silent skip is indistinguishable from a broken feature.
		hints []string
	}{
		{
			goos: "darwin",
			want: []string{
				"Packapp.app/Contents/Info.plist",
				"Packapp.app/Contents/MacOS/Packapp",
				"Packapp.app/Contents/Resources/Packapp.icns",
				"Packapp.entitlements",
			},
			hints: []string{"unsigned", "CODESIGN_ID", "hdiutil", "not notarised"},
		},
		{
			goos: "linux",
			want: []string{
				"Packapp-1.2.3-linux-" + runtime.GOARCH + "/usr/bin/packapp",
				"Packapp-1.2.3-linux-" + runtime.GOARCH + "/usr/share/applications/com.example.packapp.desktop",
				"Packapp-1.2.3-linux-" + runtime.GOARCH + "/install.sh",
				"Packapp-1.2.3-linux-" + runtime.GOARCH + ".tar.gz",
				"Packapp.AppDir/AppRun",
			},
			hints: []string{"appimagetool", "metainfo", "-license"},
		},
		{
			goos: "windows",
			want: []string{
				"Packapp/packapp.exe",
				"Packapp/packapp.exe.manifest",
				"Packapp/packapp.ico",
				"Packapp-1.2.3-windows-" + runtime.GOARCH + ".zip",
				"installer/resources.rc",
				"installer/Packapp.wxs",
				"installer/Packapp.nsi",
			},
			hints: []string{"go-winres", "windres", "makensis", "wix"},
		},
	}
	for _, c := range cases {
		t.Run(c.goos, func(t *testing.T) {
			out := t.TempDir()
			_, _, log := runPack(t, packOpts{
				goos: c.goos, out: out, icon: icon,
				dmg: true, appimage: true, // both requested; both must degrade
			})
			for _, w := range c.want {
				if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(w))); err != nil {
					t.Errorf("%s: %v", w, err)
				}
			}
			for _, h := range c.hints {
				if !strings.Contains(log, h) {
					t.Errorf("nothing in the output mentions %q, so the skipped step is "+
						"invisible to whoever ran it:\n%s", h, log)
				}
			}
		})
	}
}

// A packaging run must leave nothing behind in the app's own source
// directory. The Windows path writes a resource object there — that is where
// the go tool looks for one — and a leftover would attach this run's version
// number to every later build of the app.
func TestWindowsPackagingLeavesNoResourceObjectBehind(t *testing.T) {
	dir, err := packageDir(packApp)
	if err != nil {
		t.Fatal(err)
	}
	before, err := filepath.Glob(filepath.Join(dir, "*.syso"))
	if err != nil {
		t.Fatal(err)
	}
	runPack(t, packOpts{goos: "windows", icon: testIcon(t, 256)})
	after, err := filepath.Glob(filepath.Join(dir, "*.syso"))
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before) {
		t.Errorf("packaging left %v in the app's source directory", after)
	}
}

func TestResolveRejectsPlatformsItCannotPackage(t *testing.T) {
	for _, goos := range []string{"js", "ios", "android", "plan9"} {
		fs := flag.NewFlagSet("package", flag.ContinueOnError)
		var o packOpts
		addPackFlags(fs, &o)
		if err := fs.Parse([]string{"-os", goos}); err != nil {
			t.Fatal(err)
		}
		err := o.resolve(fs)
		if err == nil {
			t.Errorf("-os %s was accepted; desktop packaging covers darwin, linux and windows", goos)
			continue
		}
		// The message has to point somewhere, not just refuse: web and mobile
		// do have an answer, and it is a different command.
		if !strings.Contains(err.Error(), "darwin") {
			t.Errorf("-os %s: %v — the error should name the platforms that do work", goos, err)
		}
	}
}

func TestUniversalIsRejectedOffDarwin(t *testing.T) {
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	var o packOpts
	addPackFlags(fs, &o)
	if err := fs.Parse([]string{"-os", "linux", "-arch", "universal"}); err != nil {
		t.Fatal(err)
	}
	if err := o.resolve(fs); err == nil {
		t.Error("-os linux -arch universal was accepted; lipo makes Mach-O fat binaries only")
	}
}

// Credentials come from the environment as well as from flags, because that is
// where CI keeps them and where the shell scripts this replaces looked.
func TestSigningCredentialsComeFromTheEnvironment(t *testing.T) {
	t.Setenv("CODESIGN_ID", "Developer ID Application: Someone (TEAM1)")
	t.Setenv("NOTARY_PROFILE", "NOTARY")
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	var o packOpts
	addPackFlags(fs, &o)
	if err := fs.Parse(nil); err != nil {
		t.Fatal(err)
	}
	if err := o.resolve(fs); err != nil {
		t.Fatal(err)
	}
	if o.identity != "Developer ID Application: Someone (TEAM1)" || o.notary != "NOTARY" {
		t.Errorf("identity=%q notary=%q; both should have come from the environment", o.identity, o.notary)
	}
}

func TestFourPartVersion(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"1", "1.0.0.0"},
		{"1.2", "1.2.0.0"},
		{"1.2.3", "1.2.3.0"},
		{"1.2.3.4", "1.2.3.4"},
		// A prerelease or build suffix has to go: the field is four numbers,
		// and a resource compiler fails on anything else.
		{"2.0.0-rc1", "2.0.0.0"},
		{"2.0.0+build9", "2.0.0.0"},
		{"v3.1", "3.1.0.0"},
	} {
		if got := fourPart(c.in); got != c.want {
			t.Errorf("fourPart(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The installer GUIDs are derived rather than generated, because an UpgradeCode
// that changes between builds makes version 2 install beside version 1.
func TestInstallerGUIDsAreStableAndDistinct(t *testing.T) {
	a := uuidV5("gophics.wix.com.example.one", "upgrade")
	if a != uuidV5("gophics.wix.com.example.one", "upgrade") {
		t.Error("the same identifier produced two different UpgradeCodes")
	}
	if a == uuidV5("gophics.wix.com.example.two", "upgrade") {
		t.Error("two apps share an UpgradeCode; each would uninstall the other")
	}
	if a == uuidV5("gophics.wix.com.example.one", "component.exe") {
		t.Error("the UpgradeCode and a component GUID are the same value")
	}
	if len(a) != 36 || strings.Count(a, "-") != 4 || a[14] != '5' {
		t.Errorf("uuidV5 = %q, want a 36-character version-5 UUID", a)
	}
}

func TestPublisherFromID(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"com.example.tally", "Example"},
		{"dev.gophics.hn", "Gophics"},
		{"tally", "Tally"},
	} {
		if got := publisherFromID(c.in); got != c.want {
			t.Errorf("publisherFromID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// The default identifier is a placeholder, and shipping two apps that share one
// makes them overwrite each other's preferences and installs. It has to be
// said out loud.
func TestDefaultIdentifierIsReportedAsAPlaceholder(t *testing.T) {
	p, _, _ := runPack(t, packOpts{goos: runtime.GOOS, dmg: false})
	if !p.defaultedID {
		t.Error("no -id was given but defaultedID is false, so nothing warns about it")
	}
	if p.o.id != "com.example.packapp" {
		t.Errorf("default id = %q", p.o.id)
	}

	p2, _, _ := runPack(t, packOpts{goos: runtime.GOOS, id: "com.gophics.packapp", dmg: false})
	if p2.defaultedID {
		t.Error("an explicit -id was still reported as a default")
	}
}

// The packaged binary is a release build: no DWARF, no symbol table, no local
// paths from the machine that built it.
func TestPackagedBuildIsTrimmedAndStripped(t *testing.T) {
	p := &packer{o: packOpts{goos: "linux"}}
	if got := p.ldflags(); got != "-s -w" {
		t.Errorf("linux ldflags = %q", got)
	}
	// Without -H=windowsgui, double-clicking the .exe opens a console window
	// behind the app and closing it kills the app.
	p = &packer{o: packOpts{goos: "windows"}}
	if !strings.Contains(p.ldflags(), "-H=windowsgui") {
		t.Errorf("windows ldflags = %q, want the GUI subsystem", p.ldflags())
	}
	p = &packer{o: packOpts{goos: "windows", console: true}}
	if strings.Contains(p.ldflags(), "windowsgui") {
		t.Errorf("-console still asked for the GUI subsystem: %q", p.ldflags())
	}
}
