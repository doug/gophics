package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// The CLI is documented as running on Windows, and none of these paths had
// ever been exercised there: `gophics run` built a binary it could not launch,
// and `gophics run -p android` could not find an SDK, an sdkmanager or a JDK
// that Android Studio had installed in its default places. Each helper takes
// its inputs as parameters so the Windows answer can be checked from any host.

func TestExeNameAddsExeOnWindows(t *testing.T) {
	if got := exeName("windows", "app"); got != "app.exe" {
		t.Errorf("exeName(windows) = %q, want app.exe — exec.Command cannot start an extension-less file there", got)
	}
	for _, goos := range []string{"darwin", "linux"} {
		if got := exeName(goos, "app"); got != "app" {
			t.Errorf("exeName(%s) = %q, want app", goos, got)
		}
	}
}

func TestSDKManagerFindsTheWindowsBatchFile(t *testing.T) {
	sdk := t.TempDir()
	bin := filepath.Join(sdk, "cmdline-tools", "latest", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(bin, "sdkmanager.bat")
	if err := os.WriteFile(want, []byte("@echo off\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Not on PATH either, so the fallback cannot answer for the fixture.
	t.Setenv("PATH", t.TempDir())
	if got := sdkmanager(sdk); got != want {
		t.Errorf("sdkmanager = %q, want %q (the Windows tools ship only the .bat)", got, want)
	}
}

func TestAndroidSDKCandidatesCoverEveryOS(t *testing.T) {
	got := androidSDKCandidates(filepath.Join("home", "me"), filepath.Join("C:", "Users", "me", "AppData", "Local"))
	for _, want := range []string{
		filepath.Join("home", "me", "Library", "Android", "sdk"),
		filepath.Join("home", "me", "Android", "Sdk"),
		filepath.Join("C:", "Users", "me", "AppData", "Local", "Android", "Sdk"),
	} {
		found := false
		for _, c := range got {
			found = found || c == want
		}
		if !found {
			t.Errorf("androidSDKCandidates lacks %s; got %v", want, got)
		}
	}
	if n := len(androidSDKCandidates("h", "")); n != 2 {
		t.Errorf("with no LOCALAPPDATA, got %d candidates, want the two Unix ones", n)
	}
}
