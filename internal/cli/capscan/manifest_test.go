package capscan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/manifest"
)

const manifestWithMarkers = `<?xml version="1.0" encoding="utf-8"?>
<manifest xmlns:android="http://schemas.android.com/apk/res/android">
    <!-- gophics:permissions -->
    <!-- /gophics:permissions -->
    <application android:label="Demo">
        <activity android:name=".MainActivity" android:exported="true">
            <intent-filter>
                <action android:name="android.intent.action.MAIN" />
            </intent-filter>
        </activity>
    </application>
</manifest>
`

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// The generated span carries the permissions and nothing else in the file
// moves. A manifest also holds things only a human decides — the launcher
// intent filter, the theme, deep-link schemes — and regenerating the whole file
// would either lose them or mean modelling all of them.
func TestAndroidManifestWritesOnlyTheMarkedSpan(t *testing.T) {
	path := writeTemp(t, "AndroidManifest.xml", manifestWithMarkers)
	perm := manifest.Merge([]string{"Camera", "Microphone"}, manifest.Baseline)

	changed, err := AndroidManifest(path, perm, false)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("writing permissions into an empty span reported no change")
	}

	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(got)

	for _, want := range []string{
		"android.permission.CAMERA",
		"android.permission.RECORD_AUDIO",
		"android.permission.INTERNET",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the manifest does not declare %s", want)
		}
	}
	// Everything outside the span survives.
	for _, keep := range []string{
		`android:label="Demo"`,
		"android.intent.action.MAIN",
		`<activity android:name=".MainActivity"`,
	} {
		if !strings.Contains(out, keep) {
			t.Errorf("rewriting the span lost %q from the rest of the file", keep)
		}
	}
}

// Rewriting must be idempotent, or every build dirties the file and the -check
// gate fires on a tree nobody changed.
func TestAndroidManifestIsIdempotent(t *testing.T) {
	path := writeTemp(t, "AndroidManifest.xml", manifestWithMarkers)
	perm := manifest.Merge([]string{"Camera"}, manifest.Baseline)

	if _, err := AndroidManifest(path, perm, false); err != nil {
		t.Fatal(err)
	}
	first, _ := os.ReadFile(path)

	changed, err := AndroidManifest(path, perm, false)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Error("a second write with the same capabilities reported a change")
	}
	second, _ := os.ReadFile(path)
	if string(first) != string(second) {
		t.Error("writing twice produced different files")
	}
}

// -check is what a gate calls: it must report drift without touching the file.
func TestAndroidManifestCheckDoesNotWrite(t *testing.T) {
	path := writeTemp(t, "AndroidManifest.xml", manifestWithMarkers)
	perm := manifest.Merge([]string{"Camera"}, manifest.Baseline)

	before, _ := os.ReadFile(path)
	changed, err := AndroidManifest(path, perm, true)
	if err == nil {
		t.Error("a stale manifest passed -check")
	}
	if !changed {
		t.Error("-check did not report the drift")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Error("-check modified the file")
	}
}

// A manifest with no markers is a setup error, and saying so beats writing
// permissions somewhere arbitrary or silently declaring none.
func TestAndroidManifestWithoutMarkersIsAnError(t *testing.T) {
	path := writeTemp(t, "AndroidManifest.xml",
		"<manifest><application/></manifest>\n")
	_, err := AndroidManifest(path, manifest.Baseline, false)
	if err == nil {
		t.Fatal("a manifest with no markers was accepted")
	}
	if !strings.Contains(err.Error(), "markers") {
		t.Errorf("the error does not say what is wrong: %v", err)
	}
}

// iOS usage descriptions are the app's prose. The build must name the missing
// key and the capability that needs it, so the fix is obvious.
func TestIOSMissingUsageDescriptionIsReported(t *testing.T) {
	plist := writeTemp(t, "Info.plist", `<?xml version="1.0"?>
<plist version="1.0"><dict>
	<key>CFBundleName</key><string>Demo</string>
</dict></plist>
`)
	missing, err := CheckIOSUsageDescriptions(plist, []string{"Camera", "Microphone"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 2 {
		t.Fatalf("reported %d missing keys, want 2: %+v", len(missing), missing)
	}
	byKey := map[string][]string{}
	for _, m := range missing {
		byKey[m.Key] = m.Capabilities
	}
	if caps := byKey["NSCameraUsageDescription"]; len(caps) != 1 || caps[0] != "Camera" {
		t.Errorf("NSCameraUsageDescription blamed on %v, want [Camera]", caps)
	}
	if _, ok := byKey["NSMicrophoneUsageDescription"]; !ok {
		t.Error("NSMicrophoneUsageDescription was not reported")
	}
}

// Present-but-empty is the case that matters: it satisfies a grep, passes a
// build that only looks for the key, and is rejected at App Review.
func TestIOSEmptyUsageDescriptionCountsAsMissing(t *testing.T) {
	plist := writeTemp(t, "Info.plist", `<?xml version="1.0"?>
<plist version="1.0"><dict>
	<key>NSCameraUsageDescription</key><string>   </string>
</dict></plist>
`)
	missing, err := CheckIOSUsageDescriptions(plist, []string{"Camera"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 1 {
		t.Fatalf("an empty usage description was accepted: %+v", missing)
	}
}

// And real prose satisfies it.
func TestIOSPresentUsageDescriptionPasses(t *testing.T) {
	plist := writeTemp(t, "Info.plist", `<?xml version="1.0"?>
<plist version="1.0"><dict>
	<key>NSCameraUsageDescription</key><string>To scan a receipt.</string>
</dict></plist>
`)
	missing, err := CheckIOSUsageDescriptions(plist, []string{"Camera"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Errorf("a written usage description was still reported missing: %+v", missing)
	}
}

// A capability with no iOS key must not invent one.
func TestIOSCapabilityWithoutKeyNeedsNothing(t *testing.T) {
	plist := writeTemp(t, "Info.plist", `<plist><dict/></plist>`)
	missing, err := CheckIOSUsageDescriptions(plist, []string{"Haptic", "Socket"})
	if err != nil {
		t.Fatal(err)
	}
	if len(missing) != 0 {
		t.Errorf("capabilities with no iOS key produced requirements: %+v", missing)
	}
}

// The entitlements file must be valid plist and carry the sandbox key, or
// codesign rejects it.
func TestMacEntitlementsShape(t *testing.T) {
	perm := manifest.Merge([]string{"Camera", "Microphone"}, manifest.Baseline)
	out := MacEntitlements(perm)

	for _, want := range []string{
		"com.apple.security.app-sandbox",
		"com.apple.security.device.camera",
		"com.apple.security.device.audio-input",
		"com.apple.security.network.client",
		"</plist>",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("entitlements missing %q", want)
		}
	}
}
