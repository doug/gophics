package shell_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Every capability must be implemented on mobile, or say why not.
//
// This gate exists because the alternative already happened: seven capabilities
// — Share, Notifier, SecureStorage, Permissions, FilePicker, Preferences and
// WebView — had no method on the mobile Bridge at all, while the hand-written
// status table called them "stub", which means "interface plus a compile-checked
// TODO". Nothing compared the claim to the code, so the gap survived long enough
// to be documented as something else.
//
// A capability reaching a platform is a decision either way. Implemented, or
// listed below with the reason. What is not allowed is neither.
//
// "Implemented" here means the accessor exists, which is weaker than working —
// the generated capability matrix marks a capability whose implementation does
// nothing as "hollow", and that is the place to look for it. This gate is only
// about whether the platform was considered at all.
var mobileExempt = map[string]string{
	"Gamepads": "both platforms support controllers (iOS GameController, Android " +
		"InputDevice) and gophics does not bridge either yet. The Bridge used to " +
		"publish a Poll that always returned nothing, which is worse than absence: " +
		"a game could not tell 'no controller is connected' from 'this build " +
		"cannot see controllers', so it would show a pairing prompt forever. nil " +
		"is the detectable answer until the host side exists",
	"FolderPicker": "both platforms have the equivalent — Android's " +
		"ACTION_OPEN_DOCUMENT_TREE and iOS's UIDocumentPickerViewController in " +
		"folder mode, each yielding a handle that survives a restart — and " +
		"gophics does not bridge either yet. This one needs host-side work the " +
		"desktop and web backends did not: what comes back is a tree URI or a " +
		"security-scoped bookmark rather than a path, and reading it goes " +
		"through DocumentFile or NSFileCoordinator instead of ordinary file " +
		"I/O, so shell/desktop's osFolder does not carry over. nil until then, " +
		"which is what lets an app hide the open-folder button instead of " +
		"offering one that fails",
	"Menus": "a desktop menu bar; mobile apps draw their own menus in the widget tree",
	"Tray":  "a desktop system tray / menu bar extra; no mobile equivalent exists",
	"WindowControl": "title, fullscreen and resize belong to a windowing system; " +
		"the mobile analogues (orientation lock, status-bar style) are a different " +
		"capability, not this one",
	"Permissions": "mobile asks per capability instead — Camera.Authorize, " +
		"Microphone.Authorize, Notifier.Authorize, Photos.Authorize — which is " +
		"what both platforms actually model; a generic permission API would have " +
		"to invent a capability enum that maps onto neither",
	"WebView": "deliberately unbuilt: a native subview composited over the GPU " +
		"layer is the most expensive item here and the least useful, because " +
		"Apple and Google both steer OAuth to ASWebAuthenticationSession and " +
		"Custom Tabs, which OpenURL already reaches",
}

// desktopExempt: what desktop deliberately does not get, and why. Entries here
// are decisions, not backlog — a capability that is merely unbuilt should say
// "not yet" and name what building it takes.
var desktopExempt = map[string]string{
	"Biometric": "TouchID exists on Mac laptops via LocalAuthentication, but no " +
		"desktop consumer has needed it and Windows Hello / Linux have no common " +
		"story; not yet, and nil is detectable",
	"Haptic": "NSHapticFeedbackManager can buzz a trackpad; Linux and Windows " +
		"desktops have nothing comparable, so apps must treat haptics as " +
		"optional anyway — nil until a consumer justifies the objc bridge work",
	"Locale": "by design, per the contract in shell/locale.go: desktop has the " +
		"environment variables, so Locale stays nil and intl.Auto reads them " +
		"directly. The recorded caveat: a Finder-launched app gets almost no " +
		"environment, so a German Mac can read as en-US there — if that bites, " +
		"the fix is NSLocale through the objc bridge, and this entry is where " +
		"that decision gets reopened",
	"Permissions": "desktop OSes ask per resource at first use (camera, mic) " +
		"through their own prompts; there is no app-driven permission API to wrap",
	"Photos": "no desktop OS has an app-facing photo-library abstraction; " +
		"saving an image to disk is FilePicker.Save",
	"Share": "NSSharingServicePicker is real on macOS and cited in capgen's " +
		"README as the canonical FFI example, but it is a main-thread panel " +
		"with delegate callbacks — real objc work — and Linux/Windows desktops " +
		"have no share-sheet concept at all, so apps need a fallback anyway; " +
		"not yet",
	"TextInput": "this capability raises and lowers a soft keyboard; desktop " +
		"keyboards are hardware and text arrives through window key events, so " +
		"there is nothing to raise — by design, not omission",
	"WakeLock": "IOPMAssertion / SetThreadExecutionState / D-Bus inhibit all " +
		"exist; no desktop consumer yet, and a stray assertion that outlives " +
		"its app drains a laptop, so this waits for a real need",
	"WebView": "deliberately unbuilt for the same reason as mobile: a native " +
		"subview composited over the GPU layer is maximum cost for a need " +
		"OpenURL already covers",
}

// webExempt: what the browser platform does not get.
var webExempt = map[string]string{
	"Biometric": "WebAuthn is credential ceremony, not a fingerprint check — a " +
		"different model that would deserve its own capability if wanted",
	"Menus": "a desktop menu bar; a page has no chrome to put one in",
	"Photos": "no browser API adds to the OS photo library; a download " +
		"(FilePicker.Save) is what the platform offers",
	"Tray": "a desktop system tray; no web equivalent exists",
	"WakeLock": "not yet: navigator.wakeLock is real and small — the screen " +
		"variant only, which is exactly what the capability promises",
}

// terminalExempt: the terminal is a character grid on a remote-able TTY; most
// capabilities are pixel, dialog, or device concepts that the *emulator* owns,
// not the app. The ones that are honest "not yet" say so.
var terminalExempt = map[string]string{
	"Accessibility": "the terminal is already an accessible text surface; the " +
		"emulator and screen reader own it, not the app",
	"Battery":       "readable in principle (sysfs, IOKit) but no TUI consumer yet",
	"Biometric":     "no sensor reachable from a TTY",
	"Camera":        "not yet: devmedia could capture headless, but no TUI has asked",
	"CameraPreview": "no pixel surface to preview into",
	"Connectivity":  "readable in principle; no TUI consumer yet",
	"FilePicker": "no system dialog to present; a TUI picker is an app-level " +
		"widget, not a platform capability",
	"FolderPicker": "same as FilePicker: nothing to present from a TTY",
	"Gamepads":     "no TUI consumer yet",
	"Geolocation":  "no location source is part of the TTY contract",
	"Haptic":       "no hardware",
	"Lifecycle": "not yet: SIGTSTP/SIGCONT map cleanly onto background/foreground " +
		"and would make a polite TUI",
	"Links": "not yet: open/xdg-open works from a local terminal; ambiguous over " +
		"SSH, which is the design question to answer first",
	"Locale":        "not yet: $LANG is right there; lands with the desktop version",
	"Menus":         "no menu bar",
	"Notifier":      "not yet: OSC 9 exists but emulator support is patchy",
	"Permissions":   "nothing here prompts",
	"Photos":        "no photo library",
	"SecureStorage": "not yet: same stores as desktop once desktop has them",
	"Share":         "no share sheet",
	"Socket":        "not yet: nothing terminal-specific blocks it; no consumer",
	"TextInput":     "keyboards are hardware here; same reasoning as desktop",
	"Tray":          "no tray",
	"WakeLock":      "a TTY does not sleep; the emulator's host might, but that is not ours",
	"WebView":       "a character grid cannot composite a browser",
	"WindowControl": "the emulator owns the window; apps at most set the title, " +
		"which is not this capability's contract",
}

// capabilityNames reads the <X>Window interfaces the same way capgen does.
func capabilityNames(t *testing.T) []string {
	t.Helper()
	fset := token.NewFileSet()
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	var caps []string
	for _, path := range files {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok {
				return true
			}
			name := ts.Name.Name
			if name == "Window" || !strings.HasSuffix(name, "Window") {
				return true
			}
			it, ok := ts.Type.(*ast.InterfaceType)
			if !ok {
				return true
			}
			for _, m := range it.Methods.List {
				fn, ok := m.Type.(*ast.FuncType)
				if !ok || len(m.Names) != 1 {
					continue
				}
				if fn.Params.NumFields() != 0 || fn.Results.NumFields() != 1 {
					continue
				}
				caps = append(caps, m.Names[0].Name)
			}
			return true
		})
	}
	sort.Strings(caps)
	return caps
}

var windowAccessor = regexp.MustCompile(`(?m)^func \(w \*window\) (\w+)\(\) shell\.\w+`)
var bridgeAccessor = regexp.MustCompile(`(?m)^func \(b \*Bridge\) (\w+)\(\) shell\.\w+`)

// platforms is every place a capability can land. One row per platform, one
// exempt map per row: a capability is implemented there, or its entry says why
// not, and nothing is allowed to be neither. The mobile row came first (see
// the story above); the others exist because the same argument does not stop
// at mobile — desktop was carrying six unrecorded absences, keychain among
// them, that nobody had ever decided on.
var platforms = []struct {
	name     string
	dir      string
	accessor *regexp.Regexp
	exempt   map[string]string
	minHave  int // scan sanity floor: fewer accessors than this means the regex broke
}{
	{"mobile", "mobile", bridgeAccessor, mobileExempt, 10},
	{"desktop", "desktop", windowAccessor, desktopExempt, 5},
	{"web", "web", windowAccessor, webExempt, 10},
	{"terminal", "terminal", windowAccessor, terminalExempt, 1},
}

func TestEveryCapabilityReachesEveryPlatformOrSaysWhyNot(t *testing.T) {
	caps := capabilityNames(t)
	if len(caps) < 20 {
		t.Fatalf("derived only %d capabilities; the scan is broken and this test "+
			"would pass without checking anything", len(caps))
	}
	known := map[string]bool{}
	for _, c := range caps {
		known[c] = true
	}

	for _, plat := range platforms {
		have := map[string]bool{}
		paths, err := filepath.Glob(filepath.Join(plat.dir, "*.go"))
		if err != nil || len(paths) == 0 {
			t.Fatalf("no Go files under %s/: %v", plat.dir, err)
		}
		for _, p := range paths {
			if strings.HasSuffix(p, "_test.go") {
				continue
			}
			src, err := os.ReadFile(p)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range plat.accessor.FindAllStringSubmatch(string(src), -1) {
				have[m[1]] = true
			}
		}
		if len(have) < plat.minHave {
			t.Fatalf("found only %d %s capability accessors; the scan is broken",
				len(have), plat.name)
		}

		for _, c := range caps {
			_, exempt := plat.exempt[c]
			switch {
			case have[c] && exempt:
				t.Errorf("capability %q is implemented on %s but still listed as "+
					"exempt — remove its entry", c, plat.name)
			case !have[c] && !exempt:
				t.Errorf("capability %q has no %s implementation and no exempt "+
					"entry. Implement it under shell/%s, or add an entry saying why "+
					"the platform does not get it — so \"not on %s\" is a decision "+
					"on the record rather than something nobody noticed.",
					c, plat.name, plat.dir, plat.name)
			}
		}
		for name := range plat.exempt {
			if !known[name] {
				t.Errorf("%s exempt list has %q, which is not a capability", plat.name, name)
			}
		}
	}
}
