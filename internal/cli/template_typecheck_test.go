package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The scaffolded iOS host has to compile, and nothing was checking it.
//
// Two real defects reached the template unnoticed, both of the kind only a
// compiler finds: a `density.toDouble()` handed to a Float parameter on the
// Android side, and an iOS-17-only accessibility API used in a file that
// targets iOS 15. Neither is visible by reading, both break every app
// `gophics create` produces, and the existing native/ typecheck covers only the
// reference hosts — not the templates, which is what people actually get.
//
// This renders the template the way create does and runs swiftc over it against
// a real gomobile bind. Expensive and macOS-only, so it needs the same opt-in
// as the reference-host check: GOPHICS_IOS_HOSTS=1.
func TestScaffoldedIOSHostTypechecks(t *testing.T) {
	if os.Getenv("GOPHICS_IOS_HOSTS") == "" {
		t.Skip("set GOPHICS_IOS_HOSTS=1 to typecheck the scaffolded host")
	}
	for _, tool := range []string{"gomobile", "swiftc", "xcrun"} {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not installed", tool)
		}
	}

	dir := t.TempDir()
	data := map[string]string{
		"Name":           "probe",
		"MobilePkg":      "probemobile",
		"Framework":      "Probemobile",
		"BundleID":       "com.example.probe",
		"BundleIDPrefix": "com.example",
		"AndroidPkg":     "com.example.probe",
		"JNIPkg":         "com_example_probe",
		"ProjectName":    "Probe",
	}
	if err := scaffoldMobile(dir, "com.example.probe", data, true, false); err != nil {
		t.Fatal(err)
	}
	swifts, err := filepath.Glob(filepath.Join(dir, "ios", "App", "*.swift"))
	if err != nil || len(swifts) == 0 {
		t.Fatalf("no scaffolded Swift files: %v", err)
	}

	// The framework the host imports is the app's own bind package, which does
	// not exist here. Bind shell/mobile under the name the host expects and
	// stub the one package-level function that comes from the app's side, so
	// what is being checked is the host's own code.
	fw := filepath.Join(dir, "Mobile.xcframework")
	bind := exec.Command("gomobile", "bind", "-target=ios", "-o", fw,
		"github.com/doug/gophics/shell/mobile")
	if out, err := bind.CombinedOutput(); err != nil {
		t.Fatalf("gomobile bind: %v\n%s", err, out)
	}

	// All of them together, because they reference each other: App.swift
	// constructs the GophicsPlatform that GophicsPlatform.swift defines.
	var body strings.Builder
	for _, f := range swifts {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		one := strings.ReplaceAll(string(src), "import Probemobile", "import Mobile")
		one = strings.ReplaceAll(one, "@main\n", "")
		body.WriteString(one)
		body.WriteString("\n")
	}
	body.WriteString("\nfunc ProbemobileStart(_ e: NSErrorPointer) -> MobileBridge? { nil }\n")
	swift := filepath.Join(dir, "App_typecheck.swift")
	if err := os.WriteFile(swift, []byte(body.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	sdk, err := exec.Command("xcrun", "--sdk", "iphoneos", "--show-sdk-path").Output()
	if err != nil {
		t.Fatalf("xcrun: %v", err)
	}
	out, err := exec.Command("swiftc", "-typecheck",
		"-sdk", strings.TrimSpace(string(sdk)),
		// The same floor project.yml sets. Building against a newer SDK hides
		// exactly the availability bug this caught.
		"-target", "arm64-apple-ios15.0",
		"-F", filepath.Join(fw, "ios-arm64"), swift).CombinedOutput()
	if err != nil {
		t.Errorf("the scaffolded iOS host does not typecheck:\n%s", out)
	}
}
