package cli

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/doug/gophics/internal/cli/capscan"
)

// macOS packaging: a real .app bundle, optionally signed, notarised and
// wrapped in a disk image.
//
// The bundle is the whole point. A Unix executable on a Mac cannot be
// double-clicked into an app — Finder opens a Terminal window, the Dock shows
// a generic icon with the binary's name, and the window server gives it no
// activation policy, so the menu bar belongs to whatever was in front. Only a
// bundle with an Info.plist is an application.
//
// Signing and notarisation are hooks. They are what Gatekeeper on *another*
// Mac requires, and they need an Apple Developer account, so neither can be a
// precondition of packaging: without an identity this writes an unsigned
// bundle and says so, and the developer who later gets a certificate passes
// -identity to the same command.

// macMinimumSystemVersion is LSMinimumSystemVersion for a packaged app.
//
// 11.0 is the first macOS that shipped on Apple silicon, which is this
// project's primary platform, and the oldest release any of this has been run
// on. It is stated rather than left out because an absent key lets Launch
// Services try the app on a release nobody has tested, where a missing
// dynamically-loaded symbol is a crash on launch rather than a refusal to
// install.
const macMinimumSystemVersion = "11.0"

func (p *packer) packMacOS() ([]string, error) {
	out := p.outDir()
	app := filepath.Join(out, p.o.name+".app")
	// Rebuilt from scratch, not merged into. A bundle that keeps yesterday's
	// Resources is how a renamed icon or a dropped file keeps working locally
	// and fails on the first machine that receives a clean copy — and a stale
	// _CodeSignature invalidates the signature of everything else in it.
	if err := os.RemoveAll(app); err != nil {
		return nil, err
	}
	contents := filepath.Join(app, "Contents")
	macosDir := filepath.Join(contents, "MacOS")
	resDir := filepath.Join(contents, "Resources")
	for _, d := range []string{macosDir, resDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, err
		}
	}

	p.step("building %s.app (%s)", p.o.name, p.o.goarch)
	if err := p.buildMacExecutable(filepath.Join(macosDir, p.o.name)); err != nil {
		return nil, err
	}

	iconFile := ""
	if p.icon != nil {
		iconFile = p.o.name
		if err := p.icon.writeICNS(filepath.Join(resDir, iconFile+".icns")); err != nil {
			return nil, err
		}
	}

	plist := encodePlist(p.infoPlist(iconFile))
	// Parsed back before anything is signed. The display name is whatever the
	// developer typed — "Tom & Jerry" is a fine app name — and an unescaped
	// one produces a bundle that Launch Services silently refuses while
	// codesign reports something about a resource envelope. Reading our own
	// output is two lines and names the actual problem.
	if _, err := parsePlist(plist); err != nil {
		return nil, fmt.Errorf("the generated Info.plist does not parse: %w", err)
	}
	if err := os.WriteFile(filepath.Join(contents, "Info.plist"), plist, 0o644); err != nil {
		return nil, err
	}
	// PkgInfo duplicates CFBundlePackageType and CFBundleSignature. It is
	// vestigial and Launch Services does not need it, but every bundle Xcode
	// produces has one and some older tooling looks for it first.
	if err := os.WriteFile(filepath.Join(contents, "PkgInfo"), []byte("APPL????"), 0o644); err != nil {
		return nil, err
	}

	artifacts := []string{app}

	// The entitlements live beside the bundle rather than inside it: they are
	// an input to codesign, not bundle content, and a copy shipped inside the
	// app would be a misleading description of what it was actually signed
	// with.
	ents := filepath.Join(out, p.o.name+".entitlements")
	if err := os.WriteFile(ents, []byte(p.macEntitlements()), 0o644); err != nil {
		return nil, err
	}
	artifacts = append(artifacts, ents)

	if err := p.signMacApp(app, ents); err != nil {
		return nil, err
	}

	var dmg string
	if p.o.dmg {
		var err error
		if dmg, err = p.buildDMG(app); err != nil {
			return nil, err
		}
		if dmg != "" {
			artifacts = append(artifacts, dmg)
		}
	}
	if err := p.notarizeMac(app, dmg); err != nil {
		return nil, err
	}
	return artifacts, nil
}

// buildMacExecutable compiles the bundle's executable, joining the two
// architectures into one binary when -arch universal was asked for.
//
// A universal binary is what the Mac App Store expects and what lets one
// download run on both Apple silicon and Intel. lipo does the joining and
// ships with the Xcode command line tools; without it the two slices cannot be
// merged at all, so the run falls back to a single architecture and says which
// one, rather than failing.
func (p *packer) buildMacExecutable(dst string) error {
	if p.o.goarch != "universal" {
		return p.buildBinary(dst, p.o.goarch)
	}
	if !have("lipo") {
		p.o.goarch = targetGOARCH()
		p.note("lipo not found, so no universal binary: built %s only. "+
			"lipo comes with the Xcode command line tools (xcode-select --install).", p.o.goarch)
		return p.buildBinary(dst, p.o.goarch)
	}
	slices := make([]string, 0, 2)
	for _, arch := range []string{"arm64", "amd64"} {
		slice := dst + "-" + arch
		if err := p.buildBinary(slice, arch); err != nil {
			return err
		}
		defer os.Remove(slice)
		slices = append(slices, slice)
	}
	args := append([]string{"-create", "-output", dst}, slices...)
	if err := run("", nil, "lipo", args...); err != nil {
		return fmt.Errorf("lipo: %w", err)
	}
	return os.Chmod(dst, 0o755)
}

// infoPlist is the bundle's Info.plist.
func (p *packer) infoPlist(iconFile string) plistObj {
	d := plistObj{
		{"CFBundleDevelopmentRegion", "en"},
		{"CFBundleDisplayName", p.o.name},
		{"CFBundleExecutable", p.o.name},
		{"CFBundleIdentifier", p.o.id},
		{"CFBundleInfoDictionaryVersion", "6.0"},
		{"CFBundleName", p.o.name},
		{"CFBundlePackageType", "APPL"},
		{"CFBundleShortVersionString", p.o.version},
		{"CFBundleSignature", "????"},
		{"CFBundleSupportedPlatforms", []string{"MacOSX"}},
		{"CFBundleVersion", p.o.build},
		{"LSMinimumSystemVersion", macMinimumSystemVersion},
		{"LSApplicationCategoryType", macCategory(p.o.category)},
		// Without this the window server hands the app a 1x backing store and
		// upscales it, which on every Mac made since 2012 means a blurred
		// window. The logical/physical split in this framework assumes the
		// real scale factor.
		{"NSHighResolutionCapable", true},
		// gophics renders through the GPU, and on a laptop with two of them
		// macOS otherwise keeps the discrete GPU powered for the app's whole
		// lifetime. Allowing the switch is what lets an idle window sit on the
		// integrated GPU.
		{"NSSupportsAutomaticGraphicsSwitching", true},
	}
	if iconFile != "" {
		d = append(d, plistKV{"CFBundleIconFile", iconFile})
	}
	if p.o.copyright != "" {
		d = append(d, plistKV{"NSHumanReadableCopyright", p.o.copyright})
	}
	return d
}

// macCategory maps the freedesktop main category the -category flag takes to
// the LSApplicationCategoryType the App Store and Finder's "Applications"
// grouping read.
//
// One flag for two vocabularies on purpose: the category is the same fact
// about the app, and asking a developer for both spellings of it invites them
// to disagree. An unrecognised value passes through when it already looks like
// a UTI, so a developer who wants a type with no freedesktop equivalent can
// still name it.
func macCategory(c string) string {
	if strings.HasPrefix(c, "public.app-category.") {
		return c
	}
	switch strings.ToLower(strings.TrimSuffix(c, ";")) {
	case "game":
		return "public.app-category.games"
	case "graphics":
		return "public.app-category.graphics-design"
	case "audiovideo", "audio", "video":
		return "public.app-category.video"
	case "office":
		return "public.app-category.productivity"
	case "development":
		return "public.app-category.developer-tools"
	case "education":
		return "public.app-category.education"
	case "network":
		return "public.app-category.social-networking"
	case "finance":
		return "public.app-category.finance"
	case "science":
		return "public.app-category.education"
	case "system", "settings":
		return "public.app-category.utilities"
	default:
		return "public.app-category.utilities"
	}
}

// macEntitlements renders the entitlements for the capabilities the app
// actually reaches, by the same scan the Android manifest and the iOS usage
// descriptions come from.
//
// Derived rather than asked for, because it is derivable: an app that never
// calls ctx.Camera() has no business declaring the camera entitlement, and one
// that does cannot ship without it. A scan that fails is not fatal — the file
// is only an input to a signing step that may not happen — so it degrades to
// the baseline and says so.
func (p *packer) macEntitlements() string {
	target := capscan.Target{GOOS: "darwin"}
	if p.o.tags != "" {
		target.Tags = strings.Split(p.o.tags, ",")
	}
	res, err := capscan.Scan(p.dir, ".", target)
	if err != nil {
		p.note("could not scan capabilities (%v);\n"+
			"  the entitlements file carries the sandbox baseline only — check it before "+
			"submitting to the App Store.", err)
		return capscan.MacEntitlements(capscan.Permissions(capscan.Result{}))
	}
	return capscan.MacEntitlements(capscan.Permissions(res))
}

// signMacApp signs the bundle when there is an identity to sign it with.
//
// No identity is the normal case and not an error: the bundle this produced
// runs on the machine that built it, which is what a developer wants from the
// first ten runs of a packaging command. The message says what the signature
// would buy, because the failure it prevents happens on someone else's Mac and
// is otherwise discovered by that person.
func (p *packer) signMacApp(app, entitlements string) error {
	if p.o.identity == "" {
		p.note("unsigned (no -identity and no $CODESIGN_ID).\n" +
			"  It runs here; Gatekeeper on another Mac will refuse to open it. To sign:\n" +
			"    gophics package -identity \"Developer ID Application: NAME (TEAMID)\"\n" +
			"  `security find-identity -v -p codesigning` lists the identities you have.")
		return nil
	}
	if !have("codesign") {
		p.note("cannot sign from %s: codesign is part of macOS. The bundle is complete "+
			"and unsigned; sign it on a Mac with\n"+
			"    codesign --force --options runtime --timestamp --entitlements %s --sign %q %s",
			runtime.GOOS, filepath.Base(entitlements), p.o.identity, filepath.Base(app))
		return nil
	}
	p.step("signing as %s", p.o.identity)
	// --options runtime (the hardened runtime) is required for notarisation,
	// so it is on from the start rather than discovered at submission.
	//
	// Deliberately not --deep: Apple documents it as deprecated and wrong for
	// anything but a quick test, and this bundle has one Mach-O in it anyway.
	// --timestamp gets a trusted timestamp, without which the signature stops
	// validating when the certificate expires rather than when it is revoked.
	args := []string{"--force", "--options", "runtime", "--timestamp"}
	if entitlements != "" {
		args = append(args, "--entitlements", entitlements)
	}
	args = append(args, "--sign", p.o.identity, app)
	if err := run("", nil, "codesign", args...); err != nil {
		return fmt.Errorf("codesign: %w", err)
	}
	if err := run("", nil, "codesign", "--verify", "--strict", "--verbose=2", app); err != nil {
		return fmt.Errorf("codesign --verify: %w", err)
	}
	return nil
}

// buildDMG wraps the bundle in a compressed disk image, the conventional way
// to hand a Mac app to someone: mount, drag to Applications, eject.
//
// hdiutil is the one external tool with no in-process alternative here. A
// UDZO image is an HFS+ filesystem in a compressed container, and writing one
// would mean implementing a filesystem; hdiutil ships with macOS, so it is
// present wherever a .dmg can be tested at all. Packaging from Linux or
// Windows skips the image and says why — the .app itself is complete and can
// be zipped.
func (p *packer) buildDMG(app string) (string, error) {
	if !have("hdiutil") {
		p.note("no .dmg: hdiutil is a macOS tool and this is %s. The %s is complete; "+
			"produce the image on a Mac with\n    hdiutil create -volname %q -srcfolder <dir> -ov -format UDZO %s.dmg",
			runtime.GOOS, filepath.Base(app), p.o.name, p.artifactName())
		return "", nil
	}
	// hdiutil images the contents of a directory, so the drag-to-install
	// window is just a directory holding the app and a symlink to
	// /Applications. No custom background or icon layout: those need a
	// scripted Finder window, which cannot be done headlessly or in CI.
	stage := filepath.Join(p.outDir(), ".dmg")
	if err := os.RemoveAll(stage); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stage, 0o755); err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	if err := copyTree(app, filepath.Join(stage, filepath.Base(app))); err != nil {
		return "", err
	}
	if err := os.Symlink("/Applications", filepath.Join(stage, "Applications")); err != nil {
		return "", err
	}
	dmg := filepath.Join(p.outDir(), p.artifactName()+".dmg")
	p.step("building %s", filepath.Base(dmg))
	if err := run("", nil, "hdiutil", "create",
		"-volname", p.o.name, "-srcfolder", stage, "-ov", "-format", "UDZO", "-quiet", dmg); err != nil {
		return "", fmt.Errorf("hdiutil create: %w", err)
	}
	return dmg, nil
}

// notarizeMac submits the signed artifact to Apple's notary service and
// staples the ticket.
//
// Three preconditions, each reported rather than enforced. Notarisation needs
// a signature (an unsigned bundle is rejected by the service, not by us),
// credentials, and notarytool — which means xcrun, which means a Mac. The
// wording of each skip is the documentation: this is the step nobody has ever
// done before the first time, and the message is where they find out it exists.
func (p *packer) notarizeMac(app, dmg string) error {
	if p.o.identity == "" {
		p.note("not notarised: notarisation needs a signed bundle, and this one is unsigned.")
		return nil
	}
	profile := p.o.notary
	appleID, pass, team := os.Getenv("APPLE_ID"), os.Getenv("APPLE_APP_PASSWORD"), os.Getenv("APPLE_TEAM_ID")
	haveCreds := profile != "" || (appleID != "" && pass != "" && team != "")
	if !haveCreds {
		p.note("not notarised: no credentials. Gatekeeper on another Mac shows a\n" +
			"  \"cannot be opened because the developer cannot be verified\" dialog until a\n" +
			"  notarised build is stapled. Store them once:\n" +
			"    xcrun notarytool store-credentials NOTARY --apple-id you@example.com \\\n" +
			"        --team-id TEAMID --password <app-specific-password>\n" +
			"  then re-run with -notary-profile NOTARY (or set $NOTARY_PROFILE).")
		return nil
	}
	if !have("xcrun") {
		p.note("not notarised: notarytool is reached through xcrun, which is macOS only "+
			"(this is %s).", runtime.GOOS)
		return nil
	}

	// The service takes a container, never a bare bundle. The disk image is
	// the one to submit when there is one, because the ticket then covers the
	// thing the user downloads; otherwise a zip of the bundle is submitted and
	// the ticket is stapled to the bundle itself.
	submit := dmg
	cleanup := func() {}
	if submit == "" {
		zipPath := filepath.Join(p.outDir(), p.artifactName()+".notarize.zip")
		if err := p.zipBundle(app, zipPath); err != nil {
			return err
		}
		submit = zipPath
		cleanup = func() { os.Remove(zipPath) }
	}
	defer cleanup()

	args := []string{"notarytool", "submit", submit, "--wait"}
	if profile != "" {
		args = append(args, "--keychain-profile", profile)
	} else {
		args = append(args, "--apple-id", appleID, "--password", pass, "--team-id", team)
	}
	p.step("notarising %s (this waits on Apple)", filepath.Base(submit))
	if err := run("", nil, "xcrun", args...); err != nil {
		return fmt.Errorf("notarytool submit: %w", err)
	}
	// The ticket is stapled so the app validates without a network round trip
	// on first launch. A zip cannot hold one, so the bundle is stapled in that
	// case and the zip is re-made by whoever distributes it.
	for _, target := range []string{app, dmg} {
		if target == "" {
			continue
		}
		p.step("stapling %s", filepath.Base(target))
		if err := run("", nil, "xcrun", "stapler", "staple", target); err != nil {
			return fmt.Errorf("stapler staple %s: %w", filepath.Base(target), err)
		}
	}
	return nil
}

// zipBundle compresses a .app for submission, preferring ditto.
//
// ditto is the documented way to zip a bundle for the notary service because
// it preserves the resource forks and extended attributes an Info.plist-era
// bundle could carry. Nothing this packer writes has any, so the stdlib zip is
// a correct fallback — and the fallback is what lets the step work at all from
// a non-Mac.
func (p *packer) zipBundle(app, dst string) error {
	if have("ditto") {
		if err := run("", nil, "ditto", "-c", "-k", "--keepParent", app, dst); err != nil {
			return fmt.Errorf("ditto: %w", err)
		}
		return nil
	}
	return writeZip(dst, app)
}

// copyTree copies a directory recursively, preserving the executable bit.
func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case fi.IsDir():
			return os.MkdirAll(target, 0o755)
		case fi.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		default:
			if err := copyFile(p, target); err != nil {
				return err
			}
			return os.Chmod(target, fi.Mode().Perm())
		}
	})
}

// --- property lists ---------------------------------------------------------

// A property list writer and reader, for the subset this CLI produces.
//
// Written by hand rather than by shelling out to plutil, which exists only on
// macOS — and the Info.plist of a .app has to be writable from the Linux box
// that cross-builds it. Read back as well as written because the alternative
// is discovering an unescaped app name as a signing failure, or worse as an
// app that will not launch on a user's machine.

// plistKV is one key and its value, in file order. A map would be simpler and
// would also shuffle the file on every build, which makes a diff of two
// bundles useless.
type plistKV struct {
	Key string
	Val any
}

// plistObj is an ordered dictionary. Values may be string, bool, int, []string
// or a nested plistObj.
type plistObj []plistKV

func encodePlist(d plistObj) []byte {
	var b strings.Builder
	b.WriteString(xml.Header)
	b.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" " +
		"\"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	b.WriteString("<plist version=\"1.0\">\n")
	encodePlistDict(&b, d, 0)
	b.WriteString("</plist>\n")
	return []byte(b.String())
}

func encodePlistDict(b *strings.Builder, d plistObj, depth int) {
	ind := strings.Repeat("\t", depth)
	b.WriteString(ind + "<dict>\n")
	for _, kv := range d {
		b.WriteString(ind + "\t<key>")
		plistEscape(b, kv.Key)
		b.WriteString("</key>\n")
		encodePlistValue(b, kv.Val, depth+1)
	}
	b.WriteString(ind + "</dict>\n")
}

func encodePlistValue(b *strings.Builder, v any, depth int) {
	ind := strings.Repeat("\t", depth)
	switch v := v.(type) {
	case string:
		b.WriteString(ind + "<string>")
		plistEscape(b, v)
		b.WriteString("</string>\n")
	case bool:
		if v {
			b.WriteString(ind + "<true/>\n")
		} else {
			b.WriteString(ind + "<false/>\n")
		}
	case int:
		b.WriteString(ind + "<integer>" + strconv.Itoa(v) + "</integer>\n")
	case []string:
		b.WriteString(ind + "<array>\n")
		for _, s := range v {
			encodePlistValue(b, s, depth+1)
		}
		b.WriteString(ind + "</array>\n")
	case plistObj:
		encodePlistDict(b, v, depth)
	default:
		// Unreachable for the values infoPlist builds; panicking here would
		// turn a typo in a future key into a crash, so it is written as a
		// comment the reader of the plist can see.
		b.WriteString(ind + "<string><!-- unsupported value --></string>\n")
	}
}

func plistEscape(b *strings.Builder, s string) {
	// xml.EscapeText, not a hand-rolled replacer: it also handles the control
	// characters and invalid runes that would make the file unparseable.
	_ = xml.EscapeText(io.Writer(b), []byte(s))
}

// parsePlist decodes a plist into nested maps, slices and scalars.
//
// Only the element set this writer emits, plus <real> and <data>, which are
// read as a float and as raw base64 text. Enough to verify what was written
// and to let a test assert on a bundle's actual metadata rather than on the
// string the generator happened to build.
func parsePlist(data []byte) (map[string]any, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return nil, fmt.Errorf("plist: no <plist> element")
		}
		if err != nil {
			return nil, fmt.Errorf("plist: %w", err)
		}
		se, ok := tok.(xml.StartElement)
		if !ok {
			continue
		}
		switch se.Name.Local {
		case "plist":
			continue
		case "dict":
			return parsePlistDict(dec)
		default:
			return nil, fmt.Errorf("plist: root value is <%s>, want <dict>", se.Name.Local)
		}
	}
}

func parsePlistDict(dec *xml.Decoder) (map[string]any, error) {
	out := map[string]any{}
	for {
		se, ok, err := nextStartElement(dec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return out, nil // </dict>
		}
		if se.Name.Local != "key" {
			return nil, fmt.Errorf("plist: <%s> where a <key> belongs", se.Name.Local)
		}
		var key string
		if err := dec.DecodeElement(&key, &se); err != nil {
			return nil, err
		}
		vse, ok, err := nextStartElement(dec)
		if err != nil {
			return nil, err
		}
		if !ok {
			return nil, fmt.Errorf("plist: key %q has no value", key)
		}
		v, err := parsePlistValue(dec, vse)
		if err != nil {
			return nil, fmt.Errorf("plist: key %q: %w", key, err)
		}
		// A duplicate key is a real bug worth catching: the readers disagree
		// about which one wins, so the app behaves differently depending on
		// which tool looked at it.
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("plist: duplicate key %q", key)
		}
		out[key] = v
	}
}

func parsePlistValue(dec *xml.Decoder, se xml.StartElement) (any, error) {
	switch se.Name.Local {
	case "string", "data", "date":
		var s string
		err := dec.DecodeElement(&s, &se)
		return s, err
	case "integer":
		var s string
		if err := dec.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return strconv.Atoi(strings.TrimSpace(s))
	case "real":
		var s string
		if err := dec.DecodeElement(&s, &se); err != nil {
			return nil, err
		}
		return strconv.ParseFloat(strings.TrimSpace(s), 64)
	case "true", "false":
		if err := dec.Skip(); err != nil {
			return nil, err
		}
		return se.Name.Local == "true", nil
	case "dict":
		return parsePlistDict(dec)
	case "array":
		var out []any
		for {
			ise, ok, err := nextStartElement(dec)
			if err != nil {
				return nil, err
			}
			if !ok {
				return out, nil // </array>
			}
			v, err := parsePlistValue(dec, ise)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		}
	default:
		return nil, fmt.Errorf("plist: unsupported value <%s>", se.Name.Local)
	}
}

// nextStartElement returns the next start element inside the current one, or
// ok=false at its closing tag. Whitespace, comments and the DOCTYPE directive
// are skipped.
func nextStartElement(dec *xml.Decoder) (xml.StartElement, bool, error) {
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			return xml.StartElement{}, false, fmt.Errorf("plist: unexpected end of document")
		}
		if err != nil {
			return xml.StartElement{}, false, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, true, nil
		case xml.EndElement:
			return xml.StartElement{}, false, nil
		}
	}
}
