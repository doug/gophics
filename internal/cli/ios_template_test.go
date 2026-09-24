package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The iOS host's usage descriptions are in two files, and xcodegen only reads
// one of them.
//
// project.yml declares `info: path: App/Info.plist`, so `xcodegen generate` —
// which `gophics run` does on every build — rewrites the plist from the
// properties in project.yml. A usage-description key that exists only in
// Info.plist.tmpl survives exactly one build: the first `gophics create`
// probe had seven keys, one `xcodegen generate` later it had one, and every
// build after that failed the permission check against a file the next
// generate would wipe again. Both templates must carry the same set, so the
// check sees the same keys before and after xcodegen.
func TestIOSTemplatesAgreeOnUsageDescriptions(t *testing.T) {
	dir := filepath.Join("templates", "mobile", "ios")
	plist := usageKeys(t, filepath.Join(dir, "App", "Info.plist.tmpl"))
	project := usageKeys(t, filepath.Join(dir, "project.yml.tmpl"))
	if len(plist) == 0 {
		t.Fatal("Info.plist.tmpl declares no usage descriptions; the check has nothing to guard")
	}
	if strings.Join(plist, ",") != strings.Join(project, ",") {
		t.Errorf("usage-description keys differ:\n  Info.plist.tmpl: %v\n  project.yml.tmpl: %v\n"+
			"xcodegen regenerates the plist from project.yml, so a key in one and not the other is lost on the next build",
			plist, project)
	}
}

var usageKeyRe = regexp.MustCompile(`\bNS[A-Za-z]+UsageDescription\b`)

func usageKeys(t *testing.T, path string) []string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	var keys []string
	for _, k := range usageKeyRe.FindAllString(string(b), -1) {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	return keys
}
