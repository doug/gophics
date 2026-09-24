package capscan

import (
	"strings"
	"testing"
)

// syncAndroidPermissions and checkIOSPermissions both scan with a mobile
// Target, and nothing exercised that: a Target whose GOOS was silently
// dropped would type-check the host build instead, report the host's
// capability set, and declare permissions for the wrong platform without
// anyone noticing. examples/hn/mobile is a real bind package in this module,
// so the scan runs against what `gophics run` would scan.
func TestScanWithMobileTargets(t *testing.T) {
	if testing.Short() {
		t.Skip("type-checks the hn bind package for two targets")
	}
	const pkg = "../../../examples/hn/mobile"
	android, err := Scan(pkg, ".", Target{GOOS: "android", GOARCH: "arm64"})
	if err != nil {
		t.Fatalf("android scan: %v", err)
	}
	if android.Packages == 0 || len(android.Capabilities) == 0 {
		t.Fatalf("android scan covered %d packages and found %v; a scan that finds nothing is indistinguishable from one that did not run",
			android.Packages, android.Capabilities)
	}
	// TextInput is reached from the core widget set, so any app that draws a
	// text field carries it; it is the capability least likely to leave hn.
	if !contains(android.Capabilities, "TextInput") {
		t.Errorf("android scan found %v, missing TextInput", android.Capabilities)
	}

	ios, err := Scan(pkg, ".", Target{GOOS: "ios", GOARCH: "arm64"})
	if err != nil {
		t.Fatalf("ios scan: %v", err)
	}
	// The same app reaches the same capabilities on both phones; the two
	// scans differing would mean one of them type-checked the wrong build.
	if a, b := strings.Join(android.Capabilities, ","), strings.Join(ios.Capabilities, ","); a != b {
		t.Errorf("android scan found [%s], ios scan found [%s]", a, b)
	}
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
