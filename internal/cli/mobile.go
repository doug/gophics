package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// The pinned Android build bits the JNI surface shim needs; ensureAndroidSDK
// installs them if missing. Kept in sync with the scaffolded app/build.gradle
// (ndkVersion) and app CMakeLists.
const (
	androidNDK   = "26.1.10909125"
	androidCMake = "3.22.1"
)

// buildMobile wraps `gomobile bind` to produce the standalone native artifact
// (`gossamer build -p ios/android`): an .xcframework for iOS, an .aar for
// Android, under build/<platform>. `gossamer run` binds straight into the host
// project instead (see runMobile).
func buildMobile(o buildOpts) (string, error) {
	dir := outDir(o)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	switch o.platform.name {
	case "ios":
		out := filepath.Join(dir, "App.xcframework")
		return out, gomobileBind(o, "ios,iossimulator", out)
	default:
		out := filepath.Join(dir, "app.aar")
		return out, gomobileBind(o, "android", out)
	}
}

// gomobileBind runs `gomobile bind` for target, writing to out.
func gomobileBind(o buildOpts, target, out string) error {
	if !have("gomobile") {
		return fmt.Errorf("gomobile not found — install with:\n" +
			"  go install golang.org/x/mobile/cmd/gomobile@latest && gomobile init")
	}
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	args := []string{"bind", "-target", target}
	if o.platform.name == "android" {
		args = append(args, "-androidapi", "24")
	}
	args = append(args, "-o", out)
	if t := tagList(o.platform, o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	args = append(args, o.pkg)
	if err := run("", nil, "gomobile", args...); err != nil {
		return fmt.Errorf("gomobile bind: %w", err)
	}
	return nil
}

// runMobile is the `gossamer run` path for ios/android: bind the Go side into
// the host project, build it, and install + launch on a simulator/device — the
// whole loop, no per-project scripts. The host project is the sibling ios/ or
// android/ directory next to the bind package (override with -host).
func runMobile(o buildOpts) error {
	host, err := hostDirFor(o)
	if err != nil {
		return err
	}
	if _, err := os.Stat(host); err != nil {
		return fmt.Errorf("host project not found at %s\n"+
			"scaffold one with `gossamer create`, or pass -host <dir>", host)
	}
	switch o.platform.name {
	case "ios":
		return runIOS(o, host)
	default:
		return runAndroid(o, host)
	}
}

// hostDirFor resolves the platform host project directory: -host if given, else
// the sibling ios/ or android/ next to the bind package.
func hostDirFor(o buildOpts) (string, error) {
	if o.host != "" {
		return filepath.Abs(o.host)
	}
	dir, err := packageDir(o.pkg)
	if err != nil {
		return "", err
	}
	return filepath.Join(filepath.Dir(dir), o.platform.name), nil
}

// packageDir returns the filesystem directory of a Go package pattern.
func packageDir(pkg string) (string, error) {
	out, err := exec.Command("go", "list", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		return "", fmt.Errorf("locate package %q: %w", pkg, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// packageName returns a Go package's name (e.g. hnmobile).
func packageName(pkg string) (string, error) {
	out, err := exec.Command("go", "list", "-f", "{{.Name}}", pkg).Output()
	if err != nil {
		return "", fmt.Errorf("read package name for %q: %w", pkg, err)
	}
	name := strings.TrimSpace(string(out))
	if name == "" {
		return "", fmt.Errorf("empty package name for %q", pkg)
	}
	return name, nil
}

// frameworkName is the module name gomobile gives the iOS framework: the bind
// package's name, capitalized (package hnmobile → Hnmobile). The host's
// project.yml references <Name>.xcframework and Swift `import <Name>`.
func frameworkName(o buildOpts) (string, error) {
	name, err := packageName(o.pkg)
	if err != nil {
		return "", err
	}
	return strings.ToUpper(name[:1]) + name[1:], nil
}

// --- iOS ---

func runIOS(o buildOpts, host string) error {
	if runtime.GOOS != "darwin" {
		return fmt.Errorf("building for iOS requires macOS")
	}
	if !xcodeSelected() {
		return fmt.Errorf("full Xcode required (not just Command Line Tools); run:\n" +
			"  sudo xcode-select -s /Applications/Xcode.app/Contents/Developer")
	}
	fw, err := frameworkName(o)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "gossamer: binding Go → "+fw+".xcframework")
	if err := gomobileBind(o, "ios,iossimulator", filepath.Join(host, fw+".xcframework")); err != nil {
		return err
	}

	udid, dev, err := pickSimulator()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gossamer: simulator %s\n", dev)

	if fileExists(filepath.Join(host, "project.yml")) {
		if !have("xcodegen") {
			return fmt.Errorf("xcodegen not found — install with: brew install xcodegen")
		}
		if err := run(host, nil, "xcodegen", "generate"); err != nil {
			return fmt.Errorf("xcodegen: %w", err)
		}
	}
	proj, err := findByExt(host, ".xcodeproj")
	if err != nil {
		return err
	}
	scheme, err := firstScheme(proj)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "gossamer: xcodebuild "+scheme)
	if err := run(host, nil, "xcodebuild",
		"-project", filepath.Base(proj), "-scheme", scheme,
		"-sdk", "iphonesimulator", "-configuration", "Debug",
		"-destination", "id="+udid, "-derivedDataPath", "build", "build"); err != nil {
		return fmt.Errorf("xcodebuild: %w", err)
	}

	app := filepath.Join(host, "build", "Build", "Products", "Debug-iphonesimulator", scheme+".app")
	bundleID, err := appBundleID(app)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gossamer: install + launch %s\n", bundleID)
	_ = exec.Command("xcrun", "simctl", "boot", udid).Run() // ignore "already booted"
	_ = exec.Command("open", "-a", "Simulator").Run()
	if err := run("", nil, "xcrun", "simctl", "install", udid, app); err != nil {
		return fmt.Errorf("simctl install: %w", err)
	}
	_ = exec.Command("xcrun", "simctl", "terminate", udid, bundleID).Run()
	return run("", nil, "xcrun", "simctl", "launch", "--console-pty", udid, bundleID)
}

func xcodeSelected() bool {
	out, err := exec.Command("xcode-select", "-p").Output()
	return err == nil && strings.Contains(string(out), "Xcode")
}

// pickSimulator returns the udid+name of a booted iPhone simulator, else the
// first available one.
func pickSimulator() (udid, name string, err error) {
	out, err := exec.Command("xcrun", "simctl", "list", "-j", "devices", "available").Output()
	if err != nil {
		return "", "", fmt.Errorf("simctl list: %w", err)
	}
	var data struct {
		Devices map[string][]struct {
			UDID, Name, State string
		}
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "", "", fmt.Errorf("parse simctl output: %w", err)
	}
	var firstU, firstN string
	for _, devs := range data.Devices {
		for _, d := range devs {
			if !strings.Contains(d.Name, "iPhone") {
				continue
			}
			if d.State == "Booted" {
				return d.UDID, d.Name, nil
			}
			if firstU == "" {
				firstU, firstN = d.UDID, d.Name
			}
		}
	}
	if firstU == "" {
		return "", "", fmt.Errorf("no iPhone simulator available (add one in Xcode > Settings > Platforms)")
	}
	return firstU, firstN, nil
}

// firstScheme returns the project's first shared scheme.
func firstScheme(proj string) (string, error) {
	out, err := exec.Command("xcodebuild", "-list", "-json", "-project", proj).Output()
	if err != nil {
		return "", fmt.Errorf("xcodebuild -list: %w", err)
	}
	var data struct {
		Project struct{ Schemes []string }
	}
	if err := json.Unmarshal(out, &data); err != nil {
		return "", fmt.Errorf("parse scheme list: %w", err)
	}
	if len(data.Project.Schemes) == 0 {
		return "", fmt.Errorf("no schemes in %s", filepath.Base(proj))
	}
	return data.Project.Schemes[0], nil
}

// appBundleID reads CFBundleIdentifier from a built .app's Info.plist.
func appBundleID(app string) (string, error) {
	out, err := exec.Command("plutil", "-extract", "CFBundleIdentifier", "raw", "-o", "-",
		filepath.Join(app, "Info.plist")).Output()
	if err != nil {
		return "", fmt.Errorf("read bundle id from %s: %w", app, err)
	}
	return strings.TrimSpace(string(out)), nil
}

// --- Android ---

func runAndroid(o buildOpts, host string) error {
	sdk := androidHome()
	if sdk == "" {
		return fmt.Errorf("Android SDK not found — set ANDROID_HOME (or install via Android Studio)")
	}
	if err := ensureAndroidSDK(sdk); err != nil {
		return err
	}
	jdk, err := findJDK()
	if err != nil {
		return err
	}
	name, err := packageName(o.pkg)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "gossamer: binding Go → app/libs/"+name+".aar")
	aar := filepath.Join(host, "app", "libs", name+".aar")
	if err := gomobileBind(o, "android", aar); err != nil {
		return err
	}

	env := []string{"ANDROID_HOME=" + sdk, "JAVA_HOME=" + jdk}
	gradle := "gradle"
	if gw := filepath.Join(host, "gradlew"); fileExists(gw) {
		gradle = gw
	} else if !have("gradle") {
		return fmt.Errorf("no ./gradlew in %s and no gradle on PATH", host)
	}
	fmt.Fprintln(os.Stderr, "gossamer: gradle :app:assembleDebug")
	if err := run(host, env, gradle, ":app:assembleDebug"); err != nil {
		return fmt.Errorf("gradle: %w", err)
	}

	apk := filepath.Join(host, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	adb := filepath.Join(sdk, "platform-tools", "adb")
	if !deviceConnected(adb) {
		fmt.Fprintf(os.Stderr, "gossamer: built %s\n"+
			"No device/emulator connected. Start one, then:\n"+
			"  %s install -r %s\n", apk, adb, apk)
		return nil
	}
	appID, err := gradleApplicationID(filepath.Join(host, "app", "build.gradle"))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gossamer: install + launch %s\n", appID)
	if err := run("", nil, adb, "install", "-r", apk); err != nil {
		return fmt.Errorf("adb install: %w", err)
	}
	return run("", nil, adb, "shell", "am", "start", "-n", launchActivity(adb, appID))
}

// launchActivity resolves the app's launcher activity (e.g.
// dev.gossamer.hn/.MainActivity) so `am start -n` works without hardcoding the
// class; falls back to the scaffold's .MainActivity convention.
func launchActivity(adb, appID string) string {
	out, err := exec.Command(adb, "shell", "cmd", "package", "resolve-activity",
		"--brief", "-c", "android.intent.category.LAUNCHER", appID).Output()
	if err == nil {
		lines := strings.Split(strings.TrimSpace(string(out)), "\n")
		last := strings.TrimSpace(lines[len(lines)-1])
		if strings.Contains(last, "/") {
			return last
		}
	}
	return appID + "/.MainActivity"
}

func androidHome() string {
	for _, e := range []string{"ANDROID_HOME", "ANDROID_SDK_ROOT"} {
		if v := os.Getenv(e); v != "" && dirExists(v) {
			return v
		}
	}
	home, _ := os.UserHomeDir()
	for _, p := range []string{
		filepath.Join(home, "Library", "Android", "sdk"), // macOS
		filepath.Join(home, "Android", "Sdk"),            // Linux
	} {
		if dirExists(p) {
			return p
		}
	}
	return ""
}

// ensureAndroidSDK installs the pinned NDK + CMake if they're missing (the JNI
// surface shim needs them). One-time; no-op once present.
func ensureAndroidSDK(sdk string) error {
	need := map[string]string{
		"ndk;" + androidNDK:     filepath.Join(sdk, "ndk", androidNDK),
		"cmake;" + androidCMake: filepath.Join(sdk, "cmake", androidCMake),
	}
	var missing []string
	for pkg, dir := range need {
		if !dirExists(dir) {
			missing = append(missing, pkg)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sm := sdkmanager(sdk)
	if sm == "" {
		return fmt.Errorf("need %s but sdkmanager not found; install them via Android Studio > SDK Manager",
			strings.Join(missing, " + "))
	}
	for _, pkg := range missing {
		fmt.Fprintf(os.Stderr, "gossamer: installing %s (one-time)\n", pkg)
		cmd := exec.Command(sm, "--sdk_root="+sdk, pkg)
		cmd.Stdin = strings.NewReader(strings.Repeat("y\n", 50)) // accept licenses
		cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
		_ = cmd.Run()
	}
	for pkg, dir := range need {
		if !dirExists(dir) {
			return fmt.Errorf("failed to install %s", pkg)
		}
	}
	return nil
}

func sdkmanager(sdk string) string {
	for _, p := range []string{
		filepath.Join(sdk, "cmdline-tools", "latest", "bin", "sdkmanager"),
		filepath.Join(sdk, "tools", "bin", "sdkmanager"),
	} {
		if fileExists(p) {
			return p
		}
	}
	if p, err := exec.LookPath("sdkmanager"); err == nil {
		return p
	}
	return ""
}

// findJDK returns a JAVA_HOME for a JDK 17–21 (AGP 8.5's supported range).
func findJDK() (string, error) {
	if h := os.Getenv("JAVA_HOME"); h != "" {
		if v := javaMajor(h); v >= 17 && v <= 21 {
			return h, nil
		}
	}
	cands := []string{"/Applications/Android Studio.app/Contents/jbr/Contents/Home"}
	if runtime.GOOS == "darwin" {
		for _, v := range []string{"21", "17"} {
			if out, err := exec.Command("/usr/libexec/java_home", "-v", v).Output(); err == nil {
				cands = append(cands, strings.TrimSpace(string(out)))
			}
		}
	}
	for _, c := range cands {
		if v := javaMajor(c); v >= 17 && v <= 21 {
			return c, nil
		}
	}
	return "", fmt.Errorf("no JDK 17–21 found (set JAVA_HOME); AGP 8.5 needs one")
}

// javaMajor returns the major version of the JDK at home, or 0.
func javaMajor(home string) int {
	out, err := exec.Command(filepath.Join(home, "bin", "java"), "-version").CombinedOutput()
	if err != nil {
		return 0
	}
	m := regexp.MustCompile(`version "(\d+)`).FindSubmatch(out)
	if m == nil {
		return 0
	}
	n := 0
	for _, b := range m[1] {
		n = n*10 + int(b-'0')
	}
	return n
}

func deviceConnected(adb string) bool {
	out, err := exec.Command(adb, "devices").Output()
	if err != nil {
		return false
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, ln := range lines[1:] { // skip "List of devices attached"
		if strings.Contains(ln, "\tdevice") {
			return true
		}
	}
	return false
}

var reApplicationID = regexp.MustCompile(`applicationId\s+['"]([^'"]+)['"]`)

func gradleApplicationID(buildGradle string) (string, error) {
	b, err := os.ReadFile(buildGradle)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", buildGradle, err)
	}
	m := reApplicationID.FindSubmatch(b)
	if m == nil {
		return "", fmt.Errorf("no applicationId in %s", buildGradle)
	}
	return string(m[1]), nil
}

// --- shared ---

func fileExists(p string) bool { fi, err := os.Stat(p); return err == nil && !fi.IsDir() }
func dirExists(p string) bool  { fi, err := os.Stat(p); return err == nil && fi.IsDir() }

func findByExt(dir, ext string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ext) {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no %s in %s", ext, dir)
}
