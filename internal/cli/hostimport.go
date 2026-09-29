package cli

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// A host project reaches the Go side through one import of the bind package:
// `import hnmobile.Hnmobile` in the Android activity, `import Hnmobile` in the
// iOS app. When that name disagrees with the package the build produces — a
// project renamed or cloned under another directory, or scaffolded before
// create and build named the package the same way — the bind succeeds and the
// failure arrives minutes later as "Unresolved reference: lingomobile" from
// the Kotlin compiler, or "no such module" from Swift, in a file the user did
// not write. Checking the import first turns that into an error that names
// both sides and the line to change.
var (
	kotlinBindImport = regexp.MustCompile(`(?m)^\s*import\s+([a-z][a-z0-9_]*mobile)\.([A-Z][A-Za-z0-9_]*)\s*$`)
	swiftBindImport  = regexp.MustCompile(`(?m)^\s*import\s+([A-Z][A-Za-z0-9_]*mobile)\s*$`)
)

// checkHostBindImport returns an error when a source file in host imports a
// bind package other than want: the Go package name on Android, the framework
// name on iOS. A host that imports no *mobile package at all is left to its
// own devices — it may reach the bridge some other way.
func checkHostBindImport(host, want string, ios bool) error {
	exts, re := []string{".kt", ".java"}, kotlinBindImport
	if ios {
		exts, re = []string{".swift"}, swiftBindImport
	}
	var mismatch error
	walkErr := filepath.WalkDir(host, func(p string, d fs.DirEntry, err error) error {
		if err != nil || mismatch != nil {
			return nil
		}
		if d.IsDir() {
			// Build outputs hold generated copies of the sources, and
			// Pods/DerivedData hold other people's; neither is the host.
			switch d.Name() {
			case "build", ".gradle", ".cxx", "Pods", "DerivedData":
				return filepath.SkipDir
			}
			return nil
		}
		ok := false
		for _, e := range exts {
			ok = ok || strings.HasSuffix(p, e)
		}
		if !ok {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return nil
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			if m[1] == want {
				continue
			}
			rel, _ := filepath.Rel(host, p)
			fix := fmt.Sprintf("import %s.%s", want, titleFirst(want))
			if ios {
				fix = "import " + want
			}
			mismatch = fmt.Errorf(
				"%s imports %s, but this build produces %s.\n\n"+
					"The host has to import the package the build binds. It is named after\n"+
					"the last element of the app's import path, so a project renamed or\n"+
					"checked out under another name can disagree with the host it was\n"+
					"scaffolded with. Change the import (and the calls through it) to:\n\n"+
					"    %s",
				filepath.Join(filepath.Base(host), rel), m[1], want, fix)
			return nil
		}
		return nil
	})
	if walkErr != nil {
		return walkErr
	}
	return mismatch
}
