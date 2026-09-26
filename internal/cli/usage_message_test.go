package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/doug/gophics/internal/cli/capscan"
)

// With a project.yml, xcodegen regenerates Info.plist on every run, so the
// advice to add the key to the plist sent the user in a loop: the key was
// gone by the next run and the same error printed again. The message has to
// name the file that survives.
func TestMissingUsageMessageNamesProjectYMLWhenItOwnsThePlist(t *testing.T) {
	host := t.TempDir()
	plist := filepath.Join(host, "App", "Info.plist")
	yml := filepath.Join(host, "project.yml")
	missing := []capscan.MissingIOSKey{{Key: "NSCameraUsageDescription", Capabilities: []string{"camera"}}}

	got := missingUsageMessage(plist, yml, missing)
	if !strings.Contains(got, "<key>/<string>") || strings.Contains(got, "project.yml") {
		t.Errorf("without project.yml the plist is the file to edit; got:\n%s", got)
	}

	if err := os.WriteFile(yml, []byte("name: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got = missingUsageMessage(plist, yml, missing)
	for _, want := range []string{yml, "targets.<name>.info.properties", "NSCameraUsageDescription", "regenerates"} {
		if !strings.Contains(got, want) {
			t.Errorf("with project.yml the message lacks %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "<key>/<string>") {
		t.Errorf("with project.yml the message still asks for a plist edit, which xcodegen undoes:\n%s", got)
	}
}
