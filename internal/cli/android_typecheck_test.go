package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The scaffolded Android host has to compile too, and only a compiler can say
// so: the density.toDouble() defect the Swift check's comment cites was in
// the Kotlin, and the sixty lines of haptics, lifecycle and forward-delete
// handling added since were covered by nothing but a substring check for the
// bridge calls. This renders the template the way create does, binds
// shell/mobile as the .aar the host links, stubs the app's own start function
// beside it, and runs gradle's Kotlin compilation for the debug variant — not
// assembleDebug, which would also build the JNI shim and package an APK.
//
// Same opt-in shape as the iOS check: GOPHICS_ANDROID_HOSTS=1, plus an SDK and
// a JDK where `gophics run -p android` would look for them. The first run
// also needs the network for gradle's own downloads.
func TestScaffoldedAndroidHostCompiles(t *testing.T) {
	if os.Getenv("GOPHICS_ANDROID_HOSTS") == "" {
		t.Skip("set GOPHICS_ANDROID_HOSTS=1 to compile the scaffolded host")
	}
	if _, err := exec.LookPath("gomobile"); err != nil {
		t.Skip("gomobile not installed")
	}
	sdk := androidHome()
	if sdk == "" {
		t.Skip("no Android SDK (set ANDROID_HOME)")
	}
	jdk, err := findJDK()
	if err != nil {
		t.Skipf("no JDK: %v", err)
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
	if err := scaffoldMobile(dir, "com.example.probe", data, false, true); err != nil {
		t.Fatal(err)
	}
	host := filepath.Join(dir, "android")

	// The .aar the host links is the app's own bind package, which does not
	// exist here. Bind shell/mobile — the classes the host actually uses are
	// its Bridge and host interfaces — and stub the one package-level entry
	// point that comes from the app's side, so what is compiled is the host.
	aar := filepath.Join(host, "app", "libs", "mobile.aar")
	if err := os.MkdirAll(filepath.Dir(aar), 0o755); err != nil {
		t.Fatal(err)
	}
	bind := exec.Command("gomobile", "bind", "-target", "android", "-androidapi", "24",
		"-o", aar, "github.com/doug/gophics/shell/mobile")
	bind.Env = append(os.Environ(), "ANDROID_HOME="+sdk)
	if out, err := bind.CombinedOutput(); err != nil {
		t.Fatalf("gomobile bind: %v\n%s", err, out)
	}
	stub := filepath.Join(host, "app", "src", "main", "java", "probemobile", "Probemobile.kt")
	if err := os.MkdirAll(filepath.Dir(stub), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stub, []byte("package probemobile\n\n"+
		"object Probemobile {\n    fun start(): mobile.Bridge = throw UnsupportedOperationException()\n}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	gradle := exec.Command(filepath.Join(host, "gradlew"), "--no-daemon", "-q", ":app:compileDebugKotlin")
	gradle.Dir = host
	gradle.Env = append(os.Environ(), "ANDROID_HOME="+sdk, "JAVA_HOME="+jdk)
	if out, err := gradle.CombinedOutput(); err != nil {
		t.Errorf("the scaffolded Android host does not compile:\n%s", out)
	}
}
