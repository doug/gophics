package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The scaffolded hosts are copies of the reference hosts, and nothing said so.
//
// shell/mobile/native/GophicsPlatform.{kt,swift} are what the docs point at;
// internal/cli/templates/... are what `gophics create` actually writes. They
// are the same file bar one line — the package declaration on Android, the
// framework import on iOS — but they are two files on disk with no link
// between them, so an edit to either silently drifts from the other. That is
// the same shape as the bug scripts/gates.sh was written for, where `gophics
// build` and package/android.sh held separate copies of a linker flag and only
// a device could tell you they disagreed.
//
// It is not hypothetical here either: wiring setLocale into the reference hosts
// left the templates behind, so every app `gophics create` produced would have
// kept formatting as en-US on a German device while the reference host was
// fixed. This test is what caught it.
//
// Cheap and always on, unlike the swiftc check above — a string compare is not
// a reason to need an opt-in.
//
// The tally example's copies are in the table too. They are what a user of
// `gophics create` ends up with, and they drifted the other way: a commit
// meant to bring them up to the templates took the reference file verbatim,
// so the iOS host imported `Mobile` — the typecheck test's framework name —
// instead of `Tallymobile`, the only module the app binds, and nothing here
// compiles Swift by default. Their substitution is the rendered value, so the
// same check reads in both directions.
func TestScaffoldedHostsMatchTheReferenceHosts(t *testing.T) {
	for _, c := range []struct {
		native, tmpl string
		from, to     string
		// copy marks an example's rendered copy of the template rather than
		// the reference it is generated from, which flips the fix: regenerate
		// the copy from the template, not the template from the copy.
		copy bool
	}{{
		native: "shell/mobile/native/GophicsPlatform.kt",
		tmpl:   "internal/cli/templates/mobile/android/app/src/main/kotlin/GophicsPlatform.kt.tmpl",
		from:   "package dev.gophics.host",
		to:     "package {{.AndroidPkg}}",
	}, {
		native: "shell/mobile/native/GophicsPlatform.swift",
		tmpl:   "internal/cli/templates/mobile/ios/App/GophicsPlatform.swift.tmpl",
		from:   "import Mobile",
		to:     "import {{.Framework}}",
	}, {
		native: "examples/tally/android/app/src/main/java/com/gophics/tally/GophicsPlatform.kt",
		tmpl:   "internal/cli/templates/mobile/android/app/src/main/kotlin/GophicsPlatform.kt.tmpl",
		from:   "package com.gophics.tally",
		to:     "package {{.AndroidPkg}}",
		copy:   true,
	}, {
		native: "examples/tally/ios/Tally/GophicsPlatform.swift",
		tmpl:   "internal/cli/templates/mobile/ios/App/GophicsPlatform.swift.tmpl",
		from:   "import Tallymobile",
		to:     "import {{.Framework}}",
		copy:   true,
	}} {
		name := filepath.Base(c.native)
		if c.copy {
			name = "tally/" + name
		}
		t.Run(name, func(t *testing.T) {
			native, err := os.ReadFile(filepath.Join("..", "..", c.native))
			if err != nil {
				t.Fatal(err)
			}
			tmpl, err := os.ReadFile(filepath.Join("..", "..", c.tmpl))
			if err != nil {
				t.Fatal(err)
			}
			// Exactly one substitution, so a native file that stops containing
			// the line fails here rather than producing a template that is
			// quietly missing its package declaration.
			if n := strings.Count(string(native), c.from+"\n"); n != 1 {
				t.Fatalf("%s contains %q %d times, want exactly 1", c.native, c.from, n)
			}
			want := strings.Replace(string(native), c.from+"\n", c.to+"\n", 1)
			if string(tmpl) == want {
				return
			}
			if c.copy {
				t.Errorf("%s has drifted from %s.\n"+
					"They must be identical except for %q -> %q.\n"+
					"Regenerate the copy from the template:\n"+
					"    sed 's|^%s$|%s|' %s > %s",
					c.native, c.tmpl, c.to, c.from, c.to, c.from, c.tmpl, c.native)
				return
			}
			t.Errorf("%s has drifted from %s.\n"+
				"They must be identical except for %q -> %q.\n"+
				"Edit the native file, then regenerate:\n"+
				"    sed 's|^%s$|%s|' %s > %s",
				c.tmpl, c.native, c.from, c.to, c.from, c.to, c.native, c.tmpl)
		})
	}
}

// Every Swift file in an example's iOS host must import the framework the
// project binds. gomobile emits one module, named after the xcframework in
// ios/project.yml, and an `import Mobile` copied from the reference host fails
// with "no such module" — which nothing on a Mac without GOPHICS_IOS_HOSTS=1
// ever ran into. The framework name comes from project.yml rather than from
// the files themselves so a host whose every file imports the wrong module
// still fails.
func TestExampleIOSHostsImportTheirFramework(t *testing.T) {
	projects, err := filepath.Glob(filepath.Join("..", "..", "examples", "*", "ios", "project.yml"))
	if err != nil {
		t.Fatal(err)
	}
	if len(projects) == 0 {
		t.Fatal("no examples/*/ios/project.yml found; the glob is wrong, not the examples")
	}
	for _, project := range projects {
		iosDir := filepath.Dir(project)
		t.Run(filepath.Base(filepath.Dir(iosDir)), func(t *testing.T) {
			framework := boundFramework(t, project)
			swift, err := filepath.Glob(filepath.Join(iosDir, "*", "*.swift"))
			if err != nil {
				t.Fatal(err)
			}
			if len(swift) == 0 {
				t.Fatalf("%s has no Swift files beside it", project)
			}
			for _, file := range swift {
				src, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				var mobile []string
				for _, line := range strings.Split(string(src), "\n") {
					if mod, ok := strings.CutPrefix(line, "import "); ok && strings.HasSuffix(mod, "mobile") {
						mobile = append(mobile, mod)
					}
				}
				if len(mobile) != 1 || mobile[0] != framework {
					t.Errorf("%s imports %v; every file in the host must import %s, the framework %s binds",
						file, mobile, framework, project)
				}
			}
		})
	}
}

// boundFramework reads the one `- framework: X.xcframework` line out of an
// xcodegen project file.
func boundFramework(t *testing.T, project string) string {
	t.Helper()
	src, err := os.ReadFile(project)
	if err != nil {
		t.Fatal(err)
	}
	var found []string
	for _, line := range strings.Split(string(src), "\n") {
		line = strings.TrimSpace(line)
		if fw, ok := strings.CutPrefix(line, "- framework: "); ok {
			found = append(found, strings.TrimSuffix(strings.TrimSpace(fw), ".xcframework"))
		}
	}
	if len(found) != 1 {
		t.Fatalf("%s names %d frameworks, want exactly one: %v", project, len(found), found)
	}
	return found[0]
}
