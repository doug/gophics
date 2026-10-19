package cli

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"crypto/sha1"
	"encoding/hex"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

// Desktop packaging: turning a built binary into something a non-developer can
// install.
//
// `gophics build` produces an executable, which is all a developer needs and
// nothing a user can be handed: double-clicking a Unix binary on a Mac opens a
// terminal, a Linux desktop has no menu entry for it, and Windows reports an
// unknown publisher with no version information. The mobile targets have had
// generated host projects since the beginning, so desktop — the platform this
// framework is best at — was the one you could not ship.
//
// The shape here follows the mobile path deliberately. One command per job
// (`build` compiles, `package` distributes), templated per-platform files
// alongside generated metadata, and every external tool optional: a developer
// with no Apple Developer account, no WiX and no appimagetool still gets a
// working unsigned .app, a relocatable tarball and a staged Windows directory.
// Signing and notarisation are hooks, not requirements — they no-op with an
// explanation rather than failing a build, because the first thing anyone does
// with a new packaging command is run it before they have any credentials, and
// a failure there reads as "this does not work".
//
// Nothing in here shells out to a tool it cannot do without. The archives are
// written with archive/tar, archive/zip and compress/gzip; the icons are
// converted in-process (see packicon.go); the plists, .desktop entries,
// manifests and installer sources are generated. The optional tools are
// hdiutil (disk image), lipo (universal binary), codesign + notarytool
// (signing), appimagetool (AppImage), windres/go-winres (embedded version
// resource) and makensis/wix (compiled installer) — each probed, each skipped
// with a message naming what to install.

// packOpts is the resolved configuration of one `gophics package` run.
type packOpts struct {
	pkg    string // Go package to build (default ".")
	goos   string // target OS: darwin, linux or windows
	goarch string // target arch, or "universal" on darwin
	tags   string // extra build tags, as `gophics build -tags`
	out    string // output directory (default build/package/<goos>)

	name      string // display name, e.g. "Tally"
	id        string // bundle/application identifier, reverse-DNS
	version   string // marketing version, e.g. 1.2.0
	build     string // build number, monotonic per release
	icon      string // square PNG to convert; "" = <pkgdir>/icon.png if present
	publisher string // who made it: Windows properties, MSI Manufacturer
	copyright string
	category  string // freedesktop main category; mapped for macOS
	license   string // SPDX identifier, for the AppStream metainfo
	comment   string // one-line description

	identity string // macOS codesigning identity; "" = leave unsigned
	notary   string // notarytool keychain profile; "" = do not notarise
	dmg      bool   // also build a .dmg (darwin)
	appimage bool   // also build an AppImage (linux)
	console  bool   // keep the Windows console window
}

// packer carries one packaging run: the resolved options plus the few facts
// derived from the package itself.
type packer struct {
	o   packOpts
	dir string    // the app's source directory
	out io.Writer // progress and skipped-step notices

	slug       string // lowercase identifier-safe name, for file and binary names
	icon       *iconSet
	desktopCat string // the resolved freedesktop main category

	// defaultedID records that no -id was given. Reported once at the end
	// rather than at resolve time: an identifier appears in the .app, the
	// .desktop file name, the registry key and the AppImage, and shipping two
	// apps that both claim com.example.foo makes them overwrite each other's
	// settings. Worth one line of warning, not a refusal.
	defaultedID bool
}

func cmdPackage(args []string) error {
	fs := flag.NewFlagSet("package", flag.ContinueOnError)
	var o packOpts
	addPackFlags(fs, &o)
	if err := fs.Parse(flagsFirst(fs, args)); err != nil {
		return err
	}
	if err := o.resolve(fs); err != nil {
		return err
	}
	p := &packer{o: o, out: os.Stderr}
	arts, err := p.run()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: packaged %s for %s/%s →\n", p.o.name, p.o.goos, p.o.goarch)
	for _, a := range arts {
		fmt.Fprintf(os.Stderr, "  %s\n", a)
	}
	if p.defaultedID {
		fmt.Fprintf(os.Stderr, "\nnote: the bundle identifier defaulted to %s. Two apps that share one\n"+
			"identifier share their preferences and overwrite each other's installs; pass\n"+
			"-id com.yourdomain.%s before you give this to anyone.\n", p.o.id, p.slug)
	}
	return nil
}

func addPackFlags(fs *flag.FlagSet, o *packOpts) {
	fs.StringVar(&o.goos, "os", "", "target OS: darwin|linux|windows (default: GOOS, else the host)")
	fs.StringVar(&o.goarch, "arch", "", "target architecture, or \"universal\" for a fat darwin binary (default: GOARCH, else the host)")
	fs.StringVar(&o.tags, "tags", "", "extra comma-separated build tags")
	fs.StringVar(&o.out, "o", "", "output directory (default build/package/<os>)")
	fs.StringVar(&o.name, "name", "", "display name (default: the package directory, capitalized)")
	fs.StringVar(&o.id, "id", "", "bundle/application identifier in reverse-DNS form (default com.example.<name>)")
	fs.StringVar(&o.version, "version", "1.0.0", "marketing version")
	fs.StringVar(&o.build, "build", "1", "build number")
	fs.StringVar(&o.icon, "icon", "", "square PNG to convert into platform icons (default: icon.png beside the package)")
	fs.StringVar(&o.comment, "comment", "", "one-line description, for the desktop entry and installers")
	fs.StringVar(&o.publisher, "publisher", "", "publisher name for Windows properties and the MSI (default: from -id)")
	fs.StringVar(&o.copyright, "copyright", "", "copyright line shown in Finder and Windows properties")
	fs.StringVar(&o.category, "category", "Utility", "freedesktop main category (Utility, Game, Graphics, Office, …); mapped for macOS")
	fs.StringVar(&o.license, "license", "", "SPDX license identifier, for the AppStream metainfo (linux)")
	fs.StringVar(&o.identity, "identity", "", "macOS codesigning identity (default $CODESIGN_ID; unsigned when neither is set)")
	fs.StringVar(&o.notary, "notary-profile", "", "notarytool keychain profile (default $NOTARY_PROFILE; skipped when neither is set)")
	fs.BoolVar(&o.dmg, "dmg", true, "also build a .dmg disk image (darwin; needs hdiutil)")
	fs.BoolVar(&o.appimage, "appimage", false, "also build an AppImage (linux; needs appimagetool)")
	fs.BoolVar(&o.console, "console", false, "keep the Windows console window (default: a GUI app with none)")
}

// resolve fills in what the flags did not say. Credentials come from the
// environment as well as from flags because that is where a CI job keeps them
// and where the per-app shell scripts this replaces already looked.
func (o *packOpts) resolve(fs *flag.FlagSet) error {
	o.pkg = "."
	if fs.NArg() > 0 {
		o.pkg = fs.Arg(0)
	}
	if o.goos == "" {
		o.goos = targetGOOS()
	}
	switch o.goos {
	case "darwin", "linux", "windows":
	default:
		return fmt.Errorf("no desktop packaging for %s (want darwin|linux|windows); "+
			"web ships as the build/web directory and mobile through `gophics build -p ios|android`", o.goos)
	}
	if o.goarch == "" {
		o.goarch = targetGOARCH()
	}
	if o.goarch == "universal" && o.goos != "darwin" {
		return fmt.Errorf("-arch universal is a darwin thing (lipo); name a single architecture for %s", o.goos)
	}
	if o.identity == "" {
		o.identity = os.Getenv("CODESIGN_ID")
	}
	if o.notary == "" {
		o.notary = os.Getenv("NOTARY_PROFILE")
	}
	if strings.TrimSpace(o.version) == "" {
		return fmt.Errorf("-version is empty")
	}
	return nil
}

// targetGOARCH is the architecture to package for: GOARCH from the environment
// when set, the host's otherwise.
//
// The pair of targetGOOS, and here rather than beside it because packaging is
// the only command that needs to name an architecture — `build` lets the go
// tool read the variable itself, while packaging puts the architecture in file
// names and in a Windows installer's platform attribute, so it has to know the
// answer rather than pass it through.
func targetGOARCH() string {
	if a := os.Getenv("GOARCH"); a != "" {
		return a
	}
	return runtime.GOARCH
}

// run packages for one platform and returns the artifacts it produced.
func (p *packer) run() ([]string, error) {
	if err := p.resolveMeta(); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(p.outDir(), 0o755); err != nil {
		return nil, err
	}
	switch p.o.goos {
	case "darwin":
		return p.packMacOS()
	case "linux":
		return p.packLinux()
	default:
		return p.packWindows()
	}
}

// resolveMeta derives the facts the flags left unsaid from the package itself.
func (p *packer) resolveMeta() error {
	dir, err := packageDir(p.o.pkg)
	if err != nil {
		return err
	}
	p.dir = dir
	if p.o.name == "" {
		p.o.name = titleFirst(filepath.Base(dir))
	}
	if err := checkName(p.o.name); err != nil {
		return err
	}
	p.slug = sanitizeIdent(p.o.name)
	if p.o.id == "" {
		p.o.id = "com.example." + p.slug
		p.defaultedID = true
	}
	if p.o.comment == "" {
		p.o.comment = p.o.name
	}
	if p.o.publisher == "" {
		p.o.publisher = publisherFromID(p.o.id)
	}
	// Resolved once here rather than where it is used: the desktop entry is
	// rendered twice on the AppImage path — into the prefix tree and again into
	// the AppDir — and a fallback reported twice reads as two problems.
	cat, known := desktopCategory(p.o.category)
	p.desktopCat = cat
	if !known && p.o.goos == "linux" {
		p.note("%q is not a freedesktop main category, so the desktop entry says %s. "+
			"The registered set is AudioVideo, Audio, Video, Development, Education, Game, "+
			"Graphics, Network, Office, Science, Settings, System, Utility.", p.o.category, cat)
	}
	// The icon is a convention before it is a flag: examples/tally already
	// keeps a 1024px icon.png beside its main package, generated by its own
	// icon program, and that is the shape to reward. No icon at all is fine —
	// the app gets the system's generic one, which is what an unpackaged
	// binary gets today.
	if p.o.icon == "" {
		if c := filepath.Join(dir, "icon.png"); fileExists(c) {
			p.o.icon = c
		}
	}
	if p.o.icon != "" {
		set, err := loadIconSet(p.o.icon)
		if err != nil {
			return err
		}
		p.icon = set
		if set.source < 512 {
			p.note("%s is %dx%d; macOS shows icons at 1024px, so a larger source "+
				"would look sharper. Sizes above the source are skipped rather than upscaled.",
				filepath.Base(p.o.icon), set.source, set.source)
		}
	} else {
		p.note("no icon: pass -icon <square.png> or keep an icon.png beside the " +
			"package. The app gets the platform's generic icon without one.")
	}
	return nil
}

// publisherFromID guesses a publisher from a reverse-DNS identifier:
// com.example.tally → Example.
//
// WiX requires a Manufacturer and Windows shows a publisher in its properties
// and SmartScreen dialog, so something has to be there — and the organisation
// is already named in the identifier the developer chose. A guess from
// information they gave is better than the literal string "Unknown", and
// -publisher overrides it.
func publisherFromID(id string) string {
	parts := strings.Split(id, ".")
	if len(parts) >= 2 {
		return titleFirst(parts[1])
	}
	return titleFirst(id)
}

func (p *packer) outDir() string {
	if p.o.out != "" {
		return p.o.out
	}
	// Beside build/<platform> rather than inside it: `gophics build -p desktop`
	// owns build/desktop and wipes nothing, so a packaging run that wrote there
	// would leave a stale .app next to a fresh binary with no way to tell.
	return filepath.Join("build", "package", p.o.goos)
}

// artifactName is the conventional name of a distributable: Tally-1.2.0-linux-amd64.
func (p *packer) artifactName() string {
	return fmt.Sprintf("%s-%s-%s-%s", p.o.name, p.o.version, p.o.goos, p.o.goarch)
}

// step announces a stage of the packaging run.
func (p *packer) step(format string, a ...any) {
	fmt.Fprintf(p.out, "gophics: "+format+"\n", a...)
}

// note reports something skipped or worth knowing without stopping the run.
//
// Every optional tool goes through here, and the wording matters more than it
// looks: a packaging command is first run by someone who has none of the
// credentials, and a line that says what was skipped and how to enable it is
// the difference between "unsigned, as expected" and "this is broken".
func (p *packer) note(format string, a ...any) {
	fmt.Fprintf(p.out, "gophics: note — "+format+"\n", a...)
}

// buildBinary compiles the app for one GOOS/GOARCH.
//
// CGO_ENABLED=0 is explicit rather than assumed. The zero-cgo invariant means
// it is already true for every build here, and stating it is what makes
// packaging for Linux and Windows from a Mac a plain `go build` with no
// cross-toolchain — the property the whole packaging story rests on. -trimpath
// and -s -w are release settings: no local paths in the binary a user receives,
// and no symbol table it has no use for.
func (p *packer) buildBinary(dst, goarch string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	args := []string{"build", "-trimpath", "-ldflags", p.ldflags(), "-o", dst}
	if t := tagList(platform{name: "desktop"}, p.o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	args = append(args, p.o.pkg)
	env := []string{"CGO_ENABLED=0", "GOOS=" + p.o.goos, "GOARCH=" + goarch}
	if err := run("", env, "go", args...); err != nil {
		return fmt.Errorf("go build (%s/%s): %w", p.o.goos, goarch, err)
	}
	return nil
}

// ldflags are the link flags for a packaged build.
//
// -s -w drop the symbol table and DWARF: a user has no use for either, and
// they are a third of the binary. -H=windowsgui is the one that matters to
// whoever receives the app — without it Windows gives the process a console,
// so double-clicking the .exe opens a black window behind the app that closes
// it when dismissed. It costs the app its stdout, which is why -console exists
// for anyone debugging a packaged build.
func (p *packer) ldflags() string {
	f := []string{"-s", "-w"}
	if p.o.goos == "windows" && !p.o.console {
		f = append(f, "-H=windowsgui")
	}
	return strings.Join(f, " ")
}

// --- archives ---------------------------------------------------------------

// writeTarGz archives root as a single top-level directory named after it.
//
// Written with archive/tar rather than by calling tar(1), whose flags differ
// between BSD and GNU and which is not on a Windows host at all — and the
// packaging of a Linux app has to work from the Mac that built it.
func writeTarGz(dst, root string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	gz := gzip.NewWriter(f)
	tw := tar.NewWriter(gz)
	top := filepath.Base(root)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(fi, "")
		if err != nil {
			return err
		}
		hdr.Name = path.Join(top, filepath.ToSlash(rel))
		if d.IsDir() {
			hdr.Name += "/"
		}
		// A tarball that carries the packaging machine's uid, name and
		// timestamps is both a privacy leak and unreproducible: two identical
		// builds differ. Zeroing them costs nothing and means the archive is a
		// function of its contents.
		hdr.Uid, hdr.Gid = 0, 0
		hdr.Uname, hdr.Gname = "", ""
		hdr.ModTime = hdr.ModTime.UTC().Truncate(1)
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(tw, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	if err := gz.Close(); err != nil {
		return err
	}
	return f.Close()
}

// writeZip archives root as a single top-level directory named after it,
// preserving the executable bit (which a zip can carry in its external
// attributes, and which Windows ignores and Linux and macOS do not).
func writeZip(dst, root string) error {
	f, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer f.Close()
	zw := zip.NewWriter(f)
	top := filepath.Base(root)
	err = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := zip.FileInfoHeader(fi)
		if err != nil {
			return err
		}
		hdr.Name = path.Join(top, filepath.ToSlash(rel))
		hdr.Method = zip.Deflate
		if d.IsDir() {
			hdr.Name += "/"
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return f.Close()
}

// --- small shared helpers ---------------------------------------------------

// writeFileIn writes data to dir/name, creating the directory.
func writeFileIn(dir, name string, data []byte, perm os.FileMode) (string, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, data, perm); err != nil {
		return "", err
	}
	return p, nil
}

// uuidV5 derives a stable UUID from a name, as RFC 4122 section 4.3 does.
//
// WiX needs an UpgradeCode that never changes for the life of the product and
// component GUIDs that never change for a given file, or an upgrade installs
// beside the old version instead of replacing it. Deriving them from the
// bundle identifier makes them a function of the app rather than something a
// developer has to paste in and keep.
func uuidV5(namespace, name string) string {
	h := sha1.New()
	h.Write([]byte(namespace))
	h.Write([]byte(name))
	sum := h.Sum(nil)[:16]
	sum[6] = (sum[6] & 0x0f) | 0x50 // version 5
	sum[8] = (sum[8] & 0x3f) | 0x80 // RFC 4122 variant
	s := hex.EncodeToString(sum)
	return strings.ToUpper(s[0:8] + "-" + s[8:12] + "-" + s[12:16] + "-" + s[16:20] + "-" + s[20:32])
}

// fourPart renders a version as the four-field form Windows resources and MSI
// want: 1.2 → 1.2.0.0, and a trailing "-rc1" or "+meta" dropped, because the
// field is numeric and a build that puts a word in it fails to compile.
func fourPart(version string) string {
	v := version
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	fields := strings.Split(v, ".")
	out := make([]string, 4)
	for i := range out {
		out[i] = "0"
		if i < len(fields) {
			if n := digitsOnly(fields[i]); n != "" {
				out[i] = n
			}
		}
	}
	return strings.Join(out, ".")
}

func digitsOnly(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	return b.String()
}
