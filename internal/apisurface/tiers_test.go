package apisurface_test

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The tier table in docs/api-tiers.md states a number per tier, and a number in
// prose is a number that goes stale.
//
// It is the same failure the manifest exists to prevent, one level up: the
// audit that preceded this work quoted per-package counts that were wrong by
// the time anyone read them. So the doc's numbers are checked against the
// generated manifest, and every package in it must be assigned to exactly one
// tier — a new public package cannot appear without someone saying what it
// promises.
func TestTierCountsMatchTheManifest(t *testing.T) {
	tiers := map[string][]string{
		"1": {"app", "widget", "theme", "chart", "paint", "geom", "anim", "intl",
			"apptest", "sound", "sound/pitch", "sound/procedural", "sound/ogg",
			"sound/mp3", "sound/device"},
		"2": {"layout", "text", "input"},
		"3": {"shell", "shell/desktop", "shell/terminal", "shell/web"},
	}

	surface, err := os.ReadFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	perPkg := map[string]int{}
	for _, line := range strings.Split(string(surface), "\n") {
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// "shell/web.Run func" → "shell/web"
		name := line[:strings.Index(line, " ")]
		// The package is everything before the *first* dot: a path has no dots
		// in it, but a member does (paint.Color.R).
		perPkg[name[:strings.Index(name, ".")]]++
	}

	// Every package the manifest knows about has a tier.
	assigned := map[string]string{}
	for tier, pkgs := range tiers {
		for _, p := range pkgs {
			assigned[p] = tier
		}
	}
	for pkg := range perPkg {
		if _, ok := assigned[pkg]; !ok {
			t.Errorf("package %q is public and in no tier — docs/api-tiers.md has "+
				"to say what it promises before it ships", pkg)
		}
	}

	// The stated totals are the real ones.
	doc, err := os.ReadFile("../../docs/api-tiers.md")
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`\|\s+\*\*(\d) — \w+\*\*\s+\|[^|]*\|\s+([\d,]+)\s+\|`)
	found := 0
	for _, m := range row.FindAllStringSubmatch(string(doc), -1) {
		tier, stated := m[1], m[2]
		want := 0
		for _, p := range tiers[tier] {
			want += perPkg[p]
		}
		got, err := strconv.Atoi(strings.ReplaceAll(stated, ",", ""))
		if err != nil {
			t.Fatalf("tier %s: unreadable count %q", tier, stated)
		}
		if got != want {
			t.Errorf("docs/api-tiers.md says tier %s has %d names; the manifest "+
				"says %d", tier, got, want)
		}
		found++
	}
	if found != len(tiers) {
		t.Errorf("matched %d tier rows in the doc, want %d — the table's shape "+
			"changed and this check stopped reading it", found, len(tiers))
	}
}
