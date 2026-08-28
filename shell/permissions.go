package shell

import "sort"

// What each capability has to be declared as, per platform.
//
// A permission-bearing capability is useless without a declaration the platform
// can see: Android reads <uses-permission> from the manifest at install time,
// iOS reads a usage-description key from Info.plist and kills the process
// without one, and a sandboxed macOS build reads entitlements. Those files are
// scaffolded once and then owned by hand, so they drift from the code — an app
// grows a camera screen and nobody remembers the manifest until it fails on a
// device.
//
// This table is the single place that mapping lives, so it can be derived
// rather than remembered. `gophics build` finds which capabilities an app
// actually reaches, looks them up here, and writes the result into the host
// project. See internal/cli/capscan.
//
// The table is keyed by capability name — the same names capgen derives from
// the <X>Window interfaces in this package — and a test asserts every
// capability has an entry, so adding one without deciding its permissions fails
// rather than silently declaring nothing.

// ManifestPermission is what one capability requires from each platform that has a
// permission model. Every field is optional; most capabilities need nothing.
type ManifestPermission struct {
	// Android names for <uses-permission>, fully qualified.
	Android []string
	// AndroidMaxSDK caps a permission at an API level, for ones the platform
	// stopped requiring. Keyed by the permission name; absent means no cap.
	AndroidMaxSDK map[string]int
	// IOSKeys are Info.plist usage-description keys. They carry app-specific
	// prose — "why does this app want your camera" — which cannot be derived,
	// so a build fails rather than inventing one. See UsageDescriptions.
	IOSKeys []string
	// MacEntitlements are com.apple.security.* keys a sandboxed macOS build
	// needs. Ignored for an unsandboxed binary, which is the default for a
	// plain `go build`.
	MacEntitlements []string
	// RuntimeRequest marks a permission the user is asked for while the app
	// runs, not at install: Android's dangerous permissions and every iOS
	// usage-description key. Declaring it is necessary and not sufficient —
	// the app must also ask, through ctx.Permissions().
	RuntimeRequest bool
}

// capabilityManifestPermissions maps a capability to what it must declare.
//
// Entries are deliberately present-and-empty rather than absent when a
// capability needs nothing: absent would be indistinguishable from forgotten,
// and the test that guards this table cannot tell those apart either.
var capabilityManifestPermissions = map[string]ManifestPermission{
	"Accessibility": {},
	// Output only, and now that is true. While recording lived on Audio this
	// entry was a live bug: an app calling Audio.Record without ever touching
	// Microphone got no RECORD_AUDIO and no NSMicrophoneUsageDescription, so
	// the capture was denied on Android and the process terminated on iOS.
	// The comment here already described the intended model; the interface was
	// what disagreed with it.
	"Speakers": {},
	"Battery":  {},
	"Camera": {
		Android:         []string{"android.permission.CAMERA"},
		IOSKeys:         []string{"NSCameraUsageDescription"},
		MacEntitlements: []string{"com.apple.security.device.camera"},
		RuntimeRequest:  true,
	},
	"CameraPreview": {
		Android:         []string{"android.permission.CAMERA"},
		IOSKeys:         []string{"NSCameraUsageDescription"},
		MacEntitlements: []string{"com.apple.security.device.camera"},
		RuntimeRequest:  true,
	},
	"Connectivity": {
		Android: []string{"android.permission.ACCESS_NETWORK_STATE"},
	},
	// Modern Android picks files through the Storage Access Framework, which
	// hands back a URI the app already has rights to — no permission involved.
	"FilePicker": {
		MacEntitlements: []string{"com.apple.security.files.user-selected.read-write"},
	},
	"Gamepads": {},
	"Geolocation": {
		Android: []string{
			"android.permission.ACCESS_COARSE_LOCATION",
			"android.permission.ACCESS_FINE_LOCATION",
		},
		IOSKeys:         []string{"NSLocationWhenInUseUsageDescription"},
		MacEntitlements: []string{"com.apple.security.personal-information.location"},
		RuntimeRequest:  true,
	},
	"Haptic": {
		Android: []string{"android.permission.VIBRATE"},
	},
	"Lifecycle": {},
	"Links":     {},
	"Menus":     {},
	"Microphone": {
		Android:         []string{"android.permission.RECORD_AUDIO"},
		IOSKeys:         []string{"NSMicrophoneUsageDescription"},
		MacEntitlements: []string{"com.apple.security.device.audio-input"},
		RuntimeRequest:  true,
	},
	// POST_NOTIFICATIONS arrived in API 33; on older releases it does not
	// exist and declaring it unconditionally is harmless but noisy.
	"Notifier": {
		Android:        []string{"android.permission.POST_NOTIFICATIONS"},
		RuntimeRequest: true,
	},
	// Biometry needs no manifest permission on either platform: iOS gates it on
	// the same NSFaceIDUsageDescription string it shows in the prompt, and
	// Android dropped USE_BIOMETRIC's runtime requirement. The iOS key is
	// required only for Face ID — the app crashes without it on a Face ID
	// device — so it is declared here and always synced.
	"Biometric": {
		IOSKeys: []string{"NSFaceIDUsageDescription"},
	},
	"Permissions": {}, // the mechanism for asking, not something to ask for
	// Add-only access, deliberately: an app that saves a picture has no reason
	// to enumerate the library, and the add-only key is the smaller ask.
	"Photos": {
		Android:        []string{"android.permission.WRITE_EXTERNAL_STORAGE"},
		IOSKeys:        []string{"NSPhotoLibraryAddUsageDescription"},
		RuntimeRequest: true,
	},
	// Keeping the screen awake needs a manifest permission on Android only when
	// done with a PowerManager wake lock. The window flag FLAG_KEEP_SCREEN_ON,
	// which is what the reference host uses, needs none — and is the right tool,
	// because it is scoped to the window and cannot outlive it.
	"WakeLock":      {},
	"Preferences":   {},
	"SecureStorage": {},
	"Share":         {},
	"Socket": {
		Android:         []string{"android.permission.INTERNET"},
		MacEntitlements: []string{"com.apple.security.network.client"},
	},
	"TextInput": {},
	"Tray":      {},
	"WebView": {
		Android:         []string{"android.permission.INTERNET"},
		MacEntitlements: []string{"com.apple.security.network.client"},
	},
	"WindowControl": {},
}

// BaselineManifestPermission is what every gophics app needs, whatever it does.
//
// This is not derived from the capability set, and the reason is worth
// recording. The obvious design was to look for net/http in the import graph
// and declare INTERNET only when it appears — but widget/netimage.go imports
// net/http, and widget is the core package, so the check is true for every app
// that has ever been built and the precision is a fiction. Measured on
// examples/counter: 311 packages, net/http among them, no networking in the app.
//
// Declaring it unconditionally is also the honest answer rather than a
// concession. NetworkImage is in the core widget set, so any app can reach the
// network without naming it; INTERNET is a normal Android permission, granted
// at install with no prompt and no review friction; and the failure it prevents
// — an app that installs and cannot resolve a hostname — is one nobody enjoys
// diagnosing.
var BaselineManifestPermission = ManifestPermission{
	Android:         []string{"android.permission.INTERNET"},
	MacEntitlements: []string{"com.apple.security.network.client"},
}

// ManifestPermissionFor returns what a capability must declare, and whether the
// capability is known at all.
func ManifestPermissionFor(capability string) (ManifestPermission, bool) {
	p, ok := capabilityManifestPermissions[capability]
	return p, ok
}

// CapabilitiesWithPermissions lists the capabilities that require any
// declaration, sorted. Used by tests and by `gophics doctor`.
func CapabilitiesWithPermissions() []string {
	var out []string
	for name, p := range capabilityManifestPermissions {
		if len(p.Android) > 0 || len(p.IOSKeys) > 0 || len(p.MacEntitlements) > 0 {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// KnownCapabilities lists every capability the table covers, sorted.
func KnownCapabilities() []string {
	out := make([]string, 0, len(capabilityManifestPermissions))
	for name := range capabilityManifestPermissions {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// MergeManifest folds the permissions for a set of capabilities into one deduplicated,
// sorted result. Unknown names are ignored; the caller has already decided what
// to do about them.
func MergeManifest(capabilities []string, extra ...ManifestPermission) ManifestPermission {
	var (
		android = map[string]bool{}
		iosKeys = map[string]bool{}
		macEnts = map[string]bool{}
		maxSDK  = map[string]int{}
		runtime bool
	)
	add := func(p ManifestPermission) {
		for _, a := range p.Android {
			android[a] = true
		}
		for _, k := range p.IOSKeys {
			iosKeys[k] = true
		}
		for _, e := range p.MacEntitlements {
			macEnts[e] = true
		}
		for k, v := range p.AndroidMaxSDK {
			maxSDK[k] = v
		}
		runtime = runtime || p.RuntimeRequest
	}
	for _, c := range capabilities {
		if p, ok := capabilityManifestPermissions[c]; ok {
			add(p)
		}
	}
	for _, p := range extra {
		add(p)
	}

	out := ManifestPermission{RuntimeRequest: runtime}
	out.Android = sortedKeys(android)
	out.IOSKeys = sortedKeys(iosKeys)
	out.MacEntitlements = sortedKeys(macEnts)
	if len(maxSDK) > 0 {
		out.AndroidMaxSDK = maxSDK
	}
	return out
}

func sortedKeys(m map[string]bool) []string {
	if len(m) == 0 {
		return nil
	}
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
