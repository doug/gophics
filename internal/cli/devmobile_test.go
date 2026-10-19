package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// bridgeSrc reads a file from shell/mobile, which is the other end of every
// agreement tested here.
func bridgeSrc(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "shell", "mobile", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The hand-off file's name is agreed by two packages that never meet.
//
// The app picks the directory (whatever the host passed to SetFilesDir), so
// the name is the only part of the path both sides have to get right — and
// getting it wrong is silent: the app writes a snapshot, the CLI waits for a
// file that never appears, the loop still rebuilds, and state is quietly lost
// on every iteration with nothing to see in any log.
func TestDevStateFileNameAgrees(t *testing.T) {
	want := `devStateFile = "` + devStateFile + `"`
	if src := bridgeSrc(t, "devstate.go"); !strings.Contains(src, want) {
		t.Errorf("shell/mobile/devstate.go does not declare %s — the CLI waits for "+
			"files/%s after backgrounding the app, and the app has to write that name",
			want, devStateFile)
	}
}

// The linker flag names a variable that exists.
//
// -X is silent about a symbol it cannot find: a rename in shell/mobile would
// leave `gophics dev -p android` building, installing and relaunching happily
// with state preservation switched off and no diagnostic anywhere.
func TestDevRestartVarExists(t *testing.T) {
	name := devRestartVar[strings.LastIndex(devRestartVar, ".")+1:]
	if src := bridgeSrc(t, "devstate.go"); !strings.Contains(src, "var "+name+" string") {
		t.Errorf("shell/mobile/devstate.go has no `var %s string`; `-X %s=1` would "+
			"be accepted and do nothing", name, devRestartVar)
	}
}

// Only a dev bind links the hand-off in, and the alignment flag survives it.
func TestBindLDFlags(t *testing.T) {
	android := platform{name: "android"}
	ios := platform{name: "ios"}
	for _, tc := range []struct {
		name string
		o    buildOpts
		want []string
		not  []string
	}{
		{"android run", buildOpts{platform: android}, []string{alignFlag}, []string{"-X"}},
		{"android dev", buildOpts{platform: android, dev: &devSession{}},
			[]string{alignFlag, "-X " + devRestartVar + "=1"}, nil},
		{"ios run", buildOpts{platform: ios}, nil, []string{"-X", alignFlag}},
		{"ios dev", buildOpts{platform: ios, dev: &devSession{}},
			[]string{"-X " + devRestartVar + "=1"}, []string{alignFlag}},
	} {
		got := bindLDFlags(tc.o)
		for _, w := range tc.want {
			if !strings.Contains(got, w) {
				t.Errorf("%s: -ldflags %q lacks %q", tc.name, got, w)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(got, n) {
				t.Errorf("%s: -ldflags %q should not contain %q", tc.name, got, n)
			}
		}
	}
}

// A release bind covers every ABI; a dev bind narrows to the one attached.
//
// The direction matters more than the mapping: narrowing a `build` would ship
// an .aar that only runs on the machine that built it.
func TestBindTargetsNarrowOnlyForDev(t *testing.T) {
	ios := buildOpts{platform: platform{name: "ios"}}
	if got := iosBindTarget(ios); got != "ios,iossimulator" {
		t.Errorf("ios run target = %q, want both slices", got)
	}
	ios.dev = &devSession{}
	if got := iosBindTarget(ios); got != "iossimulator" {
		t.Errorf("ios simulator dev target = %q, want iossimulator only", got)
	}
	ios.device = true
	if got := iosBindTarget(ios); got != "ios" {
		t.Errorf("ios device dev target = %q, want ios only", got)
	}

	android := buildOpts{platform: platform{name: "android"}}
	if got := androidBindTarget(android, "", nil); got != "android" {
		t.Errorf("android run target = %q, want all four ABIs", got)
	}
	for abi, want := range map[string]string{
		"arm64-v8a":   "android/arm64",
		"x86_64":      "android/amd64",
		"armeabi-v7a": "android", // build.gradle filters it out; narrowing would mislead
		"":            "android", // unreadable: fall back rather than guess
	} {
		if got := androidABITarget(abi); got != want {
			t.Errorf("ABI %q → %q, want %q", abi, got, want)
		}
	}
}

// The mobile dev loop replaced a refusal, and the usage text has to have moved
// with it: a `dev` that works on a phone while `gophics help` says to iterate
// on web is worse than either.
func TestDevNoLongerRefusesMobile(t *testing.T) {
	src, err := os.ReadFile("dev.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(src), "isn't supported for") {
		t.Error("dev.go still refuses a mobile platform")
	}
	if strings.Contains(usage, "hot-reload") || strings.Contains(usage, "hot reload") {
		t.Error("usage promises hot reload, which Go cannot do on any platform; " +
			"what this tool does is state-preserving hot restart")
	}
}

// Web state preservation is a two-part agreement: the dev server's injected
// client writes the marker key, and the wasm reads exactly that one.
func TestWebDevMarkerAgrees(t *testing.T) {
	if !strings.Contains(reloadClient, `sessionStorage.setItem("gophics:dev","1")`) {
		t.Fatalf("reload client does not set the dev marker:\n%s", reloadClient)
	}
	b, err := os.ReadFile(filepath.Join("..", "..", "app", "devstate_web.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `devMarkerKey = "gophics:dev"`) {
		t.Error(`app/devstate_web.go does not read the "gophics:dev" marker the ` +
			"dev server writes, so a web reload would silently drop UI state")
	}
}
