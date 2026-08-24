package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestFindInfoPlist guards the failure that made checkIOSPermissions useless:
// it looked for App/Info.plist, no project in this tree is laid out that way,
// so it found nothing, returned nil, and never ran. A guard that silently
// passes is worse than no guard, because it gets counted on.
func TestFindInfoPlist(t *testing.T) {
	host := t.TempDir()
	// The layout Xcode actually produces: a group named after the app.
	app := filepath.Join(host, "GophicsMirror")
	if err := os.MkdirAll(app, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(app, "Info.plist")
	if err := os.WriteFile(want, []byte("<plist/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := findInfoPlist(host)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("findInfoPlist = %q, want %q", got, want)
	}
}

// TestFindInfoPlistSkipsBuildProducts: a built .app and a bound .xcframework
// each carry an Info.plist, and checking those would pass while the file a
// person edits stays wrong.
func TestFindInfoPlistSkipsBuildProducts(t *testing.T) {
	host := t.TempDir()
	for _, d := range []string{
		"Mirrormobile.xcframework/ios-arm64/Mirrormobile.framework",
		"build/Build/Products/Debug-iphoneos/GophicsMirror.app",
		"GophicsMirror.xcodeproj",
	} {
		p := filepath.Join(host, d)
		if err := os.MkdirAll(p, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(p, "Info.plist"), []byte("<plist/>"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := findInfoPlist(host)
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("findInfoPlist found %q, which is a build product, not the source plist", got)
	}
}

// TestFindInfoPlistReportsAbsence rather than erroring: a host project may
// keep its plist somewhere this cannot guess, and refusing to build over that
// would be worse than not checking.
func TestFindInfoPlistReportsAbsence(t *testing.T) {
	got, err := findInfoPlist(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("findInfoPlist = %q for an empty host, want \"\"", got)
	}
}
