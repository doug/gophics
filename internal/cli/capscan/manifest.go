package capscan

import (
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"

	"github.com/doug/gophics/internal/manifest"
)

// Writing what a scan found into a host project.
//
// Two different jobs, split by whether the answer can be derived.
//
// Android permissions and macOS entitlements are fully determined by the
// capability set, so they are generated: the build rewrites a marked span and a
// -check mode fails when the checked-in file has drifted. Same contract as
// docs/build-embeds.sh and scripts/tools/planfacts.py, for the same reason.
//
// iOS usage descriptions are not. NSCameraUsageDescription carries prose that
// only the app can write — "so you can scan a receipt" — and Apple rejects a
// placeholder at review. So those are verified rather than generated: the build
// fails naming the key and the capability that needs it, and leaves the writing
// to a human.

const (
	androidBegin = "<!-- gophics:permissions -->"
	androidEnd   = "<!-- /gophics:permissions -->"
)

var androidSpan = regexp.MustCompile(
	`(?s)([ \t]*)` + regexp.QuoteMeta(androidBegin) + `.*?` + regexp.QuoteMeta(androidEnd),
)

// AndroidManifest rewrites the marked permission span in an AndroidManifest.xml.
//
// It reports whether the file changed. With check set, nothing is written and a
// changed file is an error instead — that is what a build gate calls.
func AndroidManifest(path string, perm manifest.Permission, check bool) (bool, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return false, fmt.Errorf("read %s: %w", path, err)
	}
	text := string(src)

	loc := androidSpan.FindStringSubmatchIndex(text)
	if loc == nil {
		return false, fmt.Errorf(
			"%s has no %s … %s markers, so there is nowhere to write the "+
				"permissions; add the pair inside <manifest>", path, androidBegin, androidEnd)
	}
	indent := text[loc[2]:loc[3]]

	var b strings.Builder
	b.WriteString(indent + androidBegin + "\n")
	b.WriteString(indent + "<!-- Generated from the capabilities this app uses. Edit the code, not this. -->\n")
	for _, p := range perm.Android {
		b.WriteString(indent + `<uses-permission android:name="` + p + `"`)
		if max, ok := perm.AndroidMaxSDK[p]; ok {
			fmt.Fprintf(&b, ` android:maxSdkVersion="%d"`, max)
		}
		b.WriteString(" />\n")
	}
	b.WriteString(indent + androidEnd)

	updated := text[:loc[0]] + b.String() + text[loc[1]:]
	if updated == text {
		return false, nil
	}
	if check {
		return true, fmt.Errorf(
			"%s is out of date with the capabilities this app uses — run the "+
				"build without -check to rewrite it", path)
	}
	return true, os.WriteFile(path, []byte(updated), 0o644)
}

// MissingIOSKey is one usage-description key an app must write for itself.
type MissingIOSKey struct {
	Key          string
	Capabilities []string // which capabilities need it, sorted
}

// CheckIOSUsageDescriptions reports the usage-description keys that a build
// needs and the Info.plist does not supply with non-empty text.
//
// Only reports; never writes. A generated usage description would be a
// placeholder, App Review rejects those, and shipping one converts a build
// error into a submission rejection weeks later.
func CheckIOSUsageDescriptions(plistPath string, capabilities []string) ([]MissingIOSKey, error) {
	src, err := os.ReadFile(plistPath)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", plistPath, err)
	}
	text := string(src)

	// Which capability asked for which key, so the error can say why.
	need := map[string][]string{}
	for _, c := range capabilities {
		p, ok := manifest.For(c)
		if !ok {
			continue
		}
		for _, k := range p.IOSKeys {
			need[k] = append(need[k], c)
		}
	}

	var missing []MissingIOSKey
	for key, caps := range need {
		if hasNonEmptyPlistString(text, key) {
			continue
		}
		sort.Strings(caps)
		missing = append(missing, MissingIOSKey{Key: key, Capabilities: caps})
	}
	sort.Slice(missing, func(i, j int) bool { return missing[i].Key < missing[j].Key })
	return missing, nil
}

// hasNonEmptyPlistString reports whether the plist sets key to a non-empty
// string.
//
// Present-but-empty is the case worth catching: <string></string> satisfies a
// grep, passes a build that only checks for the key, and is rejected at review.
func hasNonEmptyPlistString(plist, key string) bool {
	re := regexp.MustCompile(
		`(?s)<key>\s*` + regexp.QuoteMeta(key) + `\s*</key>\s*<string>(.*?)</string>`)
	m := re.FindStringSubmatch(plist)
	return m != nil && strings.TrimSpace(m[1]) != ""
}

// MacEntitlements renders the entitlements a sandboxed macOS build needs.
//
// Returned rather than written: an entitlements file belongs to whatever signs
// the app, and there is no single conventional path to rewrite.
func MacEntitlements(perm manifest.Permission) string {
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>` + "\n")
	b.WriteString(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" ` +
		`"http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	b.WriteString(`<plist version="1.0">` + "\n<dict>\n")
	b.WriteString("\t<key>com.apple.security.app-sandbox</key>\n\t<true/>\n")
	for _, e := range perm.MacEntitlements {
		b.WriteString("\t<key>" + e + "</key>\n\t<true/>\n")
	}
	b.WriteString("</dict>\n</plist>\n")
	return b.String()
}

// Permissions folds a scan into the declaration a build needs, including the
// baseline every gophics app requires.
func Permissions(r Result) manifest.Permission {
	return manifest.Merge(r.Capabilities, manifest.Baseline)
}
