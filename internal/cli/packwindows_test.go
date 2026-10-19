package cli

import (
	"archive/zip"
	"debug/pe"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The staged directory is what an installer packages and what a zip of it
// unpacks to, so the executable in it has to be a Windows GUI binary — built
// from whatever host is running these tests.
func TestWindowsStageHoldsAGUIExecutable(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out, icon: testIcon(t, 512),
		name: "Packapp", id: "com.gophics.packapp", publisher: "Gophics"})
	exe := filepath.Join(out, "Packapp", "packapp.exe")
	f, err := pe.Open(exe)
	if err != nil {
		t.Fatalf("%s is not a PE binary: %v", exe, err)
	}
	defer f.Close()
	// Subsystem 2 is IMAGE_SUBSYSTEM_WINDOWS_GUI. A console binary (3) opens a
	// black window behind the app when double-clicked, and closing that window
	// kills the app.
	var subsystem uint16
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		subsystem = oh.Subsystem
	case *pe.OptionalHeader32:
		subsystem = oh.Subsystem
	default:
		t.Fatalf("no optional header in %s", exe)
	}
	if subsystem != 2 {
		t.Errorf("PE subsystem = %d, want 2 (GUI)", subsystem)
	}
}

func TestConsoleFlagKeepsTheConsoleSubsystem(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out, console: true})
	f, err := pe.Open(filepath.Join(out, "Packapp", "packapp.exe"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var subsystem uint16
	switch oh := f.OptionalHeader.(type) {
	case *pe.OptionalHeader64:
		subsystem = oh.Subsystem
	case *pe.OptionalHeader32:
		subsystem = oh.Subsystem
	}
	if subsystem != 3 {
		t.Errorf("with -console the PE subsystem is %d, want 3 (console)", subsystem)
	}
}

// The zip is the artifact someone downloads, so everything the app needs has
// to be inside it, under one directory.
func TestWindowsZipIsSelfContained(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out, icon: testIcon(t, 256)})
	zr, err := zip.OpenReader(filepath.Join(out, "Packapp-1.2.3-windows-"+runtime.GOARCH+".zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	want := map[string]bool{
		"Packapp/packapp.exe":          false,
		"Packapp/packapp.exe.manifest": false,
		"Packapp/packapp.ico":          false,
	}
	for _, f := range zr.File {
		if strings.HasPrefix(f.Name, "/") {
			t.Errorf("member %q is absolute", f.Name)
		}
		if _, ok := want[f.Name]; ok {
			want[f.Name] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s is not in the zip", name)
		}
	}
}

// The side-by-side manifest is the version-metadata fallback that needs no
// compiler at all: without a DPI declaration Windows reports 96 DPI to the
// process and upscales the frame it draws, which blurs every window on a
// high-density display.
func TestWindowsManifestDeclaresDPIAwareness(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out})
	raw, err := os.ReadFile(filepath.Join(out, "Packapp", "packapp.exe.manifest"))
	if err != nil {
		t.Fatal(err)
	}
	// Walked as XML rather than grepped, because a manifest Windows cannot
	// parse is a manifest Windows ignores — silently.
	dec := xml.NewDecoder(strings.NewReader(string(raw)))
	var dpiAwareness, execLevel string
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "dpiAwareness":
			var v string
			if err := dec.DecodeElement(&v, &se); err != nil {
				t.Fatal(err)
			}
			dpiAwareness = strings.TrimSpace(v)
		case "requestedExecutionLevel":
			for _, a := range se.Attr {
				if a.Name.Local == "level" {
					execLevel = a.Value
				}
			}
		}
	}
	if dpiAwareness != "PerMonitorV2" {
		t.Errorf("dpiAwareness = %q, want PerMonitorV2", dpiAwareness)
	}
	// An app that asks for elevation on every launch is an app users stop
	// launching, and this one writes nothing outside the user's own data.
	if execLevel != "asInvoker" {
		t.Errorf("requestedExecutionLevel = %q, want asInvoker", execLevel)
	}
}

// The .rc is the resource source, and the one thing in Windows packaging that
// needs an external compiler. It is written whether or not one is installed,
// so a Windows machine or a CI job can finish the job.
func TestResourceScriptCarriesVersionInfo(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out, icon: testIcon(t, 256),
		version: "2.5.1", name: "Packapp", publisher: "Gophics", copyright: "(c) 2026"})
	raw, err := os.ReadFile(filepath.Join(out, "installer", "resources.rc"))
	if err != nil {
		t.Fatal(err)
	}
	rc := string(raw)
	for _, want := range []string{
		"1 VERSIONINFO",
		"FILEVERSION 2,5,1,0", // numeric, four fields, or the compiler refuses it
		"PRODUCTVERSION 2,5,1,0",
		`1 ICON "packapp.ico"`,
		`1 24 "packapp.exe.manifest"`, // RT_MANIFEST at the process's own id
		`VALUE "ProductName", "Packapp"`,
		`VALUE "CompanyName", "Gophics"`,
		`VALUE "LegalCopyright", "(c) 2026"`,
		`VALUE "OriginalFilename", "packapp.exe"`,
		`VALUE "Translation", 0x409, 1200`,
	} {
		if !strings.Contains(rc, want) {
			t.Errorf("resources.rc has no %q:\n%s", want, rc)
		}
	}
	// The files it references have to be beside it, or windres cannot compile
	// it from the directory it was told to.
	for _, f := range []string{"packapp.ico", "packapp.exe.manifest"} {
		if _, err := os.Stat(filepath.Join(out, "installer", f)); err != nil {
			t.Errorf("resources.rc references %s, which is not beside it: %v", f, err)
		}
	}
}

// Both installer sources are generated whether or not a compiler for either is
// installed: an MSI is what a deployment tool expects, an NSIS .exe is what an
// individual downloads, and neither compiler runs on a Mac by default.
func TestInstallerSourcesAreCompleteAndParseable(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "windows", out: out, icon: testIcon(t, 256),
		name: "Packapp", id: "com.gophics.packapp", publisher: "Gophics", version: "2.5.1"})

	wxs, err := os.ReadFile(filepath.Join(out, "installer", "Packapp.wxs"))
	if err != nil {
		t.Fatal(err)
	}
	// Well-formed XML at least; the schema needs wix itself.
	if err := xml.Unmarshal(wxs, new(struct {
		XMLName xml.Name
	})); err != nil {
		t.Errorf("the WiX source is not well-formed XML: %v", err)
	}
	for _, want := range []string{
		`Manufacturer="Gophics"`, // WiX requires one
		`Version="2.5.1.0"`,      // MSI versions are four numeric fields
		"<MajorUpgrade",          // or version 2 installs beside version 1
		`Source="..\Packapp\packapp.exe"`,
		"<Shortcut", // or the app is installed with no way to start it
	} {
		if !strings.Contains(string(wxs), want) {
			t.Errorf("the WiX source has no %q", want)
		}
	}

	nsi, err := os.ReadFile(filepath.Join(out, "installer", "Packapp.nsi"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		`OutFile "..\Packapp-2.5.1-windows-` + runtime.GOARCH + `-setup.exe"`,
		`VIProductVersion "2.5.1.0"`,
		`File "..\Packapp\packapp.exe"`,
		"CreateShortcut",
		"WriteUninstaller",
		`"UninstallString"`, // or it cannot be removed from Settings
		"Section \"Uninstall\"",
	} {
		if !strings.Contains(string(nsi), want) {
			t.Errorf("the NSIS source has no %q", want)
		}
	}
}

// A resource object the app already carries wins: two VERSIONINFO resources in
// one binary is a link error, and the app's own is the one it means to ship.
func TestExistingResourceObjectIsLeftAlone(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "rsrc_windows_amd64.syso"), []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	var log strings.Builder
	p := &packer{o: packOpts{goos: "windows", goarch: "amd64"}, dir: dir, out: &log}
	if got := p.embedWindowsResources(t.TempDir(), "resources.rc"); len(got) != 0 {
		t.Errorf("embedWindowsResources returned %v next to an existing .syso", got)
	}
	// And it must not have removed or replaced the app's own.
	if _, err := os.Stat(filepath.Join(dir, "rsrc_windows_amd64.syso")); err != nil {
		t.Errorf("the app's own resource object is gone: %v", err)
	}
	if !strings.Contains(log.String(), "rsrc_windows_amd64.syso") {
		t.Errorf("nothing said why the version resource was left out:\n%s", log.String())
	}
}

func TestWindresTargets(t *testing.T) {
	if got := windresTarget("amd64"); got != "pe-x86-64" {
		t.Errorf("windresTarget(amd64) = %q", got)
	}
	// binutils has no PE/COFF ARM64 target, and claiming one produces an
	// object the linker rejects with an error about the file format.
	if got := windresTarget("arm64"); got != "" {
		t.Errorf("windresTarget(arm64) = %q, want \"\" so the step is skipped", got)
	}
}

func TestWixArch(t *testing.T) {
	for in, want := range map[string]string{"amd64": "x64", "386": "x86", "arm64": "arm64"} {
		if got := wixArch(in); got != want {
			t.Errorf("wixArch(%q) = %q, want %q", in, got, want)
		}
	}
}
