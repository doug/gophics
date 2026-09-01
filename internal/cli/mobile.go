package cli

import (
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"

	"regexp"
	"runtime"
	"strings"

	"github.com/doug/gophics/internal/cli/capscan"
)

// The pinned Android build bits the JNI surface shim needs; ensureAndroidSDK
// installs them if missing. Kept in sync with the scaffolded app/build.gradle
// (ndkVersion) and app CMakeLists.
const (
	androidNDK   = "26.1.10909125"
	androidCMake = "3.22.1"
)

// buildMobile wraps `gomobile bind` to produce the standalone native artifact
// (`gophics build -p ios/android`): an .xcframework for iOS, an .aar for
// Android, under build/<platform>. `gophics run` binds straight into the host
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
		// Align libgojni.so's LOAD segments to 16 KB. Devices from the Pixel 10
		// on can boot with 16 KB pages, and Android shows a "not compatible"
		// PageSizeMismatch dialog for apps whose native libs use the toolchain's
		// 4 KB default. The JNI shim gets the same treatment from its
		// CMakeLists; both libs have to be aligned for the dialog to go away.
		args = append(args, "-ldflags", "-extldflags=-Wl,-z,max-page-size=16384")
	}
	args = append(args, "-o", out)
	if t := tagList(o.platform, o.tags); t != "" {
		args = append(args, "-tags", t)
	}
	// The app's own package, generated when it does not carry one (bindgen.go).
	bindPkg, err := resolveBindPkg(o)
	if err != nil {
		return err
	}

	// Bind the shell bridge alongside the app's package.
	//
	// gomobile exports only the packages it is given, so without this a host
	// cannot name mobile.Bridge and every app re-declares forty passthroughs by
	// hand — which is what every example did until this line existed, and they
	// had already drifted apart.
	//
	// It costs shell/mobile a constraint: gomobile allows a second result only
	// when it is an error, and gobind copies Go doc comments into Javadoc where
	// prose containing a slash-star closes the comment early. Both are checked
	// by scripts/gates.sh so they fail here rather than in a gradle log.
	args = append(args, bindPkg, bridgePkg)
	if err := run("", nil, "gomobile", args...); err != nil {
		return fmt.Errorf("gomobile bind: %w", err)
	}
	return nil
}

// bridgePkg is the shell bridge every gophics mobile app runs on; hosts call
// its methods directly, so it is bound alongside the app's own package.
const bridgePkg = "github.com/doug/gophics/shell/mobile"

// runMobile is the `gophics run` path for ios/android: bind the Go side into
// the host project, build it, and install + launch on a simulator/device — the
// whole loop, no per-project scripts.
//
// The host project is the app's own ios/ or android/ directory when it has one
// (override with -host); an app that has neither gets a stock host generated
// under build/, so `gophics run -p ios` works on an app that is nothing but a
// main.go and a ui package.
func runMobile(o buildOpts) error {
	host, err := ensureHost(o)
	if err != nil {
		return err
	}
	switch o.platform.name {
	case "ios":
		return runIOS(o, host)
	default:
		return runAndroid(o, host)
	}
}

// hostDirFor resolves the platform host project directory: -host if given, else
// the ios/ or android/ belonging to the app.
//
// Which directory that is depends on what was named on the command line. A bind
// package (./examples/hn/mobile) has the host as its *sibling*; an app root
// (./examples/mirror), which is what a mobile build normally names now that the
// bind package is generated, has the host *inside* it. Getting this wrong picks
// up the directory one level too high — examples/ios rather than
// examples/mirror/ios — and reports a missing host project for one that exists.
func hostDirFor(o buildOpts) (string, error) {
	if o.host != "" {
		return filepath.Abs(o.host)
	}
	dir, err := packageDir(o.pkg)
	if err != nil {
		return "", err
	}
	name, err := packageName(o.pkg)
	if err != nil {
		return "", err
	}
	if name != "main" {
		dir = filepath.Dir(dir) // bind package: the host sits beside it
	}
	return filepath.Join(dir, o.platform.name), nil
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
	// Resolve through the same path the bind itself takes, so a generated bind
	// package is named the same here as it is there. Reading o.pkg directly
	// answers "Main" for an app root, and the host then looks for a framework
	// that was never built under a name nothing produces.
	pkg, err := resolveBindPkg(o)
	if err != nil {
		return "", err
	}
	name, err := packageName(pkg)
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
	if err := checkIOSPermissions(o, host); err != nil {
		return err
	}

	fw, err := frameworkName(o)
	if err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "gophics: binding Go → "+fw+".xcframework")
	if err := gomobileBind(o, "ios,iossimulator", filepath.Join(host, fw+".xcframework")); err != nil {
		return err
	}

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
	if o.device {
		return runIOSOnDevice(o, host, proj, scheme)
	}
	return runIOSOnSimulator(host, proj, scheme)
}

// runIOSOnSimulator builds for the simulator SDK and installs with simctl.
func runIOSOnSimulator(host, proj, scheme string) error {
	udid, dev, err := pickSimulator()
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: simulator %s\n", dev)
	fmt.Fprintln(os.Stderr, "gophics: xcodebuild "+scheme)
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
	fmt.Fprintf(os.Stderr, "gophics: install + launch %s\n", bundleID)
	_ = exec.Command("xcrun", "simctl", "boot", udid).Run() // ignore "already booted"
	_ = exec.Command("open", "-a", "Simulator").Run()
	if err := run("", nil, "xcrun", "simctl", "install", udid, app); err != nil {
		return fmt.Errorf("simctl install: %w", err)
	}
	_ = exec.Command("xcrun", "simctl", "terminate", udid, bundleID).Run()
	return run("", nil, "xcrun", "simctl", "launch", "--console-pty", udid, bundleID)
}

// runIOSOnDevice builds for the device SDK and installs with devicectl.
//
// Two things differ from the simulator beyond the SDK name. A device build is
// signed, and the host projects here declare no signing at all — so the team
// and automatic signing are passed on the command line rather than baked into
// project.yml, which would put one developer's team in the repository.
// -allowProvisioningUpdates lets Xcode create or refresh the profile rather
// than failing on a machine that has never built this bundle ID for a device.
func runIOSOnDevice(o buildOpts, host, proj, scheme string) error {
	dev, err := pickDevice()
	if err != nil {
		return err
	}
	team := o.team
	if team == "" {
		if team, err = developmentTeam(); err != nil {
			return fmt.Errorf("%w\npass one with -team <TEAMID>", err)
		}
	}
	fmt.Fprintf(os.Stderr, "gophics: device %s (team %s)\n", dev.name, team)
	fmt.Fprintln(os.Stderr, "gophics: xcodebuild "+scheme)
	if err := run(host, nil, "xcodebuild",
		"-project", filepath.Base(proj), "-scheme", scheme,
		"-sdk", "iphoneos", "-configuration", "Debug",
		"-destination", "id="+dev.udid, "-derivedDataPath", "build",
		"-allowProvisioningUpdates",
		"CODE_SIGN_STYLE=Automatic", "DEVELOPMENT_TEAM="+team,
		"build"); err != nil {
		return fmt.Errorf("xcodebuild: %w", err)
	}

	app := filepath.Join(host, "build", "Build", "Products", "Debug-iphoneos", scheme+".app")
	bundleID, err := appBundleID(app)
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: install + launch %s\n", bundleID)
	if err := run("", nil, "xcrun", "devicectl", "device", "install", "app",
		"--device", dev.identifier, app); err != nil {
		return fmt.Errorf("devicectl install: %w", err)
	}
	// --console streams the app's stdout/stderr back, which is the device
	// equivalent of simctl's --console-pty. Go's log output reaches it; note
	// it does not reach the device syslog.
	//
	// GOPHICS_* variables are forwarded into the app. devicectl passes through
	// anything prefixed DEVICECTL_CHILD_ in its own environment, which is how
	// a knob like GOPHICS_PACING reaches a process on the phone — there is
	// otherwise no way to set one there.
	return run("", childEnv(os.Environ()), "xcrun", "devicectl", "device", "process", "launch",
		"--device", dev.identifier, "--console", "--terminate-existing", bundleID)
}

// childEnv returns the DEVICECTL_CHILD_ re-exports for every GOPHICS_*
// variable in env. Only the additions: run appends them to the real
// environment itself.
func childEnv(env []string) []string {
	var out []string
	for _, kv := range env {
		if k, v, ok := strings.Cut(kv, "="); ok && strings.HasPrefix(k, "GOPHICS_") {
			out = append(out, "DEVICECTL_CHILD_"+k+"="+v)
		}
	}
	return out
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
	// The .aar is named for the bind package, which the app's build.gradle
	// references by that name — so resolve it the same way the bind does rather
	// than from o.pkg, which is "main" when an app root was named.
	bindPkg, err := resolveBindPkg(o)
	if err != nil {
		return err
	}
	name, err := packageName(bindPkg)
	if err != nil {
		return err
	}
	if err := syncAndroidPermissions(o, host); err != nil {
		return err
	}

	fmt.Fprintln(os.Stderr, "gophics: binding Go → app/libs/"+name+".aar")
	aar := filepath.Join(host, "app", "libs", name+".aar")
	// Clear any other .aar first. build.gradle includes libs/*.aar by pattern
	// rather than by name — deliberately, so the CLI can name the artifact after
	// the bind package — which means a leftover from a previous name is not
	// ignored but *added*, and gradle fails with pages of "Duplicate class
	// go.Seq" that name neither the stale file nor the cause. Renaming an app,
	// or switching between a hand-written mobile/ package and a generated one,
	// is enough to produce one.
	if err := pruneStaleAARs(filepath.Dir(aar), filepath.Base(aar)); err != nil {
		return err
	}
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
	fmt.Fprintln(os.Stderr, "gophics: gradle :app:assembleDebug")
	if err := run(host, env, gradle, ":app:assembleDebug"); err != nil {
		return fmt.Errorf("gradle: %w", err)
	}

	apk := filepath.Join(host, "app", "build", "outputs", "apk", "debug", "app-debug.apk")
	adb := filepath.Join(sdk, "platform-tools", "adb")
	if !deviceConnected(adb) {
		fmt.Fprintf(os.Stderr, "gophics: built %s\n"+
			"No device/emulator connected. Start one, then:\n"+
			"  %s install -r %s\n", apk, adb, apk)
		return nil
	}
	target, err := adbTarget(adb, o.serial)
	if err != nil {
		return err
	}
	appID, err := gradleApplicationID(filepath.Join(host, "app", "build.gradle"))
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "gophics: install + launch %s\n", appID)
	if err := run("", nil, adb, append(append([]string{}, target...), "install", "-r", apk)...); err != nil {
		return fmt.Errorf("adb install: %w", err)
	}
	launch := append(append([]string{}, target...), "shell", "am", "start", "-n", launchActivity(adb, appID))
	return run("", nil, adb, launch...)
}

// launchActivity resolves the app's launcher activity (e.g.
// dev.gophics.hn/.MainActivity) so `am start -n` works without hardcoding the
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
		fmt.Fprintf(os.Stderr, "gophics: installing %s (one-time)\n", pkg)
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

func deviceConnected(adb string) bool { return len(adbDevices(adb)) > 0 }

// adbDevices lists the serials adb reports as ready, in adb's own order —
// which puts physical devices before emulators.
func adbDevices(adb string) []string {
	out, err := exec.Command(adb, "devices").Output()
	if err != nil {
		return nil
	}
	var serials []string
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, ln := range lines[1:] { // skip "List of devices attached"
		serial, state, ok := strings.Cut(strings.TrimSpace(ln), "\t")
		if ok && strings.TrimSpace(state) == "device" {
			serials = append(serials, serial)
		}
	}
	return serials
}

// adbTarget returns the adb arguments that pin every later command to one
// device.
//
// adb refuses to install when more than one is attached, with an error that
// names neither of them — and a tablet plugged in beside a running emulator is
// the normal state of anyone debugging on hardware. -device picks explicitly;
// otherwise a physical device wins over an emulator, because someone who has
// just plugged one in means to use it.
func adbTarget(adb string, want string) ([]string, error) {
	serial, err := pickADBSerial(adbDevices(adb), want)
	if err != nil || serial == "" {
		return nil, err
	}
	return []string{"-s", serial}, nil
}

// pickADBSerial chooses among the attached devices. Separate from adbTarget so
// the choice can be tested without an adb to attach anything to.
func pickADBSerial(serials []string, want string) (string, error) {
	if len(serials) == 0 {
		return "", nil
	}
	if want != "" {
		for _, s := range serials {
			if s == want {
				return s, nil
			}
		}
		return "", fmt.Errorf("no connected device with serial %q; attached: %s",
			want, strings.Join(serials, ", "))
	}
	if len(serials) == 1 {
		return serials[0], nil
	}
	var physical []string
	for _, s := range serials {
		if !strings.HasPrefix(s, "emulator-") {
			physical = append(physical, s)
		}
	}
	switch len(physical) {
	case 1:
		fmt.Fprintf(os.Stderr, "gophics: %d devices attached; using %s "+
			"(pass -serial to choose)\n", len(serials), physical[0])
		return physical[0], nil
	case 0:
		fmt.Fprintf(os.Stderr, "gophics: %d emulators attached; using %s "+
			"(pass -serial to choose)\n", len(serials), serials[0])
		return serials[0], nil
	default:
		return "", fmt.Errorf("%d devices attached (%s); pass -serial <serial>",
			len(serials), strings.Join(physical, ", "))
	}
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

// syncAndroidPermissions derives the app's <uses-permission> set from the
// capabilities it actually reaches and writes it into the host manifest.
//
// Run before the build rather than after, so a capability added this morning is
// declared in the APK installed this afternoon. The manifest is scaffolded once
// and then hand-owned, which is exactly how it drifts: an app grows a camera
// screen and nobody remembers the manifest until it fails on a device.
func syncAndroidPermissions(o buildOpts, host string) error {
	manifest := filepath.Join(host, "app", "src", "main", "AndroidManifest.xml")
	if !fileExists(manifest) {
		return nil // a host project that keeps its manifest elsewhere
	}

	// Scan the bind package, not what was named on the command line. An app
	// root is package main, and a main package does not type-check for a mobile
	// GOOS — it wants external cgo linking that is not enabled — so scanning it
	// reports nothing and capscan correctly refuses to under-report.
	dir, err := bindPkgDir(o)
	if err != nil {
		return err
	}
	// Scan as the target, not as the host: a capability reached only from
	// android code is invisible to a scan of the host build.
	target := capscan.Target{GOOS: "android", GOARCH: "arm64"}
	if o.tags != "" {
		target.Tags = strings.Split(o.tags, ",")
	}
	res, err := capscan.Scan(dir, ".", target)
	if err != nil {
		return err
	}

	perm := capscan.Permissions(res)
	changed, err := capscan.AndroidManifest(manifest, perm, false)
	if err != nil {
		return err
	}
	if changed {
		fmt.Fprintf(os.Stderr, "gophics: manifest permissions updated (%s)\n",
			strings.Join(perm.Android, ", "))
	}
	if perm.RuntimeRequest {
		fmt.Fprintln(os.Stderr, "gophics: note — some of these are requested at "+
			"runtime, not granted at install; the app must ask via ctx.Permissions()")
	}
	return nil
}

// checkIOSPermissions fails the build when a capability needs a usage
// description the Info.plist does not supply.
//
// Checked, not generated: the text is app-specific prose that Apple reads at
// review, and a placeholder turns a build error into a rejection weeks later.
// findInfoPlist locates the app's Info.plist under an iOS host project.
//
// It used to be assumed at App/Info.plist. No project in this tree has ever
// been laid out that way — Xcode names the group after the app, so it is
// GophicsHN/Info.plist, Tally/Info.plist, GophicsMirror/Info.plist — so the
// check above found no file, returned nil, and never once ran. A guard that
// silently passes is worse than no guard, because it is counted on.
//
// Build products are skipped: a built .app and the bound .xcframework both
// contain an Info.plist, and neither is the one a person edits.
func findInfoPlist(host string) (string, error) {
	var found string
	err := filepath.WalkDir(host, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch {
			case strings.HasSuffix(d.Name(), ".xcframework"),
				strings.HasSuffix(d.Name(), ".framework"),
				strings.HasSuffix(d.Name(), ".app"),
				strings.HasSuffix(d.Name(), ".xcodeproj"),
				d.Name() == "build", d.Name() == "build-sim":
				return fs.SkipDir
			}
			return nil
		}
		if d.Name() == "Info.plist" && found == "" {
			found = path
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return found, nil
}

func checkIOSPermissions(o buildOpts, host string) error {
	plist, err := findInfoPlist(host)
	if err != nil {
		return err
	}
	if plist == "" {
		return nil
	}
	dir, err := bindPkgDir(o) // see the note in syncAndroidPermissions
	if err != nil {
		return err
	}
	target := capscan.Target{GOOS: "ios", GOARCH: "arm64"}
	if o.tags != "" {
		target.Tags = strings.Split(o.tags, ",")
	}
	res, err := capscan.Scan(dir, ".", target)
	if err != nil {
		return err
	}
	missing, err := capscan.CheckIOSUsageDescriptions(plist, res.Capabilities)
	if err != nil {
		return err
	}
	if len(missing) == 0 {
		return nil
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s is missing usage descriptions iOS requires:\n", plist)
	for _, m := range missing {
		fmt.Fprintf(&b, "  %s — needed by %s\n", m.Key, strings.Join(m.Capabilities, ", "))
	}
	b.WriteString("\nAdd each as a <key>/<string> pair saying why the app needs it.\n")
	b.WriteString("The text is shown to the user in the permission prompt and read at App Review,\n")
	b.WriteString("so it cannot be generated.")
	return errors.New(b.String())
}

// iosDevice is a paired device's two identities. They are not the same value
// and the two tools disagree about which they want: xcodebuild's
// -destination id= expects the hardware UDID, devicectl's --device expects
// the identifier it assigns.
type iosDevice struct {
	udid       string // hardware UDID, for xcodebuild
	identifier string // devicectl's handle
	name       string
}

// pickDevice returns a paired iOS device, preferring one whose tunnel is
// already up.
func pickDevice() (iosDevice, error) {
	tmp, err := os.CreateTemp("", "gophics-devices-*.json")
	if err != nil {
		return iosDevice{}, err
	}
	defer os.Remove(tmp.Name())
	tmp.Close()
	// devicectl only reports as JSON to a file, never to stdout.
	if out, err := exec.Command("xcrun", "devicectl", "list", "devices",
		"--json-output", tmp.Name()).CombinedOutput(); err != nil {
		return iosDevice{}, fmt.Errorf("devicectl list devices: %w\n%s", err, out)
	}
	raw, err := os.ReadFile(tmp.Name())
	if err != nil {
		return iosDevice{}, err
	}
	return selectDevice(raw)
}

// selectDevice picks a device out of devicectl's JSON: a paired iOS one,
// preferring a connected tunnel over a merely paired device.
func selectDevice(raw []byte) (iosDevice, error) {
	var data struct {
		Result struct {
			Devices []struct {
				Identifier         string                `json:"identifier"`
				DeviceProperties   struct{ Name string } `json:"deviceProperties"`
				HardwareProperties struct {
					UDID     string `json:"udid"`
					Platform string `json:"platform"`
				} `json:"hardwareProperties"`
				ConnectionProperties struct {
					TunnelState  string `json:"tunnelState"`
					PairingState string `json:"pairingState"`
				} `json:"connectionProperties"`
			} `json:"devices"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &data); err != nil {
		return iosDevice{}, fmt.Errorf("parse devicectl output: %w", err)
	}
	var first iosDevice
	for _, d := range data.Result.Devices {
		if d.HardwareProperties.Platform != "iOS" || d.ConnectionProperties.PairingState != "paired" {
			continue
		}
		dev := iosDevice{
			udid:       d.HardwareProperties.UDID,
			identifier: d.Identifier,
			name:       d.DeviceProperties.Name,
		}
		if d.ConnectionProperties.TunnelState == "connected" {
			return dev, nil
		}
		if first.udid == "" {
			first = dev
		}
	}
	if first.udid == "" {
		return iosDevice{}, fmt.Errorf("no paired iOS device found — connect one, unlock it, and trust this Mac")
	}
	return first, nil
}

// developmentTeam reads the Apple Developer team ID from the codesigning
// identity in the keychain.
//
// It is the certificate's organizational unit, not the identifier in
// parentheses in the identity's name — those differ, and using the visible one
// produces a team that does not exist.
func developmentTeam() (string, error) {
	out, err := exec.Command("security", "find-identity", "-v", "-p", "codesigning").Output()
	if err != nil {
		return "", fmt.Errorf("security find-identity: %w", err)
	}
	var cn string
	for line := range strings.SplitSeq(string(out), "\n") {
		a := strings.Index(line, `"`)
		b := strings.LastIndex(line, `"`)
		if a < 0 || b <= a {
			continue
		}
		n := line[a+1 : b]
		if strings.HasPrefix(n, "Apple Development") || strings.HasPrefix(n, "iPhone Developer") {
			cn = n
			break
		}
	}
	if cn == "" {
		return "", fmt.Errorf("no Apple Development signing identity in the keychain")
	}
	pemBytes, err := exec.Command("security", "find-certificate", "-c", cn, "-p").Output()
	if err != nil {
		return "", fmt.Errorf("security find-certificate %q: %w", cn, err)
	}
	blk, _ := pem.Decode(pemBytes)
	if blk == nil {
		return "", fmt.Errorf("certificate for %q is not PEM", cn)
	}
	cert, err := x509.ParseCertificate(blk.Bytes)
	if err != nil {
		return "", fmt.Errorf("parse certificate for %q: %w", cn, err)
	}
	if len(cert.Subject.OrganizationalUnit) == 0 {
		return "", fmt.Errorf("certificate for %q carries no team (OU)", cn)
	}
	return cert.Subject.OrganizationalUnit[0], nil
}
