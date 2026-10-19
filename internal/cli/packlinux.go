package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Linux packaging: a prefix tree, a desktop entry, themed icons, a
// relocatable tarball, and an AppDir an AppImage can be built from.
//
// There is no single Linux package format, and picking one (.deb, .rpm, Flatpak)
// would mean depending on that distribution's tooling, which cannot be run from
// the Mac that cross-built the binary. What *is* universal is the filesystem
// layout: an executable in <prefix>/bin, a .desktop file in
// <prefix>/share/applications, and icons under
// <prefix>/share/icons/hicolor/<size>x<size>/apps. Every desktop environment
// reads those, and every package format is a wrapper around that same tree —
// so this builds the tree, tars it, and ships an install script that copies it
// into a prefix. A developer who wants a .deb can point dpkg-deb at the tree;
// nothing is in the way.
//
// The tarball is relocatable: every path inside it is relative, the Exec line
// of the desktop entry names the bare command rather than a build-machine
// path, and the install script rewrites it to wherever the user actually put
// the files.

// desktopMainCategories are the registered main categories of the freedesktop
// menu specification. Validated rather than passed through: a Categories value
// a desktop does not recognise puts the app in no menu at all, which looks
// exactly like the app not being installed.
var desktopMainCategories = map[string]string{
	"audiovideo":  "AudioVideo",
	"audio":       "Audio",
	"video":       "Video",
	"development": "Development",
	"education":   "Education",
	"game":        "Game",
	"graphics":    "Graphics",
	"network":     "Network",
	"office":      "Office",
	"science":     "Science",
	"settings":    "Settings",
	"system":      "System",
	"utility":     "Utility",
}

// desktopCategory normalises the -category flag to a registered main category,
// reporting whether it had to fall back.
func desktopCategory(c string) (string, bool) {
	key := strings.ToLower(strings.Trim(strings.TrimSpace(c), ";"))
	if v, ok := desktopMainCategories[key]; ok {
		return v, true
	}
	return "Utility", false
}

func (p *packer) packLinux() ([]string, error) {
	out := p.outDir()
	// One directory named like the tarball, so untarring it anywhere gives a
	// self-describing folder rather than spraying usr/ into the user's cwd.
	stage := filepath.Join(out, p.artifactName())
	if err := os.RemoveAll(stage); err != nil {
		return nil, err
	}
	share := filepath.Join(stage, "usr", "share")

	p.step("building %s (%s)", p.slug, p.o.goarch)
	if err := p.buildBinary(filepath.Join(stage, "usr", "bin", p.slug), p.o.goarch); err != nil {
		return nil, err
	}

	// The desktop entry is named for the identifier, not for the app: the
	// specification keys a desktop file by its basename, and two apps called
	// tally.desktop from different authors overwrite each other in a shared
	// prefix. Reverse-DNS is what makes that impossible.
	if _, err := writeFileIn(filepath.Join(share, "applications"),
		p.o.id+".desktop", []byte(p.desktopEntry(p.slug)), 0o644); err != nil {
		return nil, err
	}
	if p.icon != nil {
		if _, err := p.icon.writeHicolor(share, p.o.id); err != nil {
			return nil, err
		}
	}
	if err := p.writeMetainfo(share); err != nil {
		return nil, err
	}
	if _, err := writeFileIn(stage, "install.sh", []byte(p.installScript()), 0o755); err != nil {
		return nil, err
	}

	artifacts := []string{stage}
	tarball := filepath.Join(out, p.artifactName()+".tar.gz")
	p.step("archiving %s", filepath.Base(tarball))
	if err := writeTarGz(tarball, stage); err != nil {
		return nil, err
	}
	artifacts = append(artifacts, tarball)

	if p.o.appimage {
		paths, err := p.buildAppImage(stage)
		if err != nil {
			return nil, err
		}
		artifacts = append(artifacts, paths...)
	}
	return artifacts, nil
}

// desktopEntry renders the .desktop file. exec is the Exec value: the bare
// command in the staged tree and in an AppDir, rewritten to an absolute path by
// the install script.
//
// The key set is the specification's required pair (Type, Name) plus the ones a
// desktop environment actually needs to show the app: Exec to launch it, Icon
// to draw it, Categories to file it in a menu, and StartupWMClass so the window
// that appears is associated with the launcher that started it — without it a
// GNOME or KDE dock shows a second, unnamed icon for the running app.
func (p *packer) desktopEntry(exec string) string {
	// Normally resolved by resolveMeta, which is also where the fallback is
	// reported; resolved here too so the renderer is callable on its own.
	cat := p.desktopCat
	if cat == "" {
		cat, _ = desktopCategory(p.o.category)
	}
	var b strings.Builder
	b.WriteString("[Desktop Entry]\n")
	// Version is the version of the specification the file follows, not the
	// app's — a mistake common enough that the spec calls it out.
	b.WriteString("Version=1.1\n")
	b.WriteString("Type=Application\n")
	b.WriteString("Name=" + desktopValue(p.o.name) + "\n")
	b.WriteString("Comment=" + desktopValue(p.o.comment) + "\n")
	b.WriteString("Exec=" + desktopValue(exec) + "\n")
	// A name, never a path: the icon theme lookup finds it in whichever prefix
	// and whichever theme it was installed into, and a user's icon theme can
	// override it. An absolute path works in some desktops and silently fails
	// in others.
	b.WriteString("Icon=" + p.o.id + "\n")
	b.WriteString("Terminal=false\n")
	b.WriteString("Categories=" + cat + ";\n")
	b.WriteString("StartupNotify=true\n")
	b.WriteString("StartupWMClass=" + p.slug + "\n")
	return b.String()
}

// desktopValue escapes a value for a desktop entry.
//
// The format is not INI: the spec gives it C-style escapes, and a literal
// newline or a stray backslash in a Name — which comes from whatever the
// developer typed — ends the key early and makes the rest of the file garbage.
func desktopValue(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

// writeMetainfo writes the AppStream component file, which is what a software
// centre (GNOME Software, KDE Discover) reads to show the app at all: a .desktop
// file alone gets a menu entry and no store listing.
//
// Skipped without a license, because project_license is a required field and a
// guess there is a licensing claim made on the developer's behalf.
func (p *packer) writeMetainfo(share string) error {
	if p.o.license == "" {
		p.note("no AppStream metainfo: it requires a project_license, which cannot be " +
			"guessed. Pass -license <SPDX id> (e.g. MIT, Apache-2.0, GPL-3.0-or-later) to " +
			"generate one; the app still appears in menus without it.")
		return nil
	}
	x := func(s string) string { return xmlEscape(s) }
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!-- Generated by gophics package. -->
<component type="desktop-application">
  <id>%s</id>
  <name>%s</name>
  <summary>%s</summary>
  <metadata_license>CC0-1.0</metadata_license>
  <project_license>%s</project_license>
  <launchable type="desktop-id">%s.desktop</launchable>
  <description>
    <p>%s</p>
  </description>
  <releases>
    <release version="%s"/>
  </releases>
</component>
`, x(p.o.id), x(p.o.name), x(p.o.comment), x(p.o.license), x(p.o.id), x(p.o.comment), x(p.o.version))
	_, err := writeFileIn(filepath.Join(share, "metainfo"), p.o.id+".metainfo.xml", []byte(body), 0o644)
	return err
}

// installScript is the tarball's installer: a POSIX sh script that copies the
// prefix tree into a prefix the user chooses and makes the desktop entry point
// at it.
//
// POSIX sh, not zsh or bash: it runs on whatever the user's distribution ships
// as /bin/sh, which on Debian is dash and has none of the extensions. It
// defaults to ~/.local, so the common case needs no root — and ~/.local/share
// is the XDG data home every desktop already searches.
func (p *packer) installScript() string {
	return fmt.Sprintf(`#!/bin/sh
# Install or remove %s.
#
#   ./install.sh                 # into ~/.local (no root needed)
#   ./install.sh /usr/local      # system-wide (needs root)
#   ./install.sh --uninstall     # remove it again from ~/.local
#
# Everything this copies is relative to the directory the script sits in, so the
# unpacked tarball can live anywhere.
set -eu

uninstall=0
if [ "${1:-}" = "--uninstall" ] || [ "${1:-}" = "-u" ]; then
	uninstall=1
	shift
fi
prefix="${1:-${PREFIX:-$HOME/.local}}"
here=$(cd -- "$(dirname -- "$0")" && pwd)

bin="$prefix/bin/%s"
desktop="$prefix/share/applications/%s.desktop"

if [ "$uninstall" = 1 ]; then
	rm -f "$bin" "$desktop" "$prefix/share/metainfo/%s.metainfo.xml"
	# Only the icons this package installed, found by name, so a shared prefix
	# keeps everyone else's.
	find "$prefix/share/icons/hicolor" -name '%s.png' -delete 2>/dev/null || true
	echo "removed %s from $prefix"
	exit 0
fi

mkdir -p "$prefix"
# -R of the contents rather than of usr/ itself, so bin/, share/ and anything
# already in the prefix merge instead of nesting.
cp -R "$here/usr/." "$prefix/"
chmod 0755 "$bin"

# The staged Exec names the bare command, which only works when the prefix's
# bin is on PATH — and ~/.local/bin often is not, for a menu launcher's
# environment in particular. Rewriting it to the absolute path is what makes
# the menu entry work regardless.
tmp="$desktop.tmp.$$"
sed "s|^Exec=.*|Exec=$bin|" "$desktop" > "$tmp"
mv "$tmp" "$desktop"

# The caches are what make the entry and the icon appear without a logout. Both
# tools are optional: the files are correct either way, a desktop just notices
# them sooner.
if command -v update-desktop-database >/dev/null 2>&1; then
	update-desktop-database "$prefix/share/applications" 2>/dev/null || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
	gtk-update-icon-cache -q -t -f "$prefix/share/icons/hicolor" 2>/dev/null || true
fi

echo "installed %s into $prefix"
case ":$PATH:" in
	*":$prefix/bin:"*) ;;
	*) echo "note: $prefix/bin is not on your PATH; the menu entry works, the shell command does not." ;;
esac
`, p.o.name, p.slug, p.o.id, p.o.id, p.o.id, p.o.name, p.o.name)
}

// buildAppImage assembles an AppDir and, if appimagetool is installed, turns it
// into a single-file AppImage.
//
// The AppDir is built either way, and that is the point: it is the whole
// content of an AppImage, so the developer who does not have appimagetool
// still has everything it needs and one command to run when they do.
// appimagetool itself is only distributed as a Linux AppImage, so packaging
// from a Mac can never produce the final file — detecting that and saying so
// beats a cryptic exec failure.
func (p *packer) buildAppImage(stage string) ([]string, error) {
	appdir := filepath.Join(p.outDir(), p.o.name+".AppDir")
	if err := os.RemoveAll(appdir); err != nil {
		return nil, err
	}
	if err := copyTree(stage, appdir); err != nil {
		return nil, err
	}
	// The install script belongs to the tarball, not to a bundle that mounts
	// itself read-only.
	if err := os.Remove(filepath.Join(appdir, "install.sh")); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	// An AppDir is identified by three things at its root: AppRun, a desktop
	// file, and an icon named exactly as the desktop file's Icon key. The
	// runtime mounts the image and execs AppRun with nothing else assumed.
	apprun := fmt.Sprintf(`#!/bin/sh
# Entry point of the %s AppImage. HERE is the mount point the runtime chose,
# which is a different path on every launch — so everything is resolved
# relative to it and nothing may be an absolute path.
HERE=$(dirname -- "$(readlink -f -- "$0")")
export PATH="$HERE/usr/bin:${PATH:-}"
export XDG_DATA_DIRS="$HERE/usr/share:${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
exec "$HERE/usr/bin/%s" "$@"
`, p.o.name, p.slug)
	if _, err := writeFileIn(appdir, "AppRun", []byte(apprun), 0o755); err != nil {
		return nil, err
	}
	if _, err := writeFileIn(appdir, p.o.id+".desktop", []byte(p.desktopEntry(p.slug)), 0o644); err != nil {
		return nil, err
	}
	if p.icon != nil {
		// The root icon and .DirIcon are what the runtime and the file manager
		// read; 256px is the size appimagetool expects to find.
		data, ok, err := p.icon.pngAt(256)
		if err != nil {
			return nil, err
		}
		if !ok {
			data, _, err = p.icon.pngAt(p.icon.source)
			if err != nil {
				return nil, err
			}
		}
		if _, err := writeFileIn(appdir, p.o.id+".png", data, 0o644); err != nil {
			return nil, err
		}
		if _, err := writeFileIn(appdir, ".DirIcon", data, 0o644); err != nil {
			return nil, err
		}
	}
	artifacts := []string{appdir}

	if !have("appimagetool") {
		p.note("AppDir written but no AppImage: appimagetool is not on PATH%s.\n"+
			"  Download it (https://github.com/AppImage/appimagetool/releases), chmod +x it, then\n"+
			"    ARCH=%s appimagetool %s",
			appimageHostHint(), appimageArch(p.o.goarch), filepath.Base(appdir))
		return artifacts, nil
	}
	img := filepath.Join(p.outDir(), fmt.Sprintf("%s-%s-%s.AppImage", p.o.name, p.o.version, appimageArch(p.o.goarch)))
	p.step("building %s", filepath.Base(img))
	if err := run("", []string{"ARCH=" + appimageArch(p.o.goarch)}, "appimagetool", appdir, img); err != nil {
		return nil, fmt.Errorf("appimagetool: %w", err)
	}
	return append(artifacts, img), nil
}

func appimageHostHint() string {
	if runtime.GOOS != "linux" {
		return " (and it is distributed only as a Linux AppImage, so it cannot run on " + runtime.GOOS + ")"
	}
	return ""
}

// appimageArch is the architecture name the AppImage tooling uses, which is
// uname's spelling rather than Go's.
func appimageArch(goarch string) string {
	switch goarch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	case "386":
		return "i686"
	case "arm":
		return "armhf"
	default:
		return goarch
	}
}

// xmlEscape escapes text for an XML element body or attribute value.
func xmlEscape(s string) string {
	var b strings.Builder
	plistEscape(&b, s)
	return b.String()
}
