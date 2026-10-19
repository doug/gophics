package cli

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"debug/elf"
	"fmt"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// desktopFile is a parsed desktop entry: the groups it declares and the keys
// of the first one.
type desktopFile struct {
	groups []string
	keys   map[string]string
	order  []string
}

// parseDesktopFile reads a desktop entry the way the specification describes
// it, so the test asserts on the file as a desktop environment would read it
// rather than on the string the generator built.
func parseDesktopFile(t *testing.T, p string) desktopFile {
	t.Helper()
	f, err := os.Open(p)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	out := desktopFile{keys: map[string]string{}}
	group := ""
	sc := bufio.NewScanner(f)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		switch {
		case text == "" || strings.HasPrefix(text, "#"):
		case strings.HasPrefix(text, "["):
			if !strings.HasSuffix(text, "]") {
				t.Fatalf("%s:%d: malformed group header %q", p, line, text)
			}
			group = text[1 : len(text)-1]
			out.groups = append(out.groups, group)
		default:
			k, v, ok := strings.Cut(text, "=")
			if !ok {
				t.Fatalf("%s:%d: %q is neither a group nor a key=value", p, line, text)
			}
			if group != "Desktop Entry" {
				continue
			}
			if _, dup := out.keys[k]; dup {
				t.Errorf("%s:%d: duplicate key %q", p, line, k)
			}
			out.keys[k] = v
			out.order = append(out.order, k)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestDesktopEntryMeetsTheSpecification(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{
		goos: "linux", out: out, icon: testIcon(t, 512),
		name: "Packapp", id: "com.gophics.packapp", comment: "A scratch app",
		category: "Graphics", license: "MIT",
	})
	entry := filepath.Join(out, "Packapp-1.2.3-linux-"+runtime.GOARCH,
		"usr", "share", "applications", "com.gophics.packapp.desktop")
	d := parseDesktopFile(t, entry)

	// The spec requires the group header to come first and to be exactly this.
	if len(d.groups) == 0 || d.groups[0] != "Desktop Entry" {
		t.Fatalf("groups = %v, want [Desktop Entry] first", d.groups)
	}
	// Type and Name are the required keys; Exec, Icon and Categories are what
	// make the entry usable rather than merely valid.
	for _, k := range []string{"Type", "Name", "Exec", "Icon", "Categories", "Terminal"} {
		if strings.TrimSpace(d.keys[k]) == "" {
			t.Errorf("%s is missing or empty; the entry has %v", k, d.order)
		}
	}
	if d.keys["Type"] != "Application" {
		t.Errorf("Type = %q, want Application", d.keys["Type"])
	}
	if d.keys["Name"] != "Packapp" {
		t.Errorf("Name = %q", d.keys["Name"])
	}
	// Categories is a list, and the spec requires the trailing semicolon.
	if !strings.HasSuffix(d.keys["Categories"], ";") {
		t.Errorf("Categories = %q, want a semicolon-terminated list", d.keys["Categories"])
	}
	main := strings.Split(strings.TrimSuffix(d.keys["Categories"], ";"), ";")[0]
	if _, ok := desktopMainCategories[strings.ToLower(main)]; !ok {
		t.Errorf("Categories names %q, which is not a registered main category", main)
	}
	// Icon is a name, not a path: the theme lookup is what finds it in whatever
	// prefix it was installed into, and a path works in some desktops only.
	if strings.ContainsRune(d.keys["Icon"], '/') || strings.HasSuffix(d.keys["Icon"], ".png") {
		t.Errorf("Icon = %q, want a bare theme name", d.keys["Icon"])
	}
	// Exec must be relocatable in the staged tree — an absolute path here would
	// be the path on the packaging machine.
	if strings.HasPrefix(d.keys["Exec"], "/") {
		t.Errorf("Exec = %q, which only exists on the machine that packaged it", d.keys["Exec"])
	}

	// And if the distribution's own validator is installed, let it have the
	// last word. Absent, this is a capability skip: nothing else validates it.
	if have("desktop-file-validate") {
		if out, err := exec.Command("desktop-file-validate", entry).CombinedOutput(); err != nil {
			t.Errorf("desktop-file-validate: %v\n%s", err, out)
		}
	} else {
		t.Log("capability skip: desktop-file-validate is not installed, so only the " +
			"required keys were checked, not the whole grammar")
	}
}

// The icon has to land where the freedesktop icon theme specification says, at
// the size the directory claims, or a panel asking for 24px gets nothing.
func TestIconsFollowTheThemeSpecification(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "linux", out: out, icon: testIcon(t, 512), id: "com.gophics.packapp"})
	hicolor := filepath.Join(out, "Packapp-1.2.3-linux-"+runtime.GOARCH,
		"usr", "share", "icons", "hicolor")
	found := 0
	err := filepath.Walk(hicolor, func(p string, fi os.FileInfo, err error) error {
		if err != nil || fi.IsDir() {
			return err
		}
		rel, err := filepath.Rel(hicolor, p)
		if err != nil {
			return err
		}
		parts := strings.Split(filepath.ToSlash(rel), "/")
		if len(parts) != 3 || parts[1] != "apps" {
			t.Errorf("%s is not <size>/apps/<name>.png", rel)
			return nil
		}
		if parts[2] != "com.gophics.packapp.png" {
			t.Errorf("icon is named %s; the theme lookup uses the Icon key's value", parts[2])
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		cfg, err := png.DecodeConfig(f)
		if err != nil {
			t.Errorf("%s: %v", rel, err)
			return nil
		}
		// The directory names the size; a 48px image in 32x32/ is what makes a
		// desktop draw a scaled icon while believing it did not.
		if parts[0] != fmt.Sprintf("%dx%d", cfg.Width, cfg.Height) {
			t.Errorf("%s holds a %dx%d image", rel, cfg.Width, cfg.Height)
		}
		found++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if found < 5 {
		t.Errorf("only %d theme icons; the standard sizes are %v", found, hicolorSizes)
	}
}

// The tarball is the minimum deliverable and has to be relocatable: every path
// inside it relative, everything under one directory, and the executable bit
// intact — a tarball whose binary unpacks without +x is a bug report that
// reads "it does not start".
func TestTarballIsRelocatable(t *testing.T) {
	out := t.TempDir()
	_, _, _ = runPack(t, packOpts{goos: "linux", out: out, icon: testIcon(t, 256)})
	tgz := filepath.Join(out, "Packapp-1.2.3-linux-"+runtime.GOARCH+".tar.gz")
	f, err := os.Open(tgz)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	top := map[string]bool{}
	var binMode int64
	seenBin := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		if strings.HasPrefix(hdr.Name, "/") || strings.Contains(hdr.Name, "..") {
			t.Errorf("member %q is not relative", hdr.Name)
		}
		top[strings.Split(hdr.Name, "/")[0]] = true
		if hdr.Uid != 0 || hdr.Uname != "" {
			t.Errorf("member %q carries uid %d / %q from the packaging machine", hdr.Name, hdr.Uid, hdr.Uname)
		}
		if path.Base(hdr.Name) == "packapp" {
			seenBin, binMode = true, hdr.Mode
		}
	}
	if len(top) != 1 {
		t.Errorf("the tarball has %d top-level entries (%v); untarring it should make one directory", len(top), top)
	}
	if !seenBin {
		t.Fatal("the tarball has no usr/bin/packapp")
	}
	if binMode&0o111 == 0 {
		t.Errorf("the binary unpacks with mode %o, which cannot be run", binMode)
	}
}

// Cross-built from whatever host runs the tests, the binary still has to be a
// Linux one. This is the property the zero-cgo invariant buys and the reason
// desktop packaging can be done for all three platforms from one machine.
func TestLinuxBinaryIsAnELF(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "linux", out: out})
	bin := filepath.Join(out, "Packapp-1.2.3-linux-"+runtime.GOARCH, "usr", "bin", "packapp")
	ef, err := elf.Open(bin)
	if err != nil {
		t.Fatalf("%s is not an ELF binary: %v", bin, err)
	}
	defer ef.Close()
	if ef.OSABI != elf.ELFOSABI_NONE && ef.OSABI != elf.ELFOSABI_LINUX {
		t.Errorf("ELF OSABI = %v", ef.OSABI)
	}
}

// The install script runs on whatever the user's distribution calls /bin/sh,
// which on Debian is dash. A bashism in it fails on exactly the machines
// nobody tested on, so the shell itself checks the syntax.
func TestInstallScriptIsPOSIXShell(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "linux", out: out})
	script := filepath.Join(out, "Packapp-1.2.3-linux-"+runtime.GOARCH, "install.sh")
	fi, err := os.Stat(script)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("install.sh is not executable (mode %v)", fi.Mode())
	}
	if msg, err := exec.Command("sh", "-n", script).CombinedOutput(); err != nil {
		t.Errorf("sh -n install.sh: %v\n%s", err, msg)
	}
	// It has to default to a prefix that needs no root, or the first thing the
	// instructions ask for is sudo.
	body, err := os.ReadFile(script)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "$HOME/.local") {
		t.Error("install.sh does not default to ~/.local, so it needs root for no reason")
	}
}

// An AppDir is identified by three things at its root. Missing any of them,
// appimagetool fails with a message about the AppDir rather than about what is
// absent from it.
func TestAppDirHasWhatTheRuntimeNeeds(t *testing.T) {
	out := t.TempDir()
	runPack(t, packOpts{goos: "linux", out: out, appimage: true,
		icon: testIcon(t, 512), id: "com.gophics.packapp"})
	appdir := filepath.Join(out, "Packapp.AppDir")

	apprun := filepath.Join(appdir, "AppRun")
	fi, err := os.Stat(apprun)
	if err != nil {
		t.Fatalf("AppRun: %v", err)
	}
	if fi.Mode().Perm()&0o111 == 0 {
		t.Errorf("AppRun is not executable (mode %v)", fi.Mode())
	}
	if msg, err := exec.Command("sh", "-n", apprun).CombinedOutput(); err != nil {
		t.Errorf("sh -n AppRun: %v\n%s", err, msg)
	}
	body, err := os.ReadFile(apprun)
	if err != nil {
		t.Fatal(err)
	}
	// The mount point differs on every launch, so an absolute path in AppRun is
	// a bundle that only runs on the machine that built it.
	if !strings.Contains(string(body), "$HERE") {
		t.Error("AppRun does not resolve its payload relative to the mount point")
	}

	// The desktop file and an icon named for its Icon key, both at the root.
	d := parseDesktopFile(t, filepath.Join(appdir, "com.gophics.packapp.desktop"))
	icon := filepath.Join(appdir, d.keys["Icon"]+".png")
	if _, err := os.Stat(icon); err != nil {
		t.Errorf("the AppDir has no %s.png for its Icon key: %v", d.keys["Icon"], err)
	}
	if _, err := os.Stat(filepath.Join(appdir, ".DirIcon")); err != nil {
		t.Errorf(".DirIcon: %v", err)
	}
	// install.sh belongs to the tarball; an image that mounts itself read-only
	// has nothing to install.
	if _, err := os.Stat(filepath.Join(appdir, "install.sh")); err == nil {
		t.Error("install.sh was copied into the AppDir")
	}
}

// A name is whatever the developer typed, and a desktop entry is not an INI
// file: the spec gives it C-style escapes, so a backslash that is passed
// through ends the value early and makes the rest of the file garbage.
func TestDesktopValueEscapes(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{`plain`, `plain`},
		{`C:\path`, `C:\\path`},
		{"two\nlines", `two\nlines`},
		{"a\ttab", `a\ttab`},
	} {
		if got := desktopValue(c.in); got != c.want {
			t.Errorf("desktopValue(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestDesktopCategoryFallsBackLoudly(t *testing.T) {
	if got, ok := desktopCategory("Graphics"); got != "Graphics" || !ok {
		t.Errorf("desktopCategory(Graphics) = %q, %v", got, ok)
	}
	if got, ok := desktopCategory("graphics;"); got != "Graphics" || !ok {
		t.Errorf("desktopCategory(graphics;) = %q, %v — case and the trailing separator are the developer's, not an error", got, ok)
	}
	// Finance is a freedesktop *sub*category, so it is not valid on its own.
	if got, ok := desktopCategory("Finance"); got != "Utility" || ok {
		t.Errorf("desktopCategory(Finance) = %q, %v; want the Utility fallback reported as a fallback", got, ok)
	}
}
