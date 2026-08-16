package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// alignFlag is the linker flag that gives libgojni.so 16 KB LOAD segments.
// Devices from the Pixel 10 on boot with 16 KB pages and show Android's
// PageSizeMismatch dialog for apps whose native libs use the toolchain's 4 KB
// default.
const alignFlag = "max-page-size=16384"

// TestAndroidScriptsAlign holds the packaging scripts to the same alignment
// flag the CLI passes.
//
// This is guarding a divergence that already happened: gomobileBind added the
// flag, examples/tally/package/android.sh called `gomobile bind` directly and
// did not, and an APK built the documented way hit the dialog on launch while
// one built through `gophics build` was fine. Nothing links the two, so the
// only thing keeping them together is a test that reads both.
func TestAndroidScriptsAlign(t *testing.T) {
	scripts, err := filepath.Glob("../../examples/*/package/android.sh")
	if err != nil {
		t.Fatal(err)
	}
	if len(scripts) == 0 {
		t.Skip("no packaging scripts to check")
	}
	for _, s := range scripts {
		b, err := os.ReadFile(s)
		if err != nil {
			t.Fatal(err)
		}
		body := string(b)
		if !strings.Contains(body, "gomobile bind") {
			continue // shells out to the CLI, which passes the flag itself
		}
		if !strings.Contains(body, alignFlag) {
			t.Errorf("%s calls `gomobile bind` without -ldflags %s;\n"+
				"APKs built this way show the 16 KB compatibility dialog on a Pixel 10",
				s, alignFlag)
		}
	}
}

// TestJNISymbolsMatchPackage checks that each example app's JNI exports name
// the package that app actually declares.
//
// A JNI symbol encodes the Java package: NativeSurface.acquire in package
// com.gophics.tally must be exported as Java_com_gophics_tally_NativeSurface_
// acquire. Nothing links the C file to build.gradle, so copying an app and
// renaming its package leaves the exports pointing at the original — which is
// exactly what happened to tally: it kept hn's symbols and died on launch with
// UnsatisfiedLinkError the moment a SurfaceView appeared. The failure needs a
// device to see, so it is worth catching here.
//
// The CLI template gets this right by construction ({{.JNIPkg}}); this covers
// the materialised copies checked into examples/.
func TestJNISymbolsMatchPackage(t *testing.T) {
	cfiles, err := filepath.Glob("../../examples/*/android/app/src/main/cpp/*.c")
	if err != nil {
		t.Fatal(err)
	}
	if len(cfiles) == 0 {
		t.Skip("no android example apps to check")
	}
	for _, cf := range cfiles {
		// <app>/android/app/src/main/cpp/x.c → up four to <app>/android/app
		root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(cf))))
		gradle, err := os.ReadFile(filepath.Join(root, "build.gradle"))
		if err != nil {
			t.Fatal(err)
		}
		pkg := namespaceOf(string(gradle))
		if pkg == "" {
			t.Errorf("%s: no namespace in build.gradle", root)
			continue
		}
		want := "Java_" + strings.ReplaceAll(pkg, ".", "_") + "_"
		b, err := os.ReadFile(cf)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(b), "\n") {
			i := strings.Index(line, "Java_")
			if i < 0 {
				continue
			}
			if !strings.HasPrefix(line[i:], want) {
				t.Errorf("%s exports %q but the app's package is %q;\n"+
					"the symbol must start with %q or the JNI lookup fails at runtime",
					cf, strings.TrimSuffix(line[i:], "(JNIEnv *env, jobject thiz, jobject surface) {"), pkg, want)
			}
		}
	}
}

// namespaceOf pulls the `namespace '<pkg>'` value out of a build.gradle.
func namespaceOf(gradle string) string {
	for line := range strings.SplitSeq(gradle, "\n") {
		f := strings.TrimSpace(line)
		if !strings.HasPrefix(f, "namespace ") {
			continue
		}
		return strings.Trim(strings.TrimSpace(strings.TrimPrefix(f, "namespace ")), "'\"")
	}
	return ""
}

// TestCLIPassesAlignFlag pins the other half of the pair, so deleting the flag
// from gomobileBind fails here rather than silently on a device.
func TestCLIPassesAlignFlag(t *testing.T) {
	b, err := os.ReadFile("mobile.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), alignFlag) {
		t.Errorf("gomobileBind no longer passes %s", alignFlag)
	}
}
