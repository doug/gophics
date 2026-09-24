package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// androidNDK and androidCMake are documented as "kept in sync with the
// scaffolded app/build.gradle" — by hand, with nothing checking. If they drift,
// ensureAndroidSDK installs one NDK and gradle demands another, and the
// failure is a gradle log on a machine that had everything the CLI said it
// needed. Same shape as TestAndroidScriptsAlign: two copies of a value with no
// link between them except a test that reads both.
func TestAndroidPinsMatchTheScaffold(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("templates", "mobile", "android", "app", "build.gradle.tmpl"))
	if err != nil {
		t.Fatal(err)
	}
	gradle := string(b)
	if want := "ndkVersion '" + androidNDK + "'"; !strings.Contains(gradle, want) {
		t.Errorf("build.gradle.tmpl does not pin %s; the CLI installs NDK %s and gradle would want a different one",
			want, androidNDK)
	}
	if want := "version '" + androidCMake + "'"; !strings.Contains(gradle, want) {
		t.Errorf("build.gradle.tmpl does not pin CMake %s, which is what the CLI installs", androidCMake)
	}
}
